package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"enclave/internal/theme"
)

// InputModel wraps a textarea for message composition with slash command autocomplete.
type InputModel struct {
	textarea textarea.Model
	focused  bool
	disabled bool
	width    int
	height   int

	// Autocomplete state
	showComplete   bool
	completions    []SlashCommand
	completeIdx    int
}

func NewInputModel() InputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (/ for commands)"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
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
	m.textarea.SetWidth(w - 6)
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
		m.textarea.Placeholder = "Type a message... (/ for commands)"
		if m.focused {
			m.textarea.Focus()
		}
	}
}

func (m *InputModel) Reset() {
	m.textarea.Reset()
	m.showComplete = false
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
		// Handle autocomplete navigation when dropdown is visible
		if m.showComplete {
			switch msg.String() {
			case "up":
				if m.completeIdx > 0 {
					m.completeIdx--
				}
				return m, nil
			case "down":
				if m.completeIdx < len(m.completions)-1 {
					m.completeIdx++
				}
				return m, nil
			case "tab":
				// Accept the selected completion
				if len(m.completions) > 0 {
					selected := m.completions[m.completeIdx]
					m.textarea.Reset()
					text := selected.Name
					if selected.HasArgs {
						text += " "
					}
					m.textarea.InsertString(text)
					m.showComplete = false
					return m, nil
				}
			case "esc":
				m.showComplete = false
				return m, nil
			case "enter":
				// If a completion is highlighted and the input is just a partial slash,
				// accept the completion first
				if len(m.completions) > 0 {
					val := strings.TrimSpace(m.textarea.Value())
					selected := m.completions[m.completeIdx]
					if val != selected.Name && !strings.HasPrefix(val, selected.Name+" ") {
						// They haven't finished typing — accept completion and submit
						m.textarea.Reset()
						m.showComplete = false
						if selected.HasArgs {
							// Needs args — fill it in and let them type
							m.textarea.InsertString(selected.Name + " ")
							return m, nil
						}
						return m, func() tea.Msg {
							return SlashCommandMsg{Name: selected.Name}
						}
					}
				}
				// Fall through to normal enter handling below
			}
		}

		switch msg.String() {
		case "ctrl+enter", "alt+enter", "ctrl+j":
			m.textarea.InsertString("\n")
			m.resizeToContent()
			m.showComplete = false
			return m, nil

		case "enter":
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			m.textarea.Reset()
			m.textarea.SetHeight(1)
			m.showComplete = false

			// Parse slash commands
			if strings.HasPrefix(text, "/") {
				parts := strings.SplitN(text, " ", 2)
				name := parts[0]
				args := ""
				if len(parts) > 1 {
					args = parts[1]
				}
				return m, func() tea.Msg {
					return SlashCommandMsg{Name: name, Args: args}
				}
			}

			return m, func() tea.Msg {
				return SendMessageCmd{Text: text}
			}
		}
	}

	// Let textarea handle the keystroke
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.resizeToContent()

	// Update autocomplete based on current text
	m.updateCompletions()

	// Emit typing indicator for non-slash, non-empty input
	val := m.textarea.Value()
	if len(val) > 0 && !strings.HasPrefix(val, "/") {
		return m, tea.Batch(cmd, func() tea.Msg { return UserTypingMsg{} })
	}

	return m, cmd
}

func (m *InputModel) updateCompletions() {
	val := m.textarea.Value()

	// Show completions when text starts with / and is a single line
	if strings.HasPrefix(val, "/") && !strings.Contains(val, "\n") {
		prefix := strings.Fields(val)
		if len(prefix) <= 1 {
			// Still typing the command name
			m.completions = FilterCommands(val)
			m.showComplete = len(m.completions) > 0
			if m.completeIdx >= len(m.completions) {
				m.completeIdx = 0
			}
			return
		}
	}

	m.showComplete = false
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

	rendered := box.Render(content)

	// Render autocomplete dropdown above the input box
	if m.showComplete && len(m.completions) > 0 {
		dropdown := m.renderCompletions()
		return dropdown + "\n" + rendered
	}

	return rendered
}

func (m InputModel) renderCompletions() string {
	t := theme.Current
	s := theme.NewStyles(t)

	var lines []string
	for i, cmd := range m.completions {
		name := cmd.Name
		desc := cmd.Description

		nameStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
		descStyle := lipgloss.NewStyle().Foreground(t.ForegroundDim)

		line := fmt.Sprintf("  %s  %s", nameStyle.Render(name), descStyle.Render(desc))

		if i == m.completeIdx {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#292e42")).
				Width(m.width - 4).
				Render(line)
		}

		lines = append(lines, line)
	}

	_ = s // using theme directly

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Width(m.width).
		Render(strings.Join(lines, "\n"))

	return box
}
