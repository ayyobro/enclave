package theme

import "github.com/charmbracelet/lipgloss"

var Dracula = Theme{
	Name: "dracula",

	Background:    lipgloss.Color("#282a36"),
	Foreground:    lipgloss.Color("#f8f8f2"),
	ForegroundDim: lipgloss.Color("#6272a4"),

	Primary:   lipgloss.Color("#bd93f9"),
	Secondary: lipgloss.Color("#ff79c6"),
	Accent:    lipgloss.Color("#8be9fd"),

	Success: lipgloss.Color("#50fa7b"),
	Warning: lipgloss.Color("#f1fa8c"),
	Error:   lipgloss.Color("#ff5555"),
	Info:    lipgloss.Color("#8be9fd"),

	OwnName:     lipgloss.Color("#50fa7b"),
	PeerName:    lipgloss.Color("#8be9fd"),
	Timestamp:   lipgloss.Color("#6272a4"),
	UnreadBadge: lipgloss.Color("#ff79c6"),

	CodeFg:      lipgloss.Color("#f8f8f2"),
	CodeBg:      lipgloss.Color("#44475a"),
	CodeBlockBg: lipgloss.Color("#383a4a"),

	Border:       lipgloss.Color("#44475a"),
	BorderActive: lipgloss.Color("#bd93f9"),
	StatusBar:    lipgloss.Color("#21222c"),
	StatusBarFg:  lipgloss.Color("#f8f8f2"),

	Online:  lipgloss.Color("#50fa7b"),
	Offline: lipgloss.Color("#6272a4"),
}
