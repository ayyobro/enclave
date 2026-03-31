package theme

import "github.com/charmbracelet/lipgloss"

// Dark is the Tokyo Night-inspired dark theme.
var Dark = Theme{
	Name: "dark",

	Background:    lipgloss.Color("#1a1b26"),
	Foreground:    lipgloss.Color("#c0caf5"),
	ForegroundDim: lipgloss.Color("#565f89"),

	Primary:   lipgloss.Color("#7aa2f7"),
	Secondary: lipgloss.Color("#bb9af7"),
	Accent:    lipgloss.Color("#7dcfff"),

	Success: lipgloss.Color("#9ece6a"),
	Warning: lipgloss.Color("#e0af68"),
	Error:   lipgloss.Color("#f7768e"),
	Info:    lipgloss.Color("#7dcfff"),

	OwnName:     lipgloss.Color("#73daca"),
	PeerName:    lipgloss.Color("#7dcfff"),
	Timestamp:   lipgloss.Color("#565f89"),
	UnreadBadge: lipgloss.Color("#bb9af7"),

	CodeFg:      lipgloss.Color("#a9b1d6"),
	CodeBg:      lipgloss.Color("#292e42"),
	CodeBlockBg: lipgloss.Color("#24283b"),

	Border:       lipgloss.Color("#3b4261"),
	BorderActive: lipgloss.Color("#7aa2f7"),
	StatusBar:    lipgloss.Color("#1a1b26"),
	StatusBarFg:  lipgloss.Color("#a9b1d6"),

	Online:  lipgloss.Color("#9ece6a"),
	Offline: lipgloss.Color("#565f89"),
}
