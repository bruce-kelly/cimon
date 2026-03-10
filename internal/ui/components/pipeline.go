package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
)

// PipelineView renders workflow runs as a vertical list.
type PipelineView struct {
	Runs          []models.WorkflowRun
	Selector      Selector
	Filter        *FilterBar
	ExpandJobs    bool
	KnownFailures map[string]bool // "repo:jobName" -> known
	TickEven      bool            // toggles pulsing dot animation
}

func NewPipelineView() *PipelineView {
	return &PipelineView{
		Filter:        &FilterBar{},
		KnownFailures: make(map[string]bool),
	}
}

func (p *PipelineView) SetRuns(runs []models.WorkflowRun) {
	p.Runs = runs
	p.Selector.SetCount(len(p.FilteredRuns()))
}

func (p *PipelineView) SelectedRun() *models.WorkflowRun {
	filtered := p.FilteredRuns()
	if len(filtered) == 0 {
		return nil
	}
	idx := p.Selector.Index()
	if idx >= len(filtered) {
		return nil
	}
	return &filtered[idx]
}

// FilteredRuns returns runs matching the current filter query.
func (p *PipelineView) FilteredRuns() []models.WorkflowRun {
	if p.Filter.Query == "" {
		return p.Runs
	}
	var filtered []models.WorkflowRun
	for _, r := range p.Runs {
		text := r.Name + " " + r.HeadBranch + " " + r.Actor + " " + r.Conclusion
		if p.Filter.Matches(text) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func (p *PipelineView) Render(width int) string {
	filtered := p.FilteredRuns()
	p.Selector.SetCount(len(filtered))

	if len(filtered) == 0 {
		return lipgloss.NewStyle().
			Foreground(ui.ColorMuted).
			Render("  No pipeline runs")
	}

	var sb strings.Builder
	for i, run := range filtered {
		selected := i == p.Selector.Index()
		sb.WriteString(p.renderRun(run, selected, width))
		sb.WriteString("\n")

		if p.ExpandJobs && len(run.Jobs) > 0 {
			for _, job := range run.Jobs {
				sb.WriteString(p.renderJob(run.Repo, job, width))
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

func (p *PipelineView) renderRun(run models.WorkflowRun, selected bool, width int) string {
	dot := ui.PulsingDot(run.Conclusion, p.TickEven)
	dotColor := ui.StatusColor(run.Conclusion)

	elapsed := FormatDuration(run.Elapsed())
	ago := FormatTimeAgo(run.UpdatedAt)

	// Short SHA
	sha := run.HeadSHA
	if len(sha) > 7 {
		sha = sha[:7]
	}

	// Job progress
	progress := RenderJobProgress(run.Jobs)

	// Failure hint: first failed step or job name
	failHint := ""
	if run.Conclusion == "failure" {
		for _, j := range run.Jobs {
			if j.Conclusion == "failure" {
				for _, s := range j.Steps {
					if s.Conclusion == "failure" {
						failHint = lipgloss.NewStyle().Foreground(ui.ColorRed).Render(" \u25b8 " + s.Name)
						break
					}
				}
				if failHint == "" {
					failHint = lipgloss.NewStyle().Foreground(ui.ColorRed).Render(" \u25b8 " + j.Name)
				}
				break
			}
		}
	}

	line := fmt.Sprintf(" %s %s  %s  %s  %s",
		lipgloss.NewStyle().Foreground(dotColor).Render(dot),
		run.Name,
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.HeadBranch),
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(sha),
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.Actor),
	)
	if progress != "" {
		line += "  " + progress
	}
	line += fmt.Sprintf("  %s  %s%s",
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
		lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(ago),
		failHint,
	)

	if selected {
		return lipgloss.NewStyle().
			Background(ui.ColorSelection).
			Width(width).
			Render(line)
	}
	return line
}

func (p *PipelineView) renderJob(repo string, job models.Job, width int) string {
	dot := ui.StatusDot(job.Conclusion)
	dotColor := ui.StatusColor(job.Conclusion)

	label := job.Name
	key := repo + ":" + job.Name
	if p.KnownFailures[key] {
		label += lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(" known")
	}

	return fmt.Sprintf("   %s %s",
		lipgloss.NewStyle().Foreground(dotColor).Render(dot),
		label,
	)
}

// FormatDuration renders a duration as a compact human-readable string.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// FormatTimeAbsolute renders a time as a wall-clock string.
func FormatTimeAbsolute(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return t.Format("15:04")
	}
	if y1 == y2 {
		return t.Format("Jan 2 15:04")
	}
	return t.Format("2006-01-02 15:04")
}

// FormatTimeAgo renders a time as a relative "ago" string.
func FormatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	}
}
