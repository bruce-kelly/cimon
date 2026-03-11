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

