package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/ui"
)

// CatchupOverlay shows a summary of changes while the user was away.
type CatchupOverlay struct {
	Visible    bool
	NewRuns    int
	NewTasks   int
	ChangedPRs int
}

func (c *CatchupOverlay) Show(newRuns, newTasks, changedPRs int) {
	if newRuns == 0 && newTasks == 0 && changedPRs == 0 {
		return // nothing to show
	}
	c.Visible = true
	c.NewRuns = newRuns
	c.NewTasks = newTasks
	c.ChangedPRs = changedPRs
}

func (c *CatchupOverlay) Dismiss() {
	c.Visible = false
}

func (c *CatchupOverlay) Render(width, height int) string {
	if !c.Visible {
		return ""
	}

	var lines []string
	lines = append(lines, "While you were away:")
	lines = append(lines, "")
	if c.NewRuns > 0 {
		lines = append(lines, fmt.Sprintf("  %d new CI runs", c.NewRuns))
	}
	if c.NewTasks > 0 {
		lines = append(lines, fmt.Sprintf("  %d agent tasks completed", c.NewTasks))
	}
	if c.ChangedPRs > 0 {
		lines = append(lines, fmt.Sprintf("  %d PRs changed", c.ChangedPRs))
	}
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("Press any key to dismiss"))

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorAccent).
		Foreground(ui.ColorFg).
		Padding(1, 3).
		Width(40)

	box := boxStyle.Render(content)

	// Center the box
	boxWidth := 40
	boxHeight := len(lines) + 4
	padLeft := (width - boxWidth) / 2
	padTop := (height - boxHeight) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	if padTop < 0 {
		padTop = 0
	}

	var sb strings.Builder
	for i := 0; i < padTop; i++ {
		sb.WriteString("\n")
	}
	for _, line := range strings.Split(box, "\n") {
		sb.WriteString(strings.Repeat(" ", padLeft) + line + "\n")
	}

	return sb.String()
}
