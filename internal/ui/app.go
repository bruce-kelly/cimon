package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/bruce-kelly/cimon/internal/models"
)

// Screen identifies which screen is currently active.
type Screen int

const (
	ScreenDashboard Screen = iota
	ScreenTimeline
	ScreenRelease
	ScreenMetrics
)

func (s Screen) String() string {
	switch s {
	case ScreenTimeline:
		return "timeline"
	case ScreenRelease:
		return "release"
	case ScreenMetrics:
		return "metrics"
	default:
		return "dashboard"
	}
}

// PollResultMsg is sent from the poller goroutine.
type PollResultMsg struct {
	Result models.PollResult
}

// App is the root Bubbletea model.
type App struct {
	screen   Screen
	width    int
	height   int
	quitting bool
	// Placeholder for future screen models
	statusText string
}

// NewApp creates a new App with default state.
func NewApp() App {
	return App{
		screen:     ScreenDashboard,
		statusText: "cimon — ready",
	}
}

func (a App) Init() tea.Cmd {
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, Keys.Quit):
			a.quitting = true
			return a, tea.Quit
		case key.Matches(msg, Keys.Screen1):
			a.screen = ScreenDashboard
		case key.Matches(msg, Keys.Screen2):
			a.screen = ScreenTimeline
		case key.Matches(msg, Keys.Screen3):
			a.screen = ScreenRelease
		case key.Matches(msg, Keys.Screen4):
			a.screen = ScreenMetrics
		}
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
	}
	return a, nil
}

func (a App) View() tea.View {
	if a.quitting {
		return tea.NewView("")
	}

	// Screen content placeholder
	content := lipgloss.NewStyle().
		Foreground(ColorFg).
		Render("  " + a.screen.String() + " screen")

	// Status bar
	statusStyle := lipgloss.NewStyle().
		Background(ColorSurface).
		Foreground(ColorMuted).
		Padding(0, 1).
		Width(a.width)

	screenIndicator := ""
	screens := []struct {
		num    string
		name   string
		active bool
	}{
		{"1", "dashboard", a.screen == ScreenDashboard},
		{"2", "timeline", a.screen == ScreenTimeline},
		{"3", "release", a.screen == ScreenRelease},
		{"4", "metrics", a.screen == ScreenMetrics},
	}
	for _, s := range screens {
		if s.active {
			screenIndicator += lipgloss.NewStyle().Foreground(ColorAccent).Render("["+s.num+"]"+s.name) + " "
		} else {
			screenIndicator += lipgloss.NewStyle().Foreground(ColorMuted).Render(" "+s.num+" "+s.name) + " "
		}
	}

	statusBar := statusStyle.Render(screenIndicator)

	// Fill remaining height
	contentHeight := a.height - 1
	if contentHeight < 0 {
		contentHeight = 0
	}
	contentStyle := lipgloss.NewStyle().
		Width(a.width).
		Height(contentHeight).
		Background(ColorBg)

	rendered := contentStyle.Render(content) + "\n" + statusBar

	v := tea.NewView(rendered)
	v.AltScreen = true
	return v
}
