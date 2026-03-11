package ui

import "charm.land/bubbles/v2/key"

// KeyMap defines all keybindings for v2's two-view model.
type KeyMap struct {
	// Navigation
	Up, Down, Enter, Escape key.Binding

	// Compact view
	BatchMerge key.Binding

	// Detail view actions
	Rerun, Approve, Merge, Dismiss key.Binding
	ViewDiff, Open                 key.Binding

	// Shared
	LogCycle, Help, Quit key.Binding

	// Deprecated: kept for v1 app.go compatibility during migration.
	// Remove in Chunk 5 cleanup.
	Screen1, Screen2, Screen3, Screen4 key.Binding
	Filter, Left, Right, Tab          key.Binding
	Dispatch                           key.Binding
}

// Keys is the global keybinding configuration.
var Keys = KeyMap{
	Up:         key.NewBinding(key.WithKeys("w", "k", "up"), key.WithHelp("w/k/↑", "up")),
	Down:       key.NewBinding(key.WithKeys("s", "j", "down"), key.WithHelp("s/j/↓", "down")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Escape:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	BatchMerge: key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "batch merge")),
	Rerun:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rerun")),
	Approve:    key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "approve")),
	Merge:      key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "merge")),
	Dismiss:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "dismiss")),
	ViewDiff:   key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view diff/logs")),
	Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in browser")),
	LogCycle:   key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "toggle log pane")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

	// Deprecated bindings — used by v1 app.go, removed in cleanup
	Screen1:  key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "dashboard")),
	Screen2:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "timeline")),
	Screen3:  key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "release")),
	Screen4:  key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "metrics")),
	Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Left:     key.NewBinding(key.WithKeys("h", "a", "left")),
	Right:    key.NewBinding(key.WithKeys("d", "right")),
	Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus")),
	Dispatch: key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "dispatch")),
}
