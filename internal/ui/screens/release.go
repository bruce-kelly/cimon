package screens

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/confidence"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// ReleaseModel tracks per-repo release status with confidence scoring.
type ReleaseModel struct {
	Repos       []string
	CurrentRepo int
	Runs        map[string][]models.WorkflowRun // repo -> release runs
	Confidence  map[string]confidence.ConfidenceResult
	Selector    components.Selector
	Width       int
	Height      int
}

func NewRelease() ReleaseModel {
	return ReleaseModel{
		Runs:       make(map[string][]models.WorkflowRun),
		Confidence: make(map[string]confidence.ConfidenceResult),
	}
}

func (r *ReleaseModel) NextRepo() {
	if len(r.Repos) == 0 {
		return
	}
	r.CurrentRepo = (r.CurrentRepo + 1) % len(r.Repos)
}

func (r *ReleaseModel) PrevRepo() {
	if len(r.Repos) == 0 {
		return
	}
	r.CurrentRepo = (r.CurrentRepo - 1 + len(r.Repos)) % len(r.Repos)
}

func (r *ReleaseModel) Render() string {
	if len(r.Repos) == 0 {
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  No release workflows configured")
	}

	repo := r.Repos[r.CurrentRepo]
	var sb strings.Builder

	// Repo header with navigation hint
	header := fmt.Sprintf("  %s  ← %d/%d →", repo, r.CurrentRepo+1, len(r.Repos))
	sb.WriteString(lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true).Render(header))
	sb.WriteString("\n\n")

	// Confidence score
	if conf, ok := r.Confidence[repo]; ok {
		sb.WriteString(r.renderConfidence(conf))
		sb.WriteString("\n\n")
	}

	// Release runs
	runs := r.Runs[repo]
	r.Selector.SetCount(len(runs))

	if len(runs) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  No release runs"))
	} else {
		for i, run := range runs {
			selected := i == r.Selector.Index()
			dot := ui.StatusDot(run.Conclusion)
			dotColor := ui.StatusColor(run.Conclusion)
			elapsed := components.FormatDuration(run.Elapsed())
			ago := components.FormatTimeAgo(run.UpdatedAt)

			line := fmt.Sprintf(" %s %s  %s  %s  %s",
				lipgloss.NewStyle().Foreground(dotColor).Render(dot),
				run.Name,
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.HeadBranch),
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed),
				lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(ago),
			)

			// Show jobs if available
			if len(run.Jobs) > 0 {
				for _, job := range run.Jobs {
					jobDot := ui.StatusDot(job.Conclusion)
					jobColor := ui.StatusColor(job.Conclusion)
					line += "\n   " + lipgloss.NewStyle().Foreground(jobColor).Render(jobDot) + " " + job.Name
				}
			}

			if selected {
				sb.WriteString(lipgloss.NewStyle().Background(ui.ColorSelection).Width(r.Width).Render(line))
			} else {
				sb.WriteString(line)
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (r *ReleaseModel) renderConfidence(conf confidence.ConfidenceResult) string {
	var sb strings.Builder

	// Score and level
	levelColor := ui.ColorGreen
	switch conf.Level {
	case confidence.LevelMedium:
		levelColor = ui.ColorAmber
	case confidence.LevelLow:
		levelColor = ui.ColorRed
	}

	sb.WriteString(fmt.Sprintf("  Confidence: %s %d/100\n",
		lipgloss.NewStyle().Foreground(levelColor).Bold(true).Render(string(conf.Level)),
		conf.Score,
	))

	// Signal breakdown
	for _, sig := range conf.Signals {
		bar := renderBar(sig.Points, sig.Max)
		sb.WriteString(fmt.Sprintf("  %s %s %d/%d %s\n",
			bar,
			sig.Name,
			sig.Points, sig.Max,
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(sig.Detail),
		))
	}

	return sb.String()
}

func renderBar(value, max int) string {
	width := 10
	filled := 0
	if max > 0 {
		filled = value * width / max
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return lipgloss.NewStyle().Foreground(ui.ColorBlue).Render(bar)
}
