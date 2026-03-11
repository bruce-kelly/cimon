package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/ui"
)

// HelpOverlay shows keybinding help.
type HelpOverlay struct {
	Visible bool
}

func (h *HelpOverlay) Toggle() {
	h.Visible = !h.Visible
}

func (h *HelpOverlay) Render(viewName string, width, height int) string {
	if !h.Visible {
		return ""
	}

	bindings := []struct{ key, desc string }{
		{"w/k", "Navigate up"},
		{"s/j", "Navigate down"},
		{"Enter", "Drill in / select"},
		{"Esc", "Back / close log pane"},
		{"r", "Rerun (on runs)"},
		{"A", "Approve PR"},
		{"m", "Merge PR"},
		{"M", "Batch merge ready agent PRs"},
		{"x", "Dismiss PR"},
		{"v", "View diff / logs"},
		{"o", "Open in browser"},
		{"l", "Toggle log pane"},
		{"?", "Toggle help"},
		{"q", "Quit"},
	}

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true).Render("Keybindings — " + viewName))
	sb.WriteString("\n\n")

	for _, b := range bindings {
		sb.WriteString("  ")
		sb.WriteString(lipgloss.NewStyle().Foreground(ui.ColorBlue).Width(12).Render(b.key))
		sb.WriteString(lipgloss.NewStyle().Foreground(ui.ColorFg).Render(b.desc))
		sb.WriteString("\n")
	}

	content := sb.String()
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorBorder).
		Padding(1, 2)

	return boxStyle.Render(content)
}
