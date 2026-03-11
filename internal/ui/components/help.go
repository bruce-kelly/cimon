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

	var bindings []struct{ key, desc string }
	switch viewName {
	case "compact":
		bindings = []struct{ key, desc string }{
			{"w/s", "Navigate up/down"},
			{"d/enter", "Drill into repo"},
			{"1", "Batch merge ready agent PRs"},
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
	case "detail":
		bindings = []struct{ key, desc string }{
			{"w/s", "Navigate up/down"},
			{"d/enter", "Drill into run/PR"},
			{"a/esc", "Back to repos"},
			{"1", "Rerun (run) / Approve (PR)"},
			{"2", "View diff / logs"},
			{"3", "Dismiss PR"},
			{"e", "Toggle log pane"},
			{"r", "Open on GitHub"},
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
	case "run-detail":
		bindings = []struct{ key, desc string }{
			{"w/s", "Navigate jobs"},
			{"d", "Expand/collapse job steps"},
			{"a/esc", "Back to repo"},
			{"1", "Rerun workflow"},
			{"2", "Rerun failed jobs"},
			{"e", "Toggle log pane"},
			{"r", "Open on GitHub"},
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
	case "pr-detail":
		bindings = []struct{ key, desc string }{
			{"w/s", "Navigate files"},
			{"d", "Jump to file diff"},
			{"a/esc", "Back to repo"},
			{"1", "Approve PR"},
			{"2", "Merge PR"},
			{"3", "Dismiss PR"},
			{"e", "Toggle log pane"},
			{"r", "Open on GitHub"},
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
	default:
		bindings = []struct{ key, desc string }{
			{"?", "Toggle help"},
			{"q", "Quit"},
		}
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
