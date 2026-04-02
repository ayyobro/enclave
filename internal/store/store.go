package store

import "time"

// Direction indicates whether a message was sent or received.
type Direction string

const (
	Sent     Direction = "sent"
	Received Direction = "received"
)

// Conversation represents a chat with a specific peer or group.
type Conversation struct {
	ID           int64
	PeerKey      string // base64 public key or group ID
	DisplayName  string
	IsGroup      bool
	LastMessage  string
	LastMsgTime  time.Time
	UnreadCount  int
}

// Message represents a single chat message stored locally.
type Message struct {
	ID             int64
	ConversationID int64
	Direction      Direction
	SenderName     string    // display name of the sender (useful for group chats)
	Plaintext      string
	Timestamp      time.Time
	ExpiresAt      time.Time // zero means no expiry
	ServerID       int64     // for dedup
	Status         string    // "delivered", "sending", "failed"
}

// Reaction represents an emoji reaction stored locally.
type Reaction struct {
	ID        int64
	MessageID int64
	FromName  string
	Emoji     string
}

// MessageStore defines the client-side local storage interface.
type MessageStore interface {
	// Conversations
	GetOrCreateConversation(peerKey, displayName string) (*Conversation, error)
	GetOrCreateGroupConversation(groupID, groupName string) (*Conversation, error)
	ListConversations() ([]Conversation, error)
	UpdateConversationName(peerKey, displayName string) error

	// Messages
	SaveMessage(conversationID int64, dir Direction, senderName, plaintext string, ts time.Time, serverID int64) (*Message, error)
	GetMessages(conversationID int64, limit, offset int) ([]Message, error)
	GetRecentMessages(conversationID int64, limit int) ([]Message, error)
	HasServerMessage(serverID int64) (bool, error)

	// Ephemeral
	SetMessageExpiry(messageID int64, expiresAt time.Time) error
	DeleteExpiredMessages() (int, error) // returns count deleted

	// Reactions
	SaveReaction(messageID int64, fromName, emoji string) error
	GetReactions(messageIDs []int64) (map[int64][]Reaction, error)

	// Unread tracking
	IncrementUnread(conversationID int64) error
	ClearUnread(conversationID int64) error

	// Search
	SearchMessages(query string, limit int) ([]SearchResult, error)

	// Lifecycle
	Close() error
}

// SearchResult pairs a message with its conversation context.
type SearchResult struct {
	Message      Message
	PeerKey      string
	DisplayName  string
}
