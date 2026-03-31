package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const sidebarWidth = 24

// MainModel composes the split-pane chat layout.
type MainModel struct {
	sidebar   SidebarModel
	chatView  ChatViewModel
	input     InputModel
	statusBar StatusBarModel

	focus     FocusPane
	width     int
	height    int

	activeContact string // pubkey of selected contact
	disconnected  bool
}

func NewMainModel(identity string) MainModel {
	return MainModel{
		sidebar:   NewSidebarModel(),
		chatView:  NewChatViewModel(),
		input:     NewInputModel(),
		statusBar: NewStatusBarModel(identity),
		focus:     FocusSidebar,
	}
}

func (m *MainModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.layout()
}

func (m *MainModel) SetConnected(c bool) {
	m.statusBar.SetConnected(c)
	if c {
		m.disconnected = false
		m.input.SetDisabled(false)
	}
}

func (m *MainModel) SetDisconnected() {
	m.disconnected = true
	m.input.SetDisabled(true)
	m.chatView.AddSystemMessage("Connection to server lost. Press Ctrl+C to exit.")
}

func (m *MainModel) ShowError(msg string) {
	m.chatView.AddSystemMessage(msg)
}

func (m *MainModel) SetContacts(contacts []ContactInfo) {
	m.sidebar.SetContacts(contacts)
	// Auto-select first contact
	if len(contacts) > 0 && m.activeContact == "" {
		m.activeContact = contacts[0].PublicKey
		m.chatView.SetPeer(contacts[0].DisplayName, contacts[0].PublicKey)
	}
}

func (m *MainModel) AddIncomingMessage(from, fromName, text string, ts time.Time) {
	if from == m.activeContact {
		m.chatView.AddMessage(ChatMessage{
			From:      from,
			FromName:  fromName,
			Text:      text,
			Timestamp: ts,
			IsOwn:     false,
		})
		m.sidebar.ClearUnread(from)
	} else {
		m.sidebar.IncrementUnread(from)
	}
}

func (m *MainModel) AddOwnMessage(to, text string, ts time.Time) {
	if to == m.activeContact {
		m.chatView.AddMessage(ChatMessage{
			Text:      text,
			Timestamp: ts,
			IsOwn:     true,
		})
	}
}

func (m *MainModel) UpdatePresence(pubKey string, online bool) {
	m.sidebar.UpdatePresence(pubKey, online)
}

func (m *MainModel) SetTyping(from string) {
	if from == m.activeContact {
		m.chatView.SetTyping(from)
	}
}

func (m *MainModel) ActiveContact() string {
	return m.activeContact
}

func (m *MainModel) layout() {
	chatWidth := m.width - sidebarWidth - 1 // 1 for gap
	if chatWidth < 20 {
		chatWidth = 20
	}

	statusHeight := 1
	inputHeight := 3
	chatHeight := m.height - inputHeight - statusHeight - 2 // borders

	m.sidebar.SetSize(sidebarWidth, m.height-statusHeight)
	m.chatView.SetSize(chatWidth, chatHeight)
	m.input.SetWidth(chatWidth)
	m.statusBar.SetWidth(m.width)

	m.updateFocus()
}

func (m *MainModel) updateFocus() {
	m.sidebar.SetFocused(m.focus == FocusSidebar)
	m.chatView.SetFocused(m.focus == FocusChat)
	m.input.SetFocused(m.focus == FocusInput)
}

func (m *MainModel) cycleFocus() {
	switch m.focus {
	case FocusSidebar:
		m.focus = FocusInput
	case FocusInput:
		m.focus = FocusChat
	case FocusChat:
		m.focus = FocusSidebar
	}
	m.updateFocus()
}

func (m MainModel) Update(msg tea.Msg) (MainModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.cycleFocus()
			return m, nil
		}

	case SelectContactMsg:
		m.activeContact = msg.PublicKey
		m.chatView.SetPeer(msg.DisplayName, msg.PublicKey)
		m.sidebar.ClearUnread(msg.PublicKey)
		m.focus = FocusInput
		m.updateFocus()
		return m, nil
	}

	// Update focused component
	var cmd tea.Cmd
	switch m.focus {
	case FocusSidebar:
		m.sidebar, cmd = m.sidebar.Update(msg)
		cmds = append(cmds, cmd)
	case FocusChat:
		m.chatView, cmd = m.chatView.Update(msg)
		cmds = append(cmds, cmd)
	case FocusInput:
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m MainModel) View() string {
	// Right pane: chat + input stacked vertically
	rightPane := lipgloss.JoinVertical(lipgloss.Left,
		m.chatView.View(),
		m.input.View(),
	)

	// Main split: sidebar | right pane
	main := lipgloss.JoinHorizontal(lipgloss.Top,
		m.sidebar.View(),
		rightPane,
	)

	// Full layout: main + status bar
	return lipgloss.JoinVertical(lipgloss.Left,
		main,
		m.statusBar.View(),
	)
}
