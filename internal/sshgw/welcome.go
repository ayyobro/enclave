package sshgw

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"enclave/internal/theme"
)

// SSHWelcomeModel is shown to users who connect via SSH gateway.
// It provides onboarding information and basic server status.
type SSHWelcomeModel struct {
	userName  string
	pubKeyB64 string
	wsAddr    string
	width     int
	height    int
}

func NewSSHWelcomeModel(userName, pubKeyB64, wsAddr string) SSHWelcomeModel {
	return SSHWelcomeModel{
		userName:  userName,
		pubKeyB64: pubKeyB64,
		wsAddr:    wsAddr,
	}
}

func (m SSHWelcomeModel) Init() tea.Cmd {
	return nil
}

func (m SSHWelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m SSHWelcomeModel) View() string {
	t := theme.Current
	s := theme.NewStyles(t)

	title := s.Title.Render(`
  ╔═╗╔╗╔╔═╗╦  ╔═╗╦  ╦╔═╗
  ║╣ ║║║║  ║  ╠═╣╚╗╔╝║╣
  ╚═╝╝╚╝╚═╝╩═╝╩ ╩ ╚╝ ╚═╝`)

	subtitle := s.Subtle.Render("Secure. On-Prem. Yours.")

	var lines []string
	lines = append(lines, "")
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, subtitle)
	lines = append(lines, "")
	lines = append(lines, "")

	labelStyle := lipgloss.NewStyle().Foreground(t.ForegroundDim).Width(18)
	valueStyle := lipgloss.NewStyle().Foreground(t.Foreground)

	lines = append(lines, fmt.Sprintf("  %s%s", labelStyle.Render("Connected as:"), valueStyle.Render(m.userName)))

	if m.pubKeyB64 != "" {
		short := m.pubKeyB64
		if len(short) > 30 {
			short = short[:30] + "..."
		}
		lines = append(lines, fmt.Sprintf("  %s%s", labelStyle.Render("SSH key:"), valueStyle.Render(short)))
	}

	lines = append(lines, fmt.Sprintf("  %s%s", labelStyle.Render("Server:"), valueStyle.Render(m.wsAddr)))
	lines = append(lines, "")
	lines = append(lines, "")

	infoStyle := lipgloss.NewStyle().Foreground(t.Primary)
	lines = append(lines, infoStyle.Render("  To start chatting, install the Enclave CLI:"))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("    1. Download the enclave binary for your platform"))
	lines = append(lines, fmt.Sprintf("    2. Run: enclave init --display-name %s --server %s --token <invite>", m.userName, m.wsAddr))
	lines = append(lines, fmt.Sprintf("    3. Run: enclave chat"))
	lines = append(lines, "")
	lines = append(lines, s.Subtle.Render("  Full TUI chat over SSH is coming in a future release."))
	lines = append(lines, "")
	lines = append(lines, s.Subtle.Render("  Press 'q' to disconnect."))

	content := strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
