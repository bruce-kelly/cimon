package ui

import "charm.land/bubbles/v2/key"

// KeyMap defines all keybindings for the application.
type KeyMap struct {
	Screen1    key.Binding
	Screen2    key.Binding
	Screen3    key.Binding
	Screen4    key.Binding
	Quit       key.Binding
	Help       key.Binding
	Filter     key.Binding
	LogCycle   key.Binding
	Up         key.Binding
	Down       key.Binding
	Left       key.Binding
	Right      key.Binding
	Tab        key.Binding
	Enter      key.Binding
	Escape     key.Binding
	Approve    key.Binding
	Merge      key.Binding
	BatchMerge key.Binding
	Rerun      key.Binding
	ViewDiff   key.Binding
	Dismiss    key.Binding
	Open       key.Binding
	Dispatch   key.Binding
}

// Keys is the default key map for the application.
var Keys = KeyMap{
	Screen1:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "dashboard")),
	Screen2:    key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "timeline")),
	Screen3:    key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "release")),
	Screen4:    key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "metrics")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	LogCycle:   key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "log pane")),
	Up:         key.NewBinding(key.WithKeys("k", "w", "up")),
	Down:       key.NewBinding(key.WithKeys("j", "s", "down")),
	Left:       key.NewBinding(key.WithKeys("h", "a", "left")),
	Right:      key.NewBinding(key.WithKeys("d", "right")),
	Tab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "actions")),
	Escape:     key.NewBinding(key.WithKeys("esc")),
	Approve:    key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "approve")),
	Merge:      key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "merge")),
	BatchMerge: key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "batch merge")),
	Rerun:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rerun")),
	ViewDiff:   key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view diff")),
	Dismiss:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "dismiss")),
	Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "browser")),
	Dispatch:   key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "dispatch")),
}
