package ui

// ViewMode represents which view is active in the TUI.
type ViewMode int

const (
	ViewCompact ViewMode = iota
	ViewDetail
	ViewRunDetail
	ViewPRDetail
)

func (v ViewMode) String() string {
	switch v {
	case ViewCompact:
		return "compact"
	case ViewDetail:
		return "detail"
	case ViewRunDetail:
		return "run-detail"
	case ViewPRDetail:
		return "pr-detail"
	default:
		return "unknown"
	}
}
