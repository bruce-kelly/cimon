package views

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// RunDetailView renders the drill-down into a single workflow run.
type RunDetailView struct {
	Run      models.WorkflowRun
	RepoName string
	Cursor   components.Selector
	expanded map[int]bool // job index -> expanded
}

// NewRunDetailView creates a run detail view, auto-expanding failed jobs.
func NewRunDetailView(run models.WorkflowRun, repoName string) *RunDetailView {
	rv := &RunDetailView{
		Run:      run,
		RepoName: repoName,
		expanded: make(map[int]bool),
	}
	rv.Cursor.SetCount(len(run.Jobs))
	for i, job := range run.Jobs {
		if job.Conclusion == "failure" {
			rv.expanded[i] = true
		}
	}
	return rv
}

// SelectedJob returns the job at the cursor, or nil if no jobs.
func (rv *RunDetailView) SelectedJob() *models.Job {
	if len(rv.Run.Jobs) == 0 {
		return nil
	}
	return &rv.Run.Jobs[rv.Cursor.Index()]
}

// IsExpanded returns whether the job at index has its steps expanded.
func (rv *RunDetailView) IsExpanded(index int) bool {
	return rv.expanded[index]
}

// ToggleExpand toggles step expansion for the selected job.
func (rv *RunDetailView) ToggleExpand() {
	idx := rv.Cursor.Index()
	rv.expanded[idx] = !rv.expanded[idx]
}

// SetJobs populates jobs (after async fetch) and auto-expands failures.
func (rv *RunDetailView) SetJobs(jobs []models.Job) {
	rv.Run.Jobs = jobs
	rv.expanded = make(map[int]bool)
	rv.Cursor.SetCount(len(jobs))
	for i, job := range jobs {
		if job.Conclusion == "failure" {
			rv.expanded[i] = true
		}
	}
}

// Render draws the run detail view.
func (rv *RunDetailView) Render(width, height int) string {
	var lines []string

	// Run summary
	dot := runStatusDot(rv.Run)
	sha := rv.Run.HeadSHA
	if len(sha) > 6 {
		sha = sha[:6]
	}
	ago := components.FormatTimeAgo(rv.Run.UpdatedAt)
	summary := fmt.Sprintf("  %s %s %s  %s by %s", rv.Run.HeadBranch, dot, sha, rv.Run.Event, rv.Run.Actor)
	lines = append(lines, summary+lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  "+ago))

	// Stats line
	stats := rv.statsLine()
	lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  "+stats))
	lines = append(lines, "")

	// Jobs section
	jobsHeader := lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  Jobs")
	lines = append(lines, jobsHeader)

	if len(rv.Run.Jobs) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  Loading jobs..."))
	}
	for i, job := range rv.Run.Jobs {
		selected := rv.Cursor.Index() == i
		lines = append(lines, rv.renderJobLine(job, selected, width))
		if rv.expanded[i] && len(job.Steps) > 0 {
			for _, step := range job.Steps {
				lines = append(lines, rv.renderStepLine(step))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (rv *RunDetailView) statsLine() string {
	status := rv.Run.Conclusion
	if rv.Run.IsActive() {
		status = rv.Run.Status
	}

	passed := 0
	total := len(rv.Run.Jobs)
	for _, j := range rv.Run.Jobs {
		if j.Conclusion == "success" {
			passed++
		}
	}

	elapsed := components.FormatDuration(rv.Run.Elapsed())

	if total == 0 {
		return fmt.Sprintf("%s · elapsed %s", status, elapsed)
	}
	return fmt.Sprintf("%s · %d/%d jobs passed · elapsed %s", status, passed, total, elapsed)
}

func (rv *RunDetailView) renderJobLine(job models.Job, selected bool, width int) string {
	dot := jobStatusDot(job)
	elapsed := ""
	if job.StartedAt != nil && job.CompletedAt != nil {
		elapsed = components.FormatDuration(job.CompletedAt.Sub(*job.StartedAt))
	}

	runner := ""
	if job.RunnerName != "" {
		runner = lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  " + job.RunnerName)
	}

	line := fmt.Sprintf("  %s %s  %s%s", dot, job.Name, elapsed, runner)
	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}
	return line
}

func (rv *RunDetailView) renderStepLine(step models.Step) string {
	dot := stepStatusDot(step)
	return fmt.Sprintf("      %s %s", dot, step.Name)
}

func runStatusDot(run models.WorkflowRun) string {
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

func jobStatusDot(job models.Job) string {
	if job.Status == "in_progress" {
		return lipgloss.NewStyle().Foreground(ui.ColorBlue).Render("●")
	}
	switch job.Conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	case "failure":
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("○")
	}
}

func stepStatusDot(step models.Step) string {
	switch step.Conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	case "failure":
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	case "skipped":
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("⧗")
	}
}
