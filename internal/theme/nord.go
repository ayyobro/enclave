package theme

import "github.com/charmbracelet/lipgloss"

var Nord = Theme{
	Name: "nord",

	Background:    lipgloss.Color("#2e3440"),
	Foreground:    lipgloss.Color("#d8dee9"),
	ForegroundDim: lipgloss.Color("#4c566a"),

	Primary:   lipgloss.Color("#88c0d0"),
	Secondary: lipgloss.Color("#b48ead"),
	Accent:    lipgloss.Color("#81a1c1"),

	Success: lipgloss.Color("#a3be8c"),
	Warning: lipgloss.Color("#ebcb8b"),
	Error:   lipgloss.Color("#bf616a"),
	Info:    lipgloss.Color("#81a1c1"),

	OwnName:     lipgloss.Color("#a3be8c"),
	PeerName:    lipgloss.Color("#88c0d0"),
	Timestamp:   lipgloss.Color("#4c566a"),
	UnreadBadge: lipgloss.Color("#b48ead"),

	CodeFg:      lipgloss.Color("#d8dee9"),
	CodeBg:      lipgloss.Color("#3b4252"),
	CodeBlockBg: lipgloss.Color("#363c4a"),

	Border:       lipgloss.Color("#3b4252"),
	BorderActive: lipgloss.Color("#88c0d0"),
	StatusBar:    lipgloss.Color("#2e3440"),
	StatusBarFg:  lipgloss.Color("#d8dee9"),

	Online:  lipgloss.Color("#a3be8c"),
	Offline: lipgloss.Color("#4c566a"),
}
