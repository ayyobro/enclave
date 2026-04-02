package tui

import "time"

// Custom tea.Msg types for the TUI event system.
// Client-side events (IncomingChat, Presence, Typing) are defined in client/appcore.go
// to avoid import cycles. The TUI receives them as interface{} via type switches.

// ConnectedMsg is sent when the WebSocket connection is established and authenticated.
type ConnectedMsg struct {
	Users []ContactInfo
}

// DisconnectedMsg is sent when the connection drops.
type DisconnectedMsg struct {
	Err error
}

// ReconnectingMsg is sent when a reconnection attempt starts.
type ReconnectingMsg struct {
	Attempt int
}

// ContactInfo represents a known contact or group for display.
type ContactInfo struct {
	PublicKey    string // public key for DMs, group ID for groups
	DisplayName string
	Online      bool
	IsGroup     bool
	UnreadCount int
	LastMessage string
	LastMsgTime time.Time
}

// SendMessageCmd is emitted by the input model to request sending a message.
type SendMessageCmd struct {
	Text string
}

// SelectContactMsg is emitted when a contact is selected in the sidebar.
type SelectContactMsg struct {
	PublicKey   string
	DisplayName string
}

// TUIErrorMsg is a generic error notification for the TUI.
type TUIErrorMsg struct {
	Err error
}

// SlashCommandMsg is emitted when the user executes a slash command.
type SlashCommandMsg struct {
	Name string // e.g. "/help", "/clear"
	Args string // everything after the command name
}

// UserTypingMsg is emitted by the input model when the user types a non-command character.
type UserTypingMsg struct{}

// TypingClearTickMsg fires after a delay to clear the typing indicator.
type TypingClearTickMsg struct{}

// EphemeralTickMsg fires periodically to purge expired ephemeral messages.
type EphemeralTickMsg struct{}

// FocusPane identifies which pane has focus.
type FocusPane int

const (
	FocusSidebar FocusPane = iota
	FocusChat
	FocusInput
)
