package theme

import "github.com/charmbracelet/lipgloss"

// Theme defines all colors used throughout the TUI.
type Theme struct {
	Name string

	// Base colors
	Background    lipgloss.Color
	Foreground    lipgloss.Color
	ForegroundDim lipgloss.Color

	// Accent colors
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color

	// Semantic colors
	Success lipgloss.Color
	Warning lipgloss.Color
	Error   lipgloss.Color
	Info    lipgloss.Color

	// Chat-specific
	OwnName     lipgloss.Color
	PeerName    lipgloss.Color
	Timestamp   lipgloss.Color
	UnreadBadge lipgloss.Color

	// Code rendering
	CodeFg      lipgloss.Color
	CodeBg      lipgloss.Color
	CodeBlockBg lipgloss.Color

	// UI elements
	Border       lipgloss.Color
	BorderActive lipgloss.Color
	StatusBar    lipgloss.Color
	StatusBarFg  lipgloss.Color

	// Online/offline
	Online  lipgloss.Color
	Offline lipgloss.Color
}

// Current is the active theme.
var Current = Dark

// Available returns all theme names.
func Available() []string {
	return []string{"dark", "light", "dracula", "nord"}
}

// Get returns a theme by name, or Dark if not found.
func Get(name string) Theme {
	switch name {
	case "dark":
		return Dark
	case "light":
		return Light
	case "dracula":
		return Dracula
	case "nord":
		return Nord
	default:
		return Dark
	}
}

// Set sets the current theme by name.
func Set(name string) {
	Current = Get(name)
}

// Styles provides pre-built lipgloss styles from the current theme.
type Styles struct {
	// Sidebar
	SidebarBorder    lipgloss.Style
	ContactName      lipgloss.Style
	ContactSelected  lipgloss.Style
	UnreadBadge      lipgloss.Style
	OnlineIndicator  lipgloss.Style
	OfflineIndicator lipgloss.Style

	// Chat
	OwnNameStyle   lipgloss.Style
	PeerNameStyle  lipgloss.Style
	TimestampStyle lipgloss.Style
	MessageBody    lipgloss.Style
	InlineCode     lipgloss.Style
	CodeBlock      lipgloss.Style

	// Status bar
	StatusBarStyle     lipgloss.Style
	StatusConnected    lipgloss.Style
	StatusDisconnected lipgloss.Style
	StatusE2E          lipgloss.Style

	// General
	Title    lipgloss.Style
	Subtle   lipgloss.Style
	ErrorMsg lipgloss.Style
	HelpKey  lipgloss.Style
	HelpDesc lipgloss.Style
}

// NewStyles creates lipgloss styles from a theme.
func NewStyles(t Theme) Styles {
	return Styles{
		SidebarBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(t.Border),

		ContactName: lipgloss.NewStyle().
			Foreground(t.Foreground),

		ContactSelected: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),

		UnreadBadge: lipgloss.NewStyle().
			Foreground(t.UnreadBadge).
			Bold(true),

		OnlineIndicator: lipgloss.NewStyle().
			Foreground(t.Online),

		OfflineIndicator: lipgloss.NewStyle().
			Foreground(t.Offline),

		OwnNameStyle: lipgloss.NewStyle().
			Foreground(t.OwnName).
			Bold(true),

		PeerNameStyle: lipgloss.NewStyle().
			Foreground(t.PeerName).
			Bold(true),

		TimestampStyle: lipgloss.NewStyle().
			Foreground(t.Timestamp),

		MessageBody: lipgloss.NewStyle().
			Foreground(t.Foreground),

		InlineCode: lipgloss.NewStyle().
			Foreground(t.CodeFg).
			Background(t.CodeBg),

		CodeBlock: lipgloss.NewStyle().
			Foreground(t.CodeFg).
			Background(t.CodeBlockBg),

		StatusBarStyle: lipgloss.NewStyle().
			Background(t.StatusBar).
			Foreground(t.StatusBarFg).
			Padding(0, 1),

		StatusConnected: lipgloss.NewStyle().
			Foreground(t.Success).
			Bold(true),

		StatusDisconnected: lipgloss.NewStyle().
			Foreground(t.Error).
			Bold(true),

		StatusE2E: lipgloss.NewStyle().
			Foreground(t.Success),

		Title: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),

		Subtle: lipgloss.NewStyle().
			Foreground(t.ForegroundDim),

		ErrorMsg: lipgloss.NewStyle().
			Foreground(t.Error),

		HelpKey: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),

		HelpDesc: lipgloss.NewStyle().
			Foreground(t.ForegroundDim),
	}
}
