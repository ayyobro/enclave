package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"enclave/internal/theme"
)

// StatusBarModel renders the bottom status bar.
type StatusBarModel struct {
	connected bool
	e2e       bool
	identity  string
	width     int
}

func NewStatusBarModel(identity string) StatusBarModel {
	return StatusBarModel{
		identity: identity,
		e2e:      true, // always true when connected
	}
}

func (m *StatusBarModel) SetConnected(c bool) {
	m.connected = c
}

func (m *StatusBarModel) SetWidth(w int) {
	m.width = w
}

func (m StatusBarModel) View() string {
	styles := theme.NewStyles(theme.Current)
	t := theme.Current

	// Connection status
	var connStatus string
	if m.connected {
		connStatus = styles.StatusConnected.Render("● Connected")
	} else {
		connStatus = styles.StatusDisconnected.Render("● Disconnected")
	}

	// E2E status
	e2eStatus := styles.StatusE2E.Render("E2E ✓")

	// Keybinding hints
	hints := []string{
		styles.HelpKey.Render("↑↓") + styles.HelpDesc.Render(" navigate"),
		styles.HelpKey.Render("tab") + styles.HelpDesc.Render(" switch"),
		styles.HelpKey.Render("ctrl+↵") + styles.HelpDesc.Render(" newline"),
		styles.HelpKey.Render("?") + styles.HelpDesc.Render(" help"),
	}

	left := fmt.Sprintf(" %s │ %s", connStatus, e2eStatus)
	right := strings.Join(hints, "  ") + " "

	// Calculate space for padding
	bar := lipgloss.NewStyle().
		Background(t.StatusBar).
		Foreground(t.StatusBarFg).
		Width(m.width)

	// Create the bar with left and right sections
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 1
	}

	return bar.Render(left + strings.Repeat(" ", gap) + right)
}
