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
		ID:          id,
		PublicKey:   publicKey,
		DisplayName: displayName,
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

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
