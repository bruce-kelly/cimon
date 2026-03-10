package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

var (
	ColorBg        = lipgloss.Color("#1a1b26")
	ColorFg        = lipgloss.Color("#c0caf5")
	ColorMuted     = lipgloss.Color("#565f89")
	ColorAccent    = lipgloss.Color("#e0af68")
	ColorGreen     = lipgloss.Color("#9ece6a")
	ColorRed       = lipgloss.Color("#f7768e")
	ColorAmber     = lipgloss.Color("#ff9e64")
	ColorBlue      = lipgloss.Color("#7aa2f7")
	ColorPurple    = lipgloss.Color("#bb9af7")
	ColorBorder    = lipgloss.Color("#3b4261")
	ColorSurface   = lipgloss.Color("#24283b")
	ColorSelection = lipgloss.Color("#364a82")
)

var RepoColors = []color.Color{
	ColorBlue, ColorPurple, ColorGreen, ColorAmber, ColorAccent,
}

func RepoColor(index int) color.Color {
	return RepoColors[index%len(RepoColors)]
}

func StatusColor(conclusion string) color.Color {
	switch conclusion {
	case "success":
		return ColorGreen
	case "failure":
		return ColorRed
	case "cancelled":
		return ColorMuted
	default:
		return ColorAccent
	}
}

func StatusDot(conclusion string) string {
	switch conclusion {
	case "success":
		return "●"
	case "failure":
		return "✗"
	case "cancelled":
		return "○"
	case "":
		return "◌"
	default:
		return "?"
	}
}

// PulsingDot returns an alternating dot for in-progress runs.
// Bright phase shows ●, dim phase shows ◌.
func PulsingDot(conclusion string, tickEven bool) string {
	if conclusion == "" {
		if tickEven {
			return "◌"
		}
		return "●"
	}
	return StatusDot(conclusion)
}
