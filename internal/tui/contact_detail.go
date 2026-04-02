package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"enclave/internal/crypto"
	"enclave/internal/theme"
)

// ContactDetailModel renders an overlay with contact key verification info.
type ContactDetailModel struct {
	visible     bool
	contact     *ContactInfo
	fingerprint string
	width       int
	height      int
}

func NewContactDetailModel() ContactDetailModel {
	return ContactDetailModel{}
}

func (m *ContactDetailModel) Show(contact *ContactInfo) {
	m.visible = true
	m.contact = contact
	if contact != nil {
		key, err := crypto.PubKeyFromBase64(contact.PublicKey)
		if err == nil {
			m.fingerprint = crypto.Fingerprint(key)
		}
	}
}

func (m *ContactDetailModel) Hide() {
	m.visible = false
	m.contact = nil
}

func (m *ContactDetailModel) IsVisible() bool {
	return m.visible
}

func (m *ContactDetailModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m ContactDetailModel) Update(msg tea.Msg) (ContactDetailModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "enter", "q":
			m.visible = false
			return m, nil
		}
	}

	return m, nil
}

func (m ContactDetailModel) View() string {
	if !m.visible || m.contact == nil {
		return ""
	}

	t := theme.Current
	s := theme.NewStyles(t)

	titleStyle := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(t.ForegroundDim).
		Width(14)

	valueStyle := lipgloss.NewStyle().
		Foreground(t.Foreground)

	// Online status
	status := s.OfflineIndicator.Render("○ offline")
	if m.contact.Online {
		status = s.OnlineIndicator.Render("● online")
	}

	// Format the fingerprint in two rows for readability
	fpParts := strings.Fields(m.fingerprint)
	fpLine1 := ""
	fpLine2 := ""
	if len(fpParts) >= 8 {
		fpLine1 = strings.Join(fpParts[:4], " ")
		fpLine2 = strings.Join(fpParts[4:], " ")
	} else {
		fpLine1 = m.fingerprint
	}

	// Build content
	var lines []string
	lines = append(lines, "")
	lines = append(lines, titleStyle.Render("  Contact: "+m.contact.DisplayName))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %s%s", labelStyle.Render("Status:"), status))
	keyDisplay := m.contact.PublicKey
	if len(keyDisplay) > 22 {
		keyDisplay = keyDisplay[:22] + "..."
	}
	lines = append(lines, fmt.Sprintf("  %s%s", labelStyle.Render("Public key:"), valueStyle.Render(keyDisplay)))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %s%s", labelStyle.Render("Fingerprint:"), valueStyle.Render(fpLine1)))
	if fpLine2 != "" {
		lines = append(lines, fmt.Sprintf("  %s%s", labelStyle.Render(""), valueStyle.Render(fpLine2)))
	}
	lines = append(lines, "")

	// Visual fingerprint — colored blocks derived from the key
	if key, err := crypto.PubKeyFromBase64(m.contact.PublicKey); err == nil {
		lines = append(lines, "  "+labelStyle.Render("Visual ID:")+"  "+visualFingerprint(key))
		lines = append(lines, "")
	}

	verifyNote := s.Subtle.Render(fmt.Sprintf(
		"  Compare this fingerprint with %s\n  through a separate channel (phone, in person)\n  to verify their identity.",
		m.contact.DisplayName,
	))
	lines = append(lines, verifyNote)
	lines = append(lines, "")

	dismissHint := s.Subtle.Render("  Press Esc to close")
	lines = append(lines, dismissHint)
	lines = append(lines, "")

	content := strings.Join(lines, "\n")

	// Overlay box
	overlayWidth := 56
	if overlayWidth > m.width-4 {
		overlayWidth = m.width - 4
	}

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(t.Primary).
		Width(overlayWidth).
		Render(content)

	// Center the overlay
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// visualFingerprint generates a row of colored block characters from a public key.
func visualFingerprint(key *[32]byte) string {
	colors := []string{
		"#f7768e", "#ff9e64", "#e0af68", "#9ece6a",
		"#73daca", "#7dcfff", "#7aa2f7", "#bb9af7",
	}

	var blocks []string
	for i := 0; i < 8; i++ {
		b := key[i*4] ^ key[i*4+1] ^ key[i*4+2] ^ key[i*4+3]
		colorIdx := int(b) % len(colors)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(colors[colorIdx]))
		blocks = append(blocks, style.Render("██"))
	}
	return strings.Join(blocks, " ")
}
