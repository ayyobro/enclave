package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"enclave/internal/theme"
)

// SidebarModel renders the contact list.
type SidebarModel struct {
	contacts []ContactInfo
	selected int
	focused  bool
	width    int
	height   int
}

func NewSidebarModel() SidebarModel {
	return SidebarModel{
		selected: 0,
	}
}

func (m *SidebarModel) SetContacts(contacts []ContactInfo) {
	m.contacts = contacts
}

func (m *SidebarModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *SidebarModel) SetFocused(f bool) {
	m.focused = f
}

func (m *SidebarModel) SelectedContact() *ContactInfo {
	if len(m.contacts) == 0 || m.selected >= len(m.contacts) {
		return nil
	}
	return &m.contacts[m.selected]
}

func (m *SidebarModel) UpdatePresence(pubKey string, online bool) {
	for i := range m.contacts {
		if m.contacts[i].PublicKey == pubKey {
			m.contacts[i].Online = online
			break
		}
	}
}

func (m *SidebarModel) IncrementUnread(pubKey string) {
	for i := range m.contacts {
		if m.contacts[i].PublicKey == pubKey {
			m.contacts[i].UnreadCount++
			break
		}
	}
}

func (m *SidebarModel) ClearUnread(pubKey string) {
	for i := range m.contacts {
		if m.contacts[i].PublicKey == pubKey {
			m.contacts[i].UnreadCount = 0
			break
		}
	}
}

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.contacts)-1 {
				m.selected++
			}
		case "enter":
			if c := m.SelectedContact(); c != nil {
				return m, func() tea.Msg {
					return SelectContactMsg{
						PublicKey:   c.PublicKey,
						DisplayName: c.DisplayName,
					}
				}
			}
		}
	}

	return m, nil
}

func (m SidebarModel) View() string {
	styles := theme.NewStyles(theme.Current)
	t := theme.Current

	borderColor := t.Border
	if m.focused {
		borderColor = t.BorderActive
	}

	// Build contact list
	var lines []string
	maxItems := m.height - 4 // account for border + title + footer
	if maxItems < 1 {
		maxItems = 1
	}

	for i, c := range m.contacts {
		if i >= maxItems {
			break
		}

		// Online indicator (groups show # instead)
		var indicator string
		if c.IsGroup {
			indicator = lipgloss.NewStyle().Foreground(t.Secondary).Render("#")
		} else if c.Online {
			indicator = styles.OnlineIndicator.Render("●")
		} else {
			indicator = styles.OfflineIndicator.Render("○")
		}

		// Name
		nameWidth := m.width - 8 // space for indicator, padding, unread badge
		if nameWidth < 4 {
			nameWidth = 4
		}
		name := c.DisplayName
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}

		var nameStyle lipgloss.Style
		if i == m.selected {
			nameStyle = styles.ContactSelected
		} else {
			nameStyle = styles.ContactName
		}

		// Unread badge
		var badge string
		if c.UnreadCount > 0 {
			badge = styles.UnreadBadge.Render(fmt.Sprintf("(%d)", c.UnreadCount))
		}

		// Compose line
		line := fmt.Sprintf(" %s %s %s", indicator, nameStyle.Render(name), badge)
		// Highlight selected row
		if i == m.selected && m.focused {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#292e42")).
				Width(m.width - 2).
				Render(line)
		}

		lines = append(lines, line)
	}

	if len(m.contacts) == 0 {
		lines = append(lines, styles.Subtle.Render("  No contacts yet"))
	}

	content := strings.Join(lines, "\n")

	// Footer hints
	footer := styles.Subtle.Render("  / cmds  ? help")

	// Pad to fill height
	contentLines := len(lines)
	remaining := m.height - 3 - contentLines - 1 // border top, title, border bottom, footer
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}
	content += "\n" + footer

	titleStyle := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(m.width).
		Height(m.height).
		Render(titleStyle.Render(" Contacts") + "\n" + content)

	return box
}
