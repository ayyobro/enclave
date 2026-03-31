package server

import "time"

// User represents a registered user in the server database.
type User struct {
	ID          int64
	PublicKey   []byte
	DisplayName string
	RegisteredAt time.Time
}

// PendingMessage is an encrypted message waiting for an offline recipient.
type PendingMessage struct {
	ID           int64
	SenderKey    []byte
	RecipientKey []byte
	Nonce        []byte
	Ciphertext   []byte
	CreatedAt    time.Time
}

// InviteToken represents a registration invite.
type InviteToken struct {
	ID        int64
	TokenHash []byte
	MaxUses   int
	UsedCount int
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Store defines the server-side storage interface.
type Store interface {
	// Users
	CreateUser(publicKey []byte, displayName string) (*User, error)
	GetUserByKey(publicKey []byte) (*User, error)
	ListUsers() ([]User, error)

	// Invite tokens
	CreateInviteToken(tokenHash []byte, maxUses int, expiresAt time.Time) error
	ValidateAndUseInvite(tokenHash []byte) error

	// Pending messages (offline delivery)
	StorePendingMessage(senderKey, recipientKey, nonce, ciphertext []byte) (int64, error)
	GetPendingMessages(recipientKey []byte) ([]PendingMessage, error)
	DeletePendingMessage(id int64) error

	// Lifecycle
	Close() error
}
