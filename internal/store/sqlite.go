package store

import (
	"database/sql"
	"fmt"
	"strings"
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
		is_group        INTEGER NOT NULL DEFAULT 0,
		last_message    TEXT NOT NULL DEFAULT '',
		last_msg_time   DATETIME,
		unread_count    INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS messages (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER NOT NULL REFERENCES conversations(id),
		direction       TEXT NOT NULL CHECK(direction IN ('sent', 'received')),
		sender_name     TEXT NOT NULL DEFAULT '',
		plaintext       TEXT NOT NULL,
		timestamp       DATETIME NOT NULL,
		expires_at      DATETIME,
		server_id       INTEGER DEFAULT 0,
		status          TEXT NOT NULL DEFAULT 'delivered',
		created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_messages_server_id ON messages(server_id) WHERE server_id > 0;

	CREATE TABLE IF NOT EXISTS reactions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id  INTEGER NOT NULL REFERENCES messages(id),
		from_name   TEXT NOT NULL,
		emoji       TEXT NOT NULL,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_reactions_message ON reactions(message_id);
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
	return s.getOrCreateConvo(peerKey, displayName, false)
}

func (s *SQLiteStore) GetOrCreateGroupConversation(groupID, groupName string) (*Conversation, error) {
	return s.getOrCreateConvo(groupID, groupName, true)
}

func (s *SQLiteStore) getOrCreateConvo(peerKey, displayName string, isGroup bool) (*Conversation, error) {
	var c Conversation
	var lastMsgTime sql.NullTime
	var isGroupInt int
	err := s.db.QueryRow(
		"SELECT id, peer_key, display_name, is_group, last_message, last_msg_time, unread_count FROM conversations WHERE peer_key = ?",
		peerKey,
	).Scan(&c.ID, &c.PeerKey, &c.DisplayName, &isGroupInt, &c.LastMessage, &lastMsgTime, &c.UnreadCount)

	if err == nil {
		c.IsGroup = isGroupInt != 0
		if lastMsgTime.Valid {
			c.LastMsgTime = lastMsgTime.Time
		}
		if c.DisplayName != displayName && displayName != "" {
			s.db.Exec("UPDATE conversations SET display_name = ? WHERE id = ?", displayName, c.ID)
			c.DisplayName = displayName
		}
		return &c, nil
	}

	if err != sql.ErrNoRows {
		return nil, err
	}

	groupInt := 0
	if isGroup {
		groupInt = 1
	}
	result, err := s.db.Exec(
		"INSERT INTO conversations (peer_key, display_name, is_group) VALUES (?, ?, ?)",
		peerKey, displayName, groupInt,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Conversation{
		ID:          id,
		PeerKey:     peerKey,
		DisplayName: displayName,
		IsGroup:     isGroup,
	}, nil
}

func (s *SQLiteStore) ListConversations() ([]Conversation, error) {
	rows, err := s.db.Query(
		"SELECT id, peer_key, display_name, is_group, last_message, last_msg_time, unread_count FROM conversations ORDER BY last_msg_time DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convos []Conversation
	for rows.Next() {
		var c Conversation
		var lastMsgTime sql.NullTime
		var isGroupInt int
		if err := rows.Scan(&c.ID, &c.PeerKey, &c.DisplayName, &isGroupInt, &c.LastMessage, &lastMsgTime, &c.UnreadCount); err != nil {
			return nil, err
		}
		c.IsGroup = isGroupInt != 0
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

func (s *SQLiteStore) SaveMessage(conversationID int64, dir Direction, senderName, plaintext string, ts time.Time, serverID int64) (*Message, error) {
	result, err := s.db.Exec(
		"INSERT INTO messages (conversation_id, direction, sender_name, plaintext, timestamp, server_id) VALUES (?, ?, ?, ?, ?, ?)",
		conversationID, string(dir), senderName, plaintext, ts, serverID,
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
		SenderName:     senderName,
		Plaintext:      plaintext,
		Timestamp:      ts,
		ServerID:       serverID,
		Status:         "delivered",
	}, nil
}

func (s *SQLiteStore) GetMessages(conversationID int64, limit, offset int) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT id, conversation_id, direction, sender_name, plaintext, timestamp, expires_at, server_id, status FROM messages WHERE conversation_id = ? ORDER BY timestamp ASC LIMIT ? OFFSET ?",
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
		`SELECT id, conversation_id, direction, sender_name, plaintext, timestamp, expires_at, server_id, status
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
		`SELECT m.id, m.conversation_id, m.direction, m.sender_name, m.plaintext, m.timestamp, m.expires_at, m.server_id, m.status,
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
		var expiresAt sql.NullTime
		if err := rows.Scan(
			&r.Message.ID, &r.Message.ConversationID, &dir, &r.Message.SenderName, &r.Message.Plaintext,
			&r.Message.Timestamp, &expiresAt, &r.Message.ServerID, &r.Message.Status,
			&r.PeerKey, &r.DisplayName,
		); err != nil {
			return nil, err
		}
		r.Message.Direction = Direction(dir)
		if expiresAt.Valid {
			r.Message.ExpiresAt = expiresAt.Time
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *SQLiteStore) SetMessageExpiry(messageID int64, expiresAt time.Time) error {
	_, err := s.db.Exec("UPDATE messages SET expires_at = ? WHERE id = ?", expiresAt, messageID)
	return err
}

func (s *SQLiteStore) DeleteExpiredMessages() (int, error) {
	// Delete reactions for expired messages first
	s.db.Exec(`DELETE FROM reactions WHERE message_id IN (
		SELECT id FROM messages WHERE expires_at IS NOT NULL AND expires_at < ?
	)`, time.Now())

	// Delete the FTS entries
	s.db.Exec(`INSERT INTO messages_fts(messages_fts, rowid, plaintext)
		SELECT 'delete', id, plaintext FROM messages
		WHERE expires_at IS NOT NULL AND expires_at < ?`, time.Now())

	result, err := s.db.Exec(
		"DELETE FROM messages WHERE expires_at IS NOT NULL AND expires_at < ?",
		time.Now(),
	)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

func (s *SQLiteStore) SaveReaction(messageID int64, fromName, emoji string) error {
	_, err := s.db.Exec(
		"INSERT INTO reactions (message_id, from_name, emoji) VALUES (?, ?, ?)",
		messageID, fromName, emoji,
	)
	return err
}

func (s *SQLiteStore) GetReactions(messageIDs []int64) (map[int64][]Reaction, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}

	// Build placeholders
	placeholders := make([]string, len(messageIDs))
	args := make([]interface{}, len(messageIDs))
	for i, id := range messageIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		"SELECT id, message_id, from_name, emoji FROM reactions WHERE message_id IN (%s) ORDER BY created_at ASC",
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64][]Reaction)
	for rows.Next() {
		var r Reaction
		if err := rows.Scan(&r.ID, &r.MessageID, &r.FromName, &r.Emoji); err != nil {
			return nil, err
		}
		result[r.MessageID] = append(result[r.MessageID], r)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var msgs []Message
	for rows.Next() {
		var m Message
		var dir string
		var expiresAt sql.NullTime
		if err := rows.Scan(&m.ID, &m.ConversationID, &dir, &m.SenderName, &m.Plaintext, &m.Timestamp, &expiresAt, &m.ServerID, &m.Status); err != nil {
			return nil, err
		}
		m.Direction = Direction(dir)
		if expiresAt.Valid {
			m.ExpiresAt = expiresAt.Time
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
