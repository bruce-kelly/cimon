package ui

import "charm.land/bubbles/v2/key"

// KeyMap defines all keybindings for WASD + numbered action scheme.
type KeyMap struct {
	// Navigation
	Up, Down, DrillIn, Back key.Binding

	// Context-sensitive actions
	Action1, Action2, Action3 key.Binding

	// Fixed actions
	Examine, Remote, Help, Quit key.Binding
}

// Keys is the global keybinding configuration.
var Keys = KeyMap{
	Up:      key.NewBinding(key.WithKeys("w", "up"), key.WithHelp("w/↑", "up")),
	Down:    key.NewBinding(key.WithKeys("s", "down"), key.WithHelp("s/↓", "down")),
	DrillIn: key.NewBinding(key.WithKeys("d", "enter"), key.WithHelp("d/enter", "select")),
	Back:    key.NewBinding(key.WithKeys("a", "esc"), key.WithHelp("a/esc", "back")),
	Action1: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "action 1")),
	Action2: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "action 2")),
	Action3: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "action 3")),
	Examine: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "examine")),
	Remote:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "github")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
