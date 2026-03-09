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

func (h *HelpOverlay) Render(screenName string, width, height int) string {
	if !h.Visible {
		return ""
	}

	bindings := []struct{ key, desc string }{
		{"1/2/3/4", "Switch screen"},
		{"j/k", "Navigate"},
		{"Tab", "Cycle focus"},
		{"Enter", "Action menu"},
		{"r", "Smart rerun"},
		{"a", "Approve PR"},
		{"m", "Merge PR"},
		{"M", "Batch merge"},
		{"v", "View diff/output"},
		{"x", "Dismiss"},
		{"o", "Open in browser"},
		{"D", "Dispatch agent"},
		{"/", "Filter"},
		{"l", "Toggle log pane"},
		{"?", "This help"},
		{"q", "Quit"},
	}

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true).Render("Keybindings — " + screenName))
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
