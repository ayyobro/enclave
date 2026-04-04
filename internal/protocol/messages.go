package protocol

import "encoding/json"

// Envelope is the top-level wire format for all WebSocket messages.
// The Type field is read first to determine how to interpret the payload.
type Envelope struct {
	Type string `json:"type"`
}

// RegisterMsg is sent by a new client to register with an invite token.
type RegisterMsg struct {
	Type        string `json:"type"`
	Token       string `json:"token"`
	PublicKey   string `json:"public_key"`
	DisplayName string `json:"display_name"`
}

// AuthMsg initiates the challenge-response authentication.
type AuthMsg struct {
	Type      string `json:"type"`
	PublicKey string `json:"public_key"`
}

// AuthRespMsg carries the client's response to the server's challenge.
type AuthRespMsg struct {
	Type     string `json:"type"`
	Response string `json:"response"`
}

// ChallengeMsg is sent by the server with a random nonce to authenticate.
type ChallengeMsg struct {
	Type      string `json:"type"`
	Nonce     string `json:"nonce"`
	ServerKey string `json:"server_key"`
}

// AuthOKMsg confirms successful authentication and provides the user list and groups.
type AuthOKMsg struct {
	Type   string      `json:"type"`
	Users  []UserInfo  `json:"users"`
	Groups []GroupInfo `json:"groups,omitempty"`
}

// AuthFailMsg indicates authentication failure.
type AuthFailMsg struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// UserInfo describes a registered user.
type UserInfo struct {
	PublicKey   string `json:"public_key"`
	DisplayName string `json:"display_name"`
	Online      bool   `json:"online"`
}

// ChatMsg carries an encrypted message between users.
type ChatMsg struct {
	Type       string `json:"type"`
	From       string `json:"from,omitempty"`
	To         string `json:"to"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	ID         int64  `json:"id,omitempty"`
	Timestamp  int64  `json:"ts,omitempty"`
}

// PresenceMsg notifies clients about online/offline status changes.
type PresenceMsg struct {
	Type      string `json:"type"`
	PublicKey string `json:"public_key"`
	Online    bool   `json:"online"`
}

// TypingMsg indicates a user is typing.
type TypingMsg struct {
	Type string `json:"type"`
	From string `json:"from,omitempty"`
	To   string `json:"to"`
}

// GroupCreateMsg requests creation of a new group.
type GroupCreateMsg struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Members []string `json:"members"` // base64 public keys of initial members
}

// GroupCreatedMsg confirms group creation.
type GroupCreatedMsg struct {
	Type    string   `json:"type"`
	GroupID string   `json:"group_id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
	Creator string   `json:"creator"`
}

// GroupInfoMsg provides group details (sent on connect).
type GroupInfoMsg struct {
	Type   string      `json:"type"`
	Groups []GroupInfo `json:"groups"`
}

// GroupInfo describes a group.
type GroupInfo struct {
	GroupID string   `json:"group_id"`
	Name    string   `json:"name"`
	Members []string `json:"members"` // base64 public keys
}

// GroupInviteMsg adds a member to a group.
type GroupInviteMsg struct {
	Type    string `json:"type"`
	GroupID string `json:"group_id"`
	Member  string `json:"member"` // base64 public key of invitee
}

// GroupLeaveMsg removes yourself from a group.
type GroupLeaveMsg struct {
	Type    string `json:"type"`
	GroupID string `json:"group_id"`
}

// GroupChatMsg carries an encrypted message to a group.
// The sender encrypts the plaintext once per recipient.
type GroupChatMsg struct {
	Type       string               `json:"type"`
	From       string               `json:"from,omitempty"`
	GroupID    string               `json:"group_id"`
	Recipients []GroupChatRecipient `json:"recipients"`
	ID         int64                `json:"id,omitempty"`
	Timestamp  int64                `json:"ts,omitempty"`
}

// GroupChatRecipient holds the per-recipient encrypted payload.
type GroupChatRecipient struct {
	To         string `json:"to"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// ReadReceiptMsg acknowledges that a message has been read.
type ReadReceiptMsg struct {
	Type      string `json:"type"`
	From      string `json:"from,omitempty"`
	To        string `json:"to"`
	MessageTS int64  `json:"message_ts"` // timestamp of the read message
}

// ReactionMsg adds a reaction to a message.
type ReactionMsg struct {
	Type      string `json:"type"`
	From      string `json:"from,omitempty"`
	To        string `json:"to"`         // recipient or group_id
	MessageTS int64  `json:"message_ts"` // timestamp of the message being reacted to
	Emoji     string `json:"emoji"`
}

// FileMetaMsg announces an incoming file transfer.
type FileMetaMsg struct {
	Type        string `json:"type"`
	From        string `json:"from,omitempty"`
	To          string `json:"to"`
	FileID      string `json:"file_id"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	Nonce       string `json:"nonce"`
	TotalChunks int    `json:"total_chunks"`
}

// FileChunkMsg carries one encrypted chunk of a file.
type FileChunkMsg struct {
	Type       string `json:"type"`
	From       string `json:"from,omitempty"`
	To         string `json:"to"`
	FileID     string `json:"file_id"`
	ChunkIndex int    `json:"chunk_index"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// VibeStartMsg announces a collaborative coding session.
type VibeStartMsg struct {
	Type     string `json:"type"`
	From     string `json:"from,omitempty"`
	To       string `json:"to"` // DM recipient or group ID
	RepoName string `json:"repo_name"`
}

// VibePromptMsg sends a prompt from a participant to the host's Claude Code.
type VibePromptMsg struct {
	Type   string `json:"type"`
	From   string `json:"from,omitempty"`
	To     string `json:"to"` // the host's public key
	Prompt string `json:"prompt"`
}

// VibeOutputMsg streams Claude Code output from the host to participants.
type VibeOutputMsg struct {
	Type   string `json:"type"`
	From   string `json:"from,omitempty"`
	To     string `json:"to"` // DM recipient or group ID
	Text   string `json:"text"`
	IsDone bool   `json:"is_done"`
}

// VibeEndMsg ends a collaborative coding session.
type VibeEndMsg struct {
	Type string `json:"type"`
	From string `json:"from,omitempty"`
	To   string `json:"to"`
}

// EphemeralMsg notifies the other party about ephemeral mode changes.
type EphemeralMsg struct {
	Type     string `json:"type"`
	From     string `json:"from,omitempty"`
	To       string `json:"to"`
	Duration string `json:"duration"` // Go duration string, or "off"
}

// ErrorMsg is sent by the server to report errors.
// KeyRotateMsg is sent by a client to rotate their keypair.
type KeyRotateMsg struct {
	Type      string `json:"type"`
	OldKey    string `json:"old_key"`    // base64 old public key
	NewKey    string `json:"new_key"`    // base64 new public key
	Signature string `json:"signature"`  // old key signs new key to prove ownership
}

// KeyChangedMsg is broadcast to notify contacts that a user's key changed.
type KeyChangedMsg struct {
	Type        string `json:"type"`
	OldKey      string `json:"old_key"`
	NewKey      string `json:"new_key"`
	DisplayName string `json:"display_name"`
}

type ErrorMsg struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ParseType reads just the type field from a raw JSON message.
func ParseType(data []byte) (string, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return "", err
	}
	return env.Type, nil
}
