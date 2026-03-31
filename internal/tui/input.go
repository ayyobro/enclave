package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"enclave/internal/theme"
)

// InputModel wraps a textarea for message composition.
type InputModel struct {
	textarea textarea.Model
	focused  bool
	disabled bool
	width    int
	height   int
}

func NewInputModel() InputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // no limit — server enforces 64KB max
	ta.SetHeight(1)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.Focus()
	return InputModel{
		textarea: ta,
		focused:  true,
		height:   3,
	}
}

func (m *InputModel) SetWidth(w int) {
	m.width = w
	m.textarea.SetWidth(w - 6) // padding + prompt char
}

func (m *InputModel) SetFocused(f bool) {
	m.focused = f
	if f && !m.disabled {
		m.textarea.Focus()
	} else {
		m.textarea.Blur()
	}
}

func (m *InputModel) SetDisabled(d bool) {
	m.disabled = d
	if d {
		m.textarea.Blur()
		m.textarea.Placeholder = "Disconnected from server"
	} else {
		m.textarea.Placeholder = "Type a message..."
		if m.focused {
			m.textarea.Focus()
		}
	}
}

func (m *InputModel) Reset() {
	m.textarea.Reset()
}

func (m *InputModel) Value() string {
	return m.textarea.Value()
}

func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	if !m.focused || m.disabled {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+enter", "alt+enter", "ctrl+j":
			// Insert newline manually
			m.textarea.InsertString("\n")
			m.resizeToContent()
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			m.textarea.Reset()
			m.textarea.SetHeight(1)

			// Handle /copy command
			if strings.HasPrefix(text, "/copy") {
				parts := strings.Fields(text)
				if len(parts) == 2 {
					if idx, err := strconv.Atoi(parts[1]); err == nil {
						return m, func() tea.Msg {
							return CopyCodeBlockMsg{Index: idx}
						}
					}
				}
				// /copy with no number — copy the latest block
				return m, func() tea.Msg {
					return CopyCodeBlockMsg{Index: 0}
				}
			}

			return m, func() tea.Msg {
				return SendMessageCmd{Text: text}
			}
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.resizeToContent()
	return m, cmd
}

func (m *InputModel) resizeToContent() {
	lines := strings.Count(m.textarea.Value(), "\n") + 1
	if lines < 1 {
		lines = 1
	}
	if lines > 12 {
		lines = 12
	}
	m.textarea.SetHeight(lines)
}

func (m InputModel) View() string {
	t := theme.Current

	borderColor := t.Border
	if m.focused {
		borderColor = t.BorderActive
	}

	prompt := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Render("> ")

	content := prompt + m.textarea.View()

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(m.width).
		Padding(0, 1)

	return box.Render(content)
}
