package ui

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
