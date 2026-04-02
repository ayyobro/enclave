package server

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		public_key   BLOB NOT NULL UNIQUE,
		display_name TEXT NOT NULL,
		registered_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS invite_tokens (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		token_hash BLOB NOT NULL UNIQUE,
		max_uses   INTEGER NOT NULL DEFAULT 1,
		used_count INTEGER NOT NULL DEFAULT 0,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS pending_messages (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		sender_key    BLOB NOT NULL,
		recipient_key BLOB NOT NULL,
		nonce         BLOB NOT NULL,
		ciphertext    BLOB NOT NULL,
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_pending_recipient ON pending_messages(recipient_key);

	CREATE TABLE IF NOT EXISTS groups (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		creator    TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS group_members (
		group_id   TEXT NOT NULL REFERENCES groups(id),
		public_key TEXT NOT NULL,
		PRIMARY KEY (group_id, public_key)
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStore) CreateUser(publicKey []byte, displayName string) (*User, error) {
	result, err := s.db.Exec(
		"INSERT INTO users (public_key, display_name) VALUES (?, ?)",
		publicKey, displayName,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &User{
		ID:           id,
		PublicKey:    publicKey,
		DisplayName:  displayName,
		RegisteredAt: time.Now(),
	}, nil
}

func (s *SQLiteStore) GetUserByKey(publicKey []byte) (*User, error) {
	var u User
	err := s.db.QueryRow(
		"SELECT id, public_key, display_name, registered_at FROM users WHERE public_key = ?",
		publicKey,
	).Scan(&u.ID, &u.PublicKey, &u.DisplayName, &u.RegisteredAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *SQLiteStore) ListUsers() ([]User, error) {
	rows, err := s.db.Query("SELECT id, public_key, display_name, registered_at FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.PublicKey, &u.DisplayName, &u.RegisteredAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *SQLiteStore) CreateInviteToken(tokenHash []byte, maxUses int, expiresAt time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO invite_tokens (token_hash, max_uses, expires_at) VALUES (?, ?, ?)",
		tokenHash, maxUses, expiresAt,
	)
	return err
}

func (s *SQLiteStore) ValidateAndUseInvite(tokenHash []byte) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id int64
	var maxUses, usedCount int
	var expiresAt time.Time

	err = tx.QueryRow(
		"SELECT id, max_uses, used_count, expires_at FROM invite_tokens WHERE token_hash = ?",
		tokenHash,
	).Scan(&id, &maxUses, &usedCount, &expiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("invalid invite token")
		}
		return err
	}

	if time.Now().After(expiresAt) {
		return fmt.Errorf("invite token has expired")
	}
	if usedCount >= maxUses {
		return fmt.Errorf("invite token has been fully used")
	}

	_, err = tx.Exec("UPDATE invite_tokens SET used_count = used_count + 1 WHERE id = ?", id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SQLiteStore) StorePendingMessage(senderKey, recipientKey, nonce, ciphertext []byte) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO pending_messages (sender_key, recipient_key, nonce, ciphertext) VALUES (?, ?, ?, ?)",
		senderKey, recipientKey, nonce, ciphertext,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) GetPendingMessages(recipientKey []byte) ([]PendingMessage, error) {
	rows, err := s.db.Query(
		"SELECT id, sender_key, recipient_key, nonce, ciphertext, created_at FROM pending_messages WHERE recipient_key = ? ORDER BY created_at ASC",
		recipientKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []PendingMessage
	for rows.Next() {
		var m PendingMessage
		if err := rows.Scan(&m.ID, &m.SenderKey, &m.RecipientKey, &m.Nonce, &m.Ciphertext, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (s *SQLiteStore) DeletePendingMessage(id int64) error {
	_, err := s.db.Exec("DELETE FROM pending_messages WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) CreateGroup(id, name, creatorKey string, memberKeys []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("INSERT INTO groups (id, name, creator) VALUES (?, ?, ?)", id, name, creatorKey)
	if err != nil {
		return err
	}

	for _, key := range memberKeys {
		_, err = tx.Exec("INSERT INTO group_members (group_id, public_key) VALUES (?, ?)", id, key)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetGroup(id string) (*Group, error) {
	var g Group
	err := s.db.QueryRow("SELECT id, name, creator, created_at FROM groups WHERE id = ?", id).
		Scan(&g.ID, &g.Name, &g.Creator, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *SQLiteStore) GetGroupMembers(groupID string) ([]string, error) {
	rows, err := s.db.Query("SELECT public_key FROM group_members WHERE group_id = ?", groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *SQLiteStore) GetUserGroups(publicKey string) ([]Group, error) {
	rows, err := s.db.Query(
		`SELECT g.id, g.name, g.creator, g.created_at FROM groups g
		 JOIN group_members gm ON g.id = gm.group_id
		 WHERE gm.public_key = ?`, publicKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Creator, &g.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *SQLiteStore) AddGroupMember(groupID, publicKey string) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO group_members (group_id, public_key) VALUES (?, ?)", groupID, publicKey)
	return err
}

func (s *SQLiteStore) RemoveGroupMember(groupID, publicKey string) error {
	_, err := s.db.Exec("DELETE FROM group_members WHERE group_id = ? AND public_key = ?", groupID, publicKey)
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
