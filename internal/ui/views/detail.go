package views

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/review"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// DetailView renders the per-repo drill-in with runs and PRs.
type DetailView struct {
	Repo     RepoState
	Cursor   components.Selector
	RunCount int // boundary: cursor < RunCount means run selected
}

// NewDetailView creates a detail view for the given repo.
// Deduplicates runs to latest per workflow, sorted by group.
func NewDetailView(repo RepoState) *DetailView {
	repo.Runs = latestRunPerWorkflow(repo.Runs)
	sortRunsByGroup(repo.Runs, repo.WorkflowGroups)
	dv := &DetailView{
		Repo:     repo,
		RunCount: len(repo.Runs),
	}
	total := len(repo.Runs) + len(repo.ReviewItems)
	dv.Cursor.SetCount(total)
	return dv
}

// latestRunPerWorkflow returns the most recent run per workflow file.
// Runs arrive newest-first from the API, so the first per file wins.
func latestRunPerWorkflow(runs []models.WorkflowRun) []models.WorkflowRun {
	seen := make(map[string]bool)
	var result []models.WorkflowRun
	for _, r := range runs {
		if seen[r.WorkflowFile] {
			continue
		}
		seen[r.WorkflowFile] = true
		result = append(result, r)
	}
	return result
}

// groupPriority returns sort order for group labels (lower = first).
func groupPriority(label string) int {
	lower := strings.ToLower(label)
	switch {
	case strings.Contains(lower, "ci"):
		return 0
	case strings.Contains(lower, "build"):
		return 1
	case strings.Contains(lower, "release") || strings.Contains(lower, "deploy"):
		return 2
	case strings.Contains(lower, "agent"):
		return 3
	default:
		return 4
	}
}

func sortRunsByGroup(runs []models.WorkflowRun, groups map[string]string) {
	if groups == nil {
		return
	}
	sort.SliceStable(runs, func(i, j int) bool {
		gi := groupPriority(groups[runs[i].WorkflowFile])
		gj := groupPriority(groups[runs[j].WorkflowFile])
		return gi < gj
	})
}

// IsRunSelected returns true if the cursor is on a run (not a PR).
func (dv *DetailView) IsRunSelected() bool {
	return dv.Cursor.Index() < dv.RunCount
}

// SelectedRun returns the run at cursor, or nil if cursor is on a PR.
func (dv *DetailView) SelectedRun() *models.WorkflowRun {
	idx := dv.Cursor.Index()
	if idx >= dv.RunCount || dv.RunCount == 0 {
		return nil
	}
	return &dv.Repo.Runs[idx]
}

// SelectedReviewItem returns the review item at cursor, or nil if cursor is on a run.
func (dv *DetailView) SelectedReviewItem() *review.ReviewItem {
	idx := dv.Cursor.Index()
	if idx < dv.RunCount {
		return nil
	}
	prIdx := idx - dv.RunCount
	if prIdx >= len(dv.Repo.ReviewItems) {
		return nil
	}
	return &dv.Repo.ReviewItems[prIdx]
}

// Render draws the detail view for one repo.
func (dv *DetailView) Render(width, height int) string {
	var lines []string

	// Repo header
	repoHeader := lipgloss.NewStyle().Foreground(ui.ColorFg).Bold(true).Render(dv.Repo.RepoName)
	lines = append(lines, repoHeader)
	lines = append(lines, "")

	if len(dv.Repo.Runs) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  No recent runs"))
	}
	lastGroup := ""
	for i, run := range dv.Repo.Runs {
		groupLabel := dv.Repo.WorkflowGroups[run.WorkflowFile]
		if groupLabel == "" {
			groupLabel = "Other"
		}
		if groupLabel != lastGroup {
			if lastGroup != "" {
				lines = append(lines, "")
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(groupLabel))
			lastGroup = groupLabel
		}
		selected := dv.Cursor.Index() == i
		lines = append(lines, detailRunLine(run, selected, width))
		if selected && len(run.Jobs) > 0 {
			for _, job := range run.Jobs {
				lines = append(lines, detailJobLine(job))
			}
		}
	}

	lines = append(lines, "")

	// Pull Requests section
	prHeader := lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("Pull Requests")
	lines = append(lines, prHeader)

	if len(dv.Repo.ReviewItems) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  No open PRs"))
	}
	for i, item := range dv.Repo.ReviewItems {
		cursorIdx := dv.RunCount + i
		selected := dv.Cursor.Index() == cursorIdx
		lines = append(lines, detailPRLine(item, selected, width))
	}

	return strings.Join(lines, "\n")
}

func detailRunLine(run models.WorkflowRun, selected bool, width int) string {
	dot := detailRunStatusDot(run)
	sha := run.HeadSHA
	if len(sha) > 6 {
		sha = sha[:6]
	}
	ago := components.FormatTimeAgo(run.UpdatedAt)
	name := lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.Name)
	line := fmt.Sprintf("  %s %s %s  %s  %s", run.HeadBranch, dot, sha, ago, name)

	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}
	return line
}

func detailRunStatusDot(run models.WorkflowRun) string {
	if run.IsActive() {
		return lipgloss.NewStyle().Foreground(ui.ColorBlue).Render("●")
	}
	switch run.Conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	case "failure":
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("○")
	}
}

func detailJobLine(job models.Job) string {
	var dot string
	switch job.Conclusion {
	case "success":
		dot = lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	case "failure":
		dot = lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	default:
		dot = lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("○")
	}

	elapsed := ""
	if job.StartedAt != nil && job.CompletedAt != nil {
		elapsed = components.FormatDuration(job.CompletedAt.Sub(*job.StartedAt))
	}

	return fmt.Sprintf("    %s %s  %s", dot, job.Name, elapsed)
}

func detailPRLine(item review.ReviewItem, selected bool, width int) string {
	pr := item.PR

	var ciDot string
	switch pr.CIStatus {
	case "success":
		ciDot = lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("CI✓")
	case "failure":
		ciDot = lipgloss.NewStyle().Foreground(ui.ColorRed).Render("CI✗")
	default:
		ciDot = lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("CI⧗")
	}

	agent := ""
	if pr.IsAgent {
		agent = lipgloss.NewStyle().Foreground(ui.ColorPurple).Render("agent") + "  "
	}

	approved := ""
	if pr.ReviewState == "approved" {
		approved = " " + lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✔")
	}

	title := pr.Title
	maxTitle := width - 30
	if maxTitle < 10 {
		maxTitle = 10
	}
	if len(title) > maxTitle {
		title = title[:maxTitle-1] + "…"
	}

	age := detailFormatAge(item.Age.Hours())
	num := fmt.Sprintf("#%d", pr.Number)

	line := fmt.Sprintf("  %s  %s  %s%s %s%s", num, title, agent, age, ciDot, approved)

	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}
	return line
}

func detailFormatAge(hours float64) string {
	if hours < 1 {
		return "<1h"
	}
	if hours < 24 {
		return fmt.Sprintf("%.0fh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%.0fd", days)
}
