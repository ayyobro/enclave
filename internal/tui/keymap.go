package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keybindings for the TUI.
type KeyMap struct {
	Quit     key.Binding
	Tab      key.Binding
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Slash    key.Binding
	Help     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Escape   key.Binding
}

// DefaultKeyMap returns the standard keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch pane"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select/send"),
		),
		Slash: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "commands"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "scroll up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "scroll down"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
	}
}
