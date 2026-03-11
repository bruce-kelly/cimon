package views

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// CompactView renders the compact repo list with inline expansion.
type CompactView struct {
	Repos  []RepoState
	Cursor components.Selector
}

// NewCompactView creates a compact view from repo states.
func NewCompactView(repos []RepoState) *CompactView {
	cv := &CompactView{Repos: repos}
	cv.Cursor.SetCount(len(repos))
	return cv
}

// UpdateRepos replaces the repo list, preserving cursor position.
func (cv *CompactView) UpdateRepos(repos []RepoState) {
	cv.Repos = repos
	cv.Cursor.SetCount(len(repos))
}

// SelectedRepo returns the currently selected repo state, or nil if empty.
func (cv *CompactView) SelectedRepo() *RepoState {
	if len(cv.Repos) == 0 {
		return nil
	}
	return &cv.Repos[cv.Cursor.Index()]
}

// AcknowledgeSelected clears the NEW flag on the selected repo.
func (cv *CompactView) AcknowledgeSelected() {
	if r := cv.SelectedRepo(); r != nil {
		r.NewFlag = false
		r.UserAcknowledged = true
	}
}

// Render draws the compact view. Returns a plain string (caller wraps in tea.View).
func (cv *CompactView) Render(width, height int) string {
	if len(cv.Repos) == 0 {
		return renderEmpty(width, height)
	}

	var lines []string

	for i, repo := range cv.Repos {
		selected := i == cv.Cursor.Index()
		lines = append(lines, renderRepoLine(repo, selected, width))

		// Inline expansion for critical failures
		if repo.Inline.Worst == StatusFailed && repo.Inline.FailedWorkflow != "" {
			lines = append(lines, renderFailedInline(repo.Inline, width))
		}
		// Inline note for agent-only failures
		if repo.Inline.AgentFailCount > 0 && repo.Inline.Worst != StatusFailed {
			lines = append(lines, renderAgentFailInline(repo.Inline.AgentFailCount, width))
		}
		// Inline expansion for active runs
		for _, ar := range repo.Inline.ActiveRuns {
			lines = append(lines, renderActiveInline(ar, width))
		}
	}

	// All-green message if every repo is passing with no active runs
	allQuiet := true
	for _, r := range cv.Repos {
		if r.Inline.Worst != StatusPassing {
			allQuiet = false
			break
		}
	}
	if allQuiet {
		for len(lines) < height-3 {
			lines = append(lines, "")
		}
		style := lipgloss.NewStyle().Foreground(ui.ColorGreen)
		lines = append(lines, style.Render("all passing"))
	}

	return strings.Join(lines, "\n")
}

func renderEmpty(width, height int) string {
	msg := "No repos configured. Run `cimon init`."
	style := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	return style.Render(msg)
}

func renderRepoLine(repo RepoState, selected bool, width int) string {
	dot := statusDot(repo.Inline.Worst)
	icon := statusIcon(repo.Inline)
	prText := renderPRSummary(repo.PRSummary)

	newFlag := ""
	if repo.NewFlag {
		newFlag = lipgloss.NewStyle().Foreground(ui.ColorRed).Bold(true).Render(" NEW")
	}

	line := fmt.Sprintf("%s %s  %s  %s%s", dot, repo.RepoName, icon, prText, newFlag)

	if selected {
		style := lipgloss.NewStyle().Background(ui.ColorSelection)
		line = style.Render(padRight(line, width))
	}

	return line
}

func renderPRSummary(s PRSummary) string {
	if s.Total == 0 {
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("—")
	}
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)

	noun := "PRs"
	if s.Total == 1 {
		noun = "PR"
	}
	text := fmt.Sprintf("%d %s", s.Total, noun)

	if s.Ready > 0 {
		text += muted.Render(fmt.Sprintf(" (%d ready)", s.Ready))
	} else if s.CIPending {
		text += muted.Render(" (CI ⧗)")
	}
	return text
}

func renderFailedInline(status InlineStatus, width int) string {
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	red := lipgloss.NewStyle().Foreground(ui.ColorRed)

	var jobs []string
	for _, j := range status.FailedJobs {
		jobs = append(jobs, j+" "+red.Render("✗"))
	}
	ago := components.FormatTimeAgo(status.FailedAt)
	return muted.Render("  "+status.FailedWorkflow+": ") + strings.Join(jobs, "  ") + muted.Render("  "+ago)
}

func renderAgentFailInline(count int, width int) string {
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	amber := lipgloss.NewStyle().Foreground(ui.ColorAmber)
	noun := "agent workflow"
	if count > 1 {
		noun = "agent workflows"
	}
	return muted.Render("  ") + amber.Render(fmt.Sprintf("%d %s failing", count, noun))
}

func renderActiveInline(ar ActiveRunInfo, width int) string {
	muted := lipgloss.NewStyle().Foreground(ui.ColorMuted)
	green := lipgloss.NewStyle().Foreground(ui.ColorGreen)

	bar := progressBar(ar.CompletedJobs, ar.TotalJobs, 10)
	elapsed := components.FormatDuration(ar.Elapsed)
	progress := fmt.Sprintf("%d/%d", ar.CompletedJobs, ar.TotalJobs)

	return muted.Render("  "+ar.Name+" ") + green.Render(bar) + muted.Render(" "+progress+"  "+elapsed)
}

func statusDot(worst RepoStatus) string {
	var c color.Color
	switch worst {
	case StatusFailed:
		c = ui.ColorRed
	case StatusAgentFailed:
		c = ui.ColorAmber
	case StatusActive:
		c = ui.ColorBlue
	case StatusPending:
		c = ui.ColorAmber
	default:
		c = ui.ColorGreen
	}
	return lipgloss.NewStyle().Foreground(c).Render("■")
}

func statusIcon(status InlineStatus) string {
	switch status.Worst {
	case StatusFailed:
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render("✗")
	case StatusAgentFailed:
		return lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("⚠")
	case StatusActive:
		if status.Releasing {
			return lipgloss.NewStyle().Foreground(ui.ColorBlue).Render("●") +
				" " + lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("releasing")
		}
		return lipgloss.NewStyle().Foreground(ui.ColorBlue).Render("●")
	case StatusPending:
		return lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("⧗")
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓")
	}
}

func progressBar(completed, total, barWidth int) string {
	if total == 0 {
		return strings.Repeat("░", barWidth)
	}
	filled := barWidth * completed / total
	if filled > barWidth {
		filled = barWidth
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
}

func padRight(s string, width int) string {
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

// ClearExpiredNewFlags clears NEW flags older than the given duration.
func ClearExpiredNewFlags(repos []RepoState, maxAge time.Duration) {
	now := time.Now()
	for i := range repos {
		if repos[i].NewFlag && !repos[i].LastNotableChange.IsZero() {
			if now.Sub(repos[i].LastNotableChange) > maxAge {
				repos[i].NewFlag = false
			}
		}
	}
}
