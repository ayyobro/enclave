package theme

import "github.com/charmbracelet/lipgloss"

var Light = Theme{
	Name: "light",

	Background:    lipgloss.Color("#fafafa"),
	Foreground:    lipgloss.Color("#383a42"),
	ForegroundDim: lipgloss.Color("#a0a1a7"),

	Primary:   lipgloss.Color("#4078f2"),
	Secondary: lipgloss.Color("#a626a4"),
	Accent:    lipgloss.Color("#0184bc"),

	Success: lipgloss.Color("#50a14f"),
	Warning: lipgloss.Color("#c18401"),
	Error:   lipgloss.Color("#e45649"),
	Info:    lipgloss.Color("#0184bc"),

	OwnName:     lipgloss.Color("#50a14f"),
	PeerName:    lipgloss.Color("#4078f2"),
	Timestamp:   lipgloss.Color("#a0a1a7"),
	UnreadBadge: lipgloss.Color("#a626a4"),

	CodeFg:      lipgloss.Color("#383a42"),
	CodeBg:      lipgloss.Color("#e5e5e6"),
	CodeBlockBg: lipgloss.Color("#eaeaeb"),

	Border:       lipgloss.Color("#d3d3d4"),
	BorderActive: lipgloss.Color("#4078f2"),
	StatusBar:    lipgloss.Color("#eaeaeb"),
	StatusBarFg:  lipgloss.Color("#383a42"),

	Online:  lipgloss.Color("#50a14f"),
	Offline: lipgloss.Color("#a0a1a7"),
}
