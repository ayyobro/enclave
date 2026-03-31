package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"enclave/internal/theme"
)

// ConnectModel shows a spinner while connecting to the server.
type ConnectModel struct {
	spinner spinner.Model
	status  string
	err     error
	width   int
	height  int
}

func NewConnectModel() ConnectModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.Current.Primary)
	return ConnectModel{
		spinner: s,
		status:  "Connecting...",
	}
}

func (m ConnectModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m ConnectModel) Update(msg tea.Msg) (ConnectModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case ReconnectingMsg:
		m.status = fmt.Sprintf("Reconnecting (attempt %d)...", msg.Attempt)
	case TUIErrorMsg:
		m.err = msg.Err
		m.status = "Connection failed"
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m ConnectModel) View() string {
	styles := theme.NewStyles(theme.Current)

	logo := styles.Title.Render(`
  ╔═╗╔╗╔╔═╗╦  ╔═╗╦  ╦╔═╗
  ║╣ ║║║║  ║  ╠═╣╚╗╔╝║╣
  ╚═╝╝╚╝╚═╝╩═╝╩ ╩ ╚╝ ╚═╝`)

	subtitle := styles.Subtle.Render("Secure. On-Prem. Yours.")

	var statusLine string
	if m.err != nil {
		statusLine = styles.ErrorMsg.Render(fmt.Sprintf("✗ %s: %v", m.status, m.err))
	} else {
		statusLine = fmt.Sprintf("%s %s", m.spinner.View(), m.status)
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		"",
		logo,
		"",
		subtitle,
		"",
		statusLine,
		"",
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
