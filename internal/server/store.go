package server

import "time"

// User represents a registered user in the server database.
type User struct {
	ID           int64
	PublicKey    []byte
	DisplayName  string
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

// Group represents a chat group.
type Group struct {
	ID        string
	Name      string
	Creator   string // base64 public key
	CreatedAt time.Time
}

// GroupMember represents a member of a group.
type GroupMember struct {
	GroupID   string
	PublicKey []byte
}

// Store defines the server-side storage interface.
type Store interface {
	// Users
	CreateUser(publicKey []byte, displayName string) (*User, error)
	GetUserByKey(publicKey []byte) (*User, error)
	ListUsers() ([]User, error)
	RotateUserKey(oldKey, newKey []byte, displayName string) error
	RevokeUser(publicKey []byte) error
	IsKeyRevoked(publicKey []byte) (bool, error)

	// Invite tokens
	CreateInviteToken(tokenHash []byte, maxUses int, expiresAt time.Time) error
	ValidateAndUseInvite(tokenHash []byte) error

	// Pending messages (offline delivery)
	StorePendingMessage(senderKey, recipientKey, nonce, ciphertext []byte) (int64, error)
	GetPendingMessages(recipientKey []byte) ([]PendingMessage, error)
	DeletePendingMessage(id int64) error

	// Groups
	CreateGroup(id, name, creatorKey string, memberKeys []string) error
	GetGroup(id string) (*Group, error)
	GetGroupMembers(groupID string) ([]string, error) // returns base64 public keys
	GetUserGroups(publicKey string) ([]Group, error)
	AddGroupMember(groupID, publicKey string) error
	RemoveGroupMember(groupID, publicKey string) error

	// Lifecycle
	Close() error
}
