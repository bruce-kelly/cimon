package screens

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/bruce-kelly/cimon/internal/ui"
	"github.com/bruce-kelly/cimon/internal/ui/components"
)

// TimelineModel shows cross-repo chronological feed.
type TimelineModel struct {
	Runs     []models.WorkflowRun
	Selector components.Selector
	Filter   *components.FilterBar
	Width    int
	Height   int
	// Map repo names to color indices
	repoColorIdx map[string]int
}

func NewTimeline() TimelineModel {
	return TimelineModel{
		Filter:       &components.FilterBar{},
		repoColorIdx: make(map[string]int),
	}
}

func (t *TimelineModel) SetRuns(runs []models.WorkflowRun) {
	t.Runs = runs
	// Sort by updated_at descending
	sort.Slice(t.Runs, func(i, j int) bool {
		return t.Runs[i].UpdatedAt.After(t.Runs[j].UpdatedAt)
	})
	// Assign repo colors
	seen := 0
	for _, r := range t.Runs {
		if _, ok := t.repoColorIdx[r.Repo]; !ok {
			t.repoColorIdx[r.Repo] = seen
			seen++
		}
	}
	t.Selector.SetCount(len(t.filteredRuns()))
}

func (t *TimelineModel) filteredRuns() []models.WorkflowRun {
	if t.Filter.Query == "" {
		return t.Runs
	}
	var filtered []models.WorkflowRun
	for _, r := range t.Runs {
		text := r.Name + " " + r.Repo + " " + r.HeadBranch + " " + r.Actor
		if t.Filter.Matches(text) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func (t *TimelineModel) Render() string {
	filtered := t.filteredRuns()
	t.Selector.SetCount(len(filtered))

	if len(filtered) == 0 {
		return lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("  No timeline events")
	}

	var sb strings.Builder
	for i, run := range filtered {
		selected := i == t.Selector.Index()
		dot := ui.StatusDot(run.Conclusion)
		dotColor := ui.StatusColor(run.Conclusion)

		// Shorten repo to just name
		repoName := run.Repo
		if idx := strings.LastIndex(repoName, "/"); idx >= 0 {
			repoName = repoName[idx+1:]
		}
		repoIdx := t.repoColorIdx[run.Repo]
		repoColor := ui.RepoColor(repoIdx)
		repoPrefix := lipgloss.NewStyle().Foreground(repoColor).Render(repoName)

		elapsed := components.FormatDuration(run.Elapsed())
		absTime := components.FormatTimeAbsolute(run.UpdatedAt)
		progress := components.RenderJobProgress(run.Jobs)

		line := fmt.Sprintf(" %s  %s %s %s  %s",
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(absTime),
			lipgloss.NewStyle().Foreground(dotColor).Render(dot),
			repoPrefix,
			run.Name,
			lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(run.Actor),
		)
		if progress != "" {
			line += "  " + progress
		}
		line += "  " + lipgloss.NewStyle().Foreground(ui.ColorMuted).Render(elapsed)

		if selected {
			sb.WriteString(lipgloss.NewStyle().Background(ui.ColorSelection).Width(t.Width).Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
