package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"enclave/internal/theme"
)

// VibeStatus tracks the state of a collaborative coding session.
type VibeStatus int

const (
	VibeNone     VibeStatus = iota
	VibeIdle                // session active, waiting for prompts
	VibeRunning             // claude is processing
	VibePending             // prompt awaiting host approval
	VibeWaiting             // participant waiting for host to approve
)

// StatusBarModel renders the bottom status bar.
type StatusBarModel struct {
	connected   bool
	e2e         bool
	identity    string
	width       int
	vibeStatus  VibeStatus
	vibeRepo    string
	ephemeralOn bool
}

func NewStatusBarModel(identity string) StatusBarModel {
	return StatusBarModel{
		identity: identity,
		e2e:      true,
	}
}

func (m *StatusBarModel) SetConnected(c bool) {
	m.connected = c
}

func (m *StatusBarModel) SetWidth(w int) {
	m.width = w
}

func (m *StatusBarModel) SetVibeStatus(status VibeStatus, repo string) {
	m.vibeStatus = status
	if repo != "" {
		m.vibeRepo = repo
	}
}

func (m *StatusBarModel) SetEphemeral(on bool) {
	m.ephemeralOn = on
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

	// Build left section
	left := fmt.Sprintf(" %s │ %s", connStatus, e2eStatus)

	// Vibe indicator
	if m.vibeStatus != VibeNone {
		vibeStyle := lipgloss.NewStyle().Bold(true)
		var vibeText string
		switch m.vibeStatus {
		case VibeIdle:
			vibeText = vibeStyle.Foreground(t.Secondary).Render("🎸 vibe: " + m.vibeRepo)
		case VibeRunning:
			vibeText = vibeStyle.Foreground(t.Warning).Render("🎸 claude running...")
		case VibePending:
			vibeText = vibeStyle.Foreground(t.Warning).Render("🎸 prompt pending [y/n]")
		case VibeWaiting:
			vibeText = vibeStyle.Foreground(t.ForegroundDim).Render("🎸 awaiting approval...")
		}
		left += " │ " + vibeText
	}

	// Ephemeral indicator
	if m.ephemeralOn {
		left += " │ " + lipgloss.NewStyle().Foreground(t.Warning).Render("⏱ ephemeral")
	}

	// Keybinding hints
	hints := []string{
		styles.HelpKey.Render("↑↓") + styles.HelpDesc.Render(" navigate"),
		styles.HelpKey.Render("tab") + styles.HelpDesc.Render(" switch"),
		styles.HelpKey.Render("ctrl+↵") + styles.HelpDesc.Render(" newline"),
		styles.HelpKey.Render("?") + styles.HelpDesc.Render(" help"),
	}

	right := strings.Join(hints, "  ") + " "

	bar := lipgloss.NewStyle().
		Background(t.StatusBar).
		Foreground(t.StatusBarFg).
		Width(m.width)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 1
	}

	return bar.Render(left + strings.Repeat(" ", gap) + right)
}
