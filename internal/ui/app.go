package ui

// ViewMode represents which view is active in the TUI.
type ViewMode int

const (
	ViewCompact ViewMode = iota
	ViewDetail
)

func (v ViewMode) String() string {
	switch v {
	case ViewCompact:
		return "compact"
	case ViewDetail:
		return "detail"
	default:
		return "unknown"
	}
}

// Deprecated: Screen is kept for v1 compatibility during migration.
// Remove in Chunk 5 cleanup.
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
