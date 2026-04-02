package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements MessageStore using a local SQLite database.
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
	CREATE TABLE IF NOT EXISTS conversations (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		peer_key        TEXT NOT NULL UNIQUE,
		display_name    TEXT NOT NULL,
		last_message    TEXT NOT NULL DEFAULT '',
		last_msg_time   DATETIME,
		unread_count    INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS messages (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER NOT NULL REFERENCES conversations(id),
		direction       TEXT NOT NULL CHECK(direction IN ('sent', 'received')),
		plaintext       TEXT NOT NULL,
		timestamp       DATETIME NOT NULL,
		server_id       INTEGER DEFAULT 0,
		status          TEXT NOT NULL DEFAULT 'delivered',
		created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_messages_server_id ON messages(server_id) WHERE server_id > 0;
	`

	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Create FTS5 table if it doesn't exist.
	// FTS5 doesn't support IF NOT EXISTS, so check manually.
	var count int
	err := s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='messages_fts'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		fts := `
		CREATE VIRTUAL TABLE messages_fts USING fts5(
			plaintext,
			content=messages,
			content_rowid=id
		);

		CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, plaintext) VALUES (new.id, new.plaintext);
		END;

		CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, plaintext) VALUES('delete', old.id, old.plaintext);
		END;
		`
		if _, err := s.db.Exec(fts); err != nil {
			return fmt.Errorf("creating FTS table: %w", err)
		}
	}

	return nil
}

func (s *SQLiteStore) GetOrCreateConversation(peerKey, displayName string) (*Conversation, error) {
	// Try to get existing
	var c Conversation
	var lastMsgTime sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, peer_key, display_name, last_message, last_msg_time, unread_count FROM conversations WHERE peer_key = ?",
		peerKey,
	).Scan(&c.ID, &c.PeerKey, &c.DisplayName, &c.LastMessage, &lastMsgTime, &c.UnreadCount)

	if err == nil {
		if lastMsgTime.Valid {
			c.LastMsgTime = lastMsgTime.Time
		}
		// Update display name if changed
		if c.DisplayName != displayName && displayName != "" {
			s.db.Exec("UPDATE conversations SET display_name = ? WHERE id = ?", displayName, c.ID)
			c.DisplayName = displayName
		}
		return &c, nil
	}

	if err != sql.ErrNoRows {
		return nil, err
	}

	// Create new
	result, err := s.db.Exec(
		"INSERT INTO conversations (peer_key, display_name) VALUES (?, ?)",
		peerKey, displayName,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Conversation{
		ID:          id,
		PeerKey:     peerKey,
		DisplayName: displayName,
	}, nil
}

func (s *SQLiteStore) ListConversations() ([]Conversation, error) {
	rows, err := s.db.Query(
		"SELECT id, peer_key, display_name, last_message, last_msg_time, unread_count FROM conversations ORDER BY last_msg_time DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convos []Conversation
	for rows.Next() {
		var c Conversation
		var lastMsgTime sql.NullTime
		if err := rows.Scan(&c.ID, &c.PeerKey, &c.DisplayName, &c.LastMessage, &lastMsgTime, &c.UnreadCount); err != nil {
			return nil, err
		}
		if lastMsgTime.Valid {
			c.LastMsgTime = lastMsgTime.Time
		}
		convos = append(convos, c)
	}
	return convos, rows.Err()
}

func (s *SQLiteStore) UpdateConversationName(peerKey, displayName string) error {
	_, err := s.db.Exec("UPDATE conversations SET display_name = ? WHERE peer_key = ?", displayName, peerKey)
	return err
}

func (s *SQLiteStore) SaveMessage(conversationID int64, dir Direction, plaintext string, ts time.Time, serverID int64) (*Message, error) {
	result, err := s.db.Exec(
		"INSERT INTO messages (conversation_id, direction, plaintext, timestamp, server_id) VALUES (?, ?, ?, ?, ?)",
		conversationID, string(dir), plaintext, ts, serverID,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()

	// Update conversation's last message
	preview := plaintext
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}
	s.db.Exec(
		"UPDATE conversations SET last_message = ?, last_msg_time = ? WHERE id = ?",
		preview, ts, conversationID,
	)

	return &Message{
		ID:             id,
		ConversationID: conversationID,
		Direction:      dir,
		Plaintext:      plaintext,
		Timestamp:      ts,
		ServerID:       serverID,
		Status:         "delivered",
	}, nil
}

func (s *SQLiteStore) GetMessages(conversationID int64, limit, offset int) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT id, conversation_id, direction, plaintext, timestamp, server_id, status FROM messages WHERE conversation_id = ? ORDER BY timestamp ASC LIMIT ? OFFSET ?",
		conversationID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows)
}

func (s *SQLiteStore) GetRecentMessages(conversationID int64, limit int) ([]Message, error) {
	// Get the last N messages, but return them in chronological order
	rows, err := s.db.Query(
		`SELECT id, conversation_id, direction, plaintext, timestamp, server_id, status
		 FROM (
			SELECT * FROM messages WHERE conversation_id = ? ORDER BY timestamp DESC LIMIT ?
		 ) ORDER BY timestamp ASC`,
		conversationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows)
}

func (s *SQLiteStore) HasServerMessage(serverID int64) (bool, error) {
	if serverID == 0 {
		return false, nil
	}
	var count int
	err := s.db.QueryRow("SELECT count(*) FROM messages WHERE server_id = ?", serverID).Scan(&count)
	return count > 0, err
}

func (s *SQLiteStore) IncrementUnread(conversationID int64) error {
	_, err := s.db.Exec("UPDATE conversations SET unread_count = unread_count + 1 WHERE id = ?", conversationID)
	return err
}

func (s *SQLiteStore) ClearUnread(conversationID int64) error {
	_, err := s.db.Exec("UPDATE conversations SET unread_count = 0 WHERE id = ?", conversationID)
	return err
}

func (s *SQLiteStore) SearchMessages(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT m.id, m.conversation_id, m.direction, m.plaintext, m.timestamp, m.server_id, m.status,
		        c.peer_key, c.display_name
		 FROM messages_fts fts
		 JOIN messages m ON m.id = fts.rowid
		 JOIN conversations c ON c.id = m.conversation_id
		 WHERE messages_fts MATCH ?
		 ORDER BY m.timestamp DESC
		 LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var dir string
		if err := rows.Scan(
			&r.Message.ID, &r.Message.ConversationID, &dir, &r.Message.Plaintext,
			&r.Message.Timestamp, &r.Message.ServerID, &r.Message.Status,
			&r.PeerKey, &r.DisplayName,
		); err != nil {
			return nil, err
		}
		r.Message.Direction = Direction(dir)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var msgs []Message
	for rows.Next() {
		var m Message
		var dir string
		if err := rows.Scan(&m.ID, &m.ConversationID, &dir, &m.Plaintext, &m.Timestamp, &m.ServerID, &m.Status); err != nil {
			return nil, err
		}
		m.Direction = Direction(dir)
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
