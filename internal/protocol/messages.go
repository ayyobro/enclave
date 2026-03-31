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

// AuthOKMsg confirms successful authentication and provides the user list.
type AuthOKMsg struct {
	Type  string     `json:"type"`
	Users []UserInfo `json:"users"`
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

// ErrorMsg is sent by the server to report errors.
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
