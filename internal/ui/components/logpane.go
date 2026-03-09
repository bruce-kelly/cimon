package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/ui"
)

type LogPaneMode int

const (
	LogPaneHidden LogPaneMode = iota
	LogPaneHalf
	LogPaneFull
)

type LineType int

const (
	LineContext LineType = iota
	LineAdded
	LineRemoved
	LineHunk
	LineHeader
)

func ClassifyLine(line string) LineType {
	switch {
	case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
		return LineHeader
	case strings.HasPrefix(line, "+"):
		return LineAdded
	case strings.HasPrefix(line, "-"):
		return LineRemoved
	case strings.HasPrefix(line, "@@"):
		return LineHunk
	case strings.HasPrefix(line, "diff "):
		return LineHeader
	default:
		return LineContext
	}
}

type LogPane struct {
	Mode      LogPaneMode
	Content   string
	Title     string
	IsLive    bool // streaming agent output
	ScrollPos int
}

func (l *LogPane) CycleMode() {
	l.Mode = (l.Mode + 1) % 3
}

func (l *LogPane) SetContent(title, content string, isLive bool) {
	l.Title = title
	l.Content = content
	l.IsLive = isLive
	l.ScrollPos = 0
}

func (l *LogPane) Clear() {
	l.Content = ""
	l.Title = ""
	l.IsLive = false
}

func (l *LogPane) Render(width, height int) string {
	if l.Mode == LogPaneHidden || height <= 0 {
		return ""
	}

	// Calculate pane height
	paneHeight := height / 2
	if l.Mode == LogPaneFull {
		paneHeight = height
	}
	if paneHeight < 3 {
		paneHeight = 3
	}

	// Header
	header := lipgloss.NewStyle().
		Background(ui.ColorSurface).
		Foreground(ui.ColorAccent).
		Width(width).
		Padding(0, 1).
		Render(l.renderHeader())

	// Content with diff highlighting
	lines := strings.Split(l.Content, "\n")
	contentHeight := paneHeight - 1 // minus header

	// Scroll to bottom for LIVE mode
	start := l.ScrollPos
	if l.IsLive {
		start = len(lines) - contentHeight
	}
	if start < 0 {
		start = 0
	}
	end := start + contentHeight
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	sb.WriteString(header + "\n")

	for i := start; i < end; i++ {
		line := lines[i]
		sb.WriteString(l.highlightLine(line))
		if i < end-1 {
			sb.WriteString("\n")
		}
	}

	// Pad remaining height
	rendered := end - start
	for i := rendered; i < contentHeight; i++ {
		sb.WriteString("\n")
	}

	return sb.String()
}

func (l *LogPane) renderHeader() string {
	title := l.Title
	if title == "" {
		title = "Log"
	}
	if l.IsLive {
		title += " [LIVE]"
	}
	return title
}

func (l *LogPane) highlightLine(line string) string {
	lt := ClassifyLine(line)
	var style lipgloss.Style

	switch lt {
	case LineAdded:
		style = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	case LineRemoved:
		style = lipgloss.NewStyle().Foreground(ui.ColorRed)
	case LineHunk:
		style = lipgloss.NewStyle().Foreground(ui.ColorBlue)
	case LineHeader:
		style = lipgloss.NewStyle().Foreground(ui.ColorPurple)
	default:
		style = lipgloss.NewStyle().Foreground(ui.ColorMuted)
	}

	return style.Render(line)
}
