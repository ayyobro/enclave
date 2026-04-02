package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestConversationCRUD(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	// Create conversation
	c, err := s.GetOrCreateConversation("key-alice", "alice")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if c.DisplayName != "alice" {
		t.Errorf("name = %q, want alice", c.DisplayName)
	}

	// Get same conversation again
	c2, err := s.GetOrCreateConversation("key-alice", "alice")
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if c2.ID != c.ID {
		t.Errorf("expected same ID, got %d and %d", c.ID, c2.ID)
	}

	// Update display name
	c3, err := s.GetOrCreateConversation("key-alice", "Alice B")
	if err != nil {
		t.Fatalf("update name: %v", err)
	}
	if c3.DisplayName != "Alice B" {
		t.Errorf("name = %q, want 'Alice B'", c3.DisplayName)
	}

	// List conversations
	convos, err := s.ListConversations()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(convos) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(convos))
	}
}

func TestMessagePersistence(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c, _ := s.GetOrCreateConversation("key-bob", "bob")

	// Save messages
	now := time.Now()
	m1, err := s.SaveMessage(c.ID, Sent, "me", "Hello Bob", now.Add(-2*time.Minute), 0)
	if err != nil {
		t.Fatalf("save m1: %v", err)
	}
	m2, err := s.SaveMessage(c.ID, Received, "bob", "Hey Alice!", now.Add(-1*time.Minute), 101)
	if err != nil {
		t.Fatalf("save m2: %v", err)
	}
	m3, err := s.SaveMessage(c.ID, Sent, "me", "How's it going?", now, 0)
	if err != nil {
		t.Fatalf("save m3: %v", err)
	}

	_ = m1
	_ = m2
	_ = m3

	// Get all messages
	msgs, err := s.GetMessages(c.ID, 100, 0)
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Plaintext != "Hello Bob" {
		t.Errorf("first message = %q", msgs[0].Plaintext)
	}
	if msgs[1].Direction != Received {
		t.Errorf("second message direction = %q, want received", msgs[1].Direction)
	}

	// Get recent (last 2)
	recent, err := s.GetRecentMessages(c.ID, 2)
	if err != nil {
		t.Fatalf("get recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent, got %d", len(recent))
	}
	// Should be in chronological order
	if recent[0].Plaintext != "Hey Alice!" {
		t.Errorf("recent[0] = %q, want 'Hey Alice!'", recent[0].Plaintext)
	}

	// Conversation last_message should be updated
	convos, _ := s.ListConversations()
	if convos[0].LastMessage != "How's it going?" {
		t.Errorf("last_message = %q", convos[0].LastMessage)
	}
}

func TestDeduplication(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c, _ := s.GetOrCreateConversation("key-bob", "bob")
	s.SaveMessage(c.ID, Received, "bob", "hello", time.Now(), 42)

	has, err := s.HasServerMessage(42)
	if err != nil {
		t.Fatalf("has: %v", err)
	}
	if !has {
		t.Error("expected to find server message 42")
	}

	has, _ = s.HasServerMessage(99)
	if has {
		t.Error("should not find server message 99")
	}
}

func TestUnreadTracking(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c, _ := s.GetOrCreateConversation("key-bob", "bob")

	s.IncrementUnread(c.ID)
	s.IncrementUnread(c.ID)
	s.IncrementUnread(c.ID)

	c2, _ := s.GetOrCreateConversation("key-bob", "bob")
	if c2.UnreadCount != 3 {
		t.Errorf("unread = %d, want 3", c2.UnreadCount)
	}

	s.ClearUnread(c.ID)
	c3, _ := s.GetOrCreateConversation("key-bob", "bob")
	if c3.UnreadCount != 0 {
		t.Errorf("unread after clear = %d, want 0", c3.UnreadCount)
	}
}

func TestSearch(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c1, _ := s.GetOrCreateConversation("key-alice", "alice")
	c2, _ := s.GetOrCreateConversation("key-bob", "bob")

	s.SaveMessage(c1.ID, Sent, "me", "The deployment is ready", time.Now(), 0)
	s.SaveMessage(c1.ID, Received, "alice", "Great, let me check the staging server", time.Now(), 0)
	s.SaveMessage(c2.ID, Sent, "me", "Can you review the pull request?", time.Now(), 0)
	s.SaveMessage(c2.ID, Received, "bob", "The server looks good to me", time.Now(), 0)

	// Search for "server"
	results, err := s.SearchMessages("server", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'server', got %d", len(results))
	}

	// Search for "deployment"
	results, err = s.SearchMessages("deployment", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'deployment', got %d", len(results))
	}
	if results[0].DisplayName != "alice" {
		t.Errorf("result contact = %q, want alice", results[0].DisplayName)
	}
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	return s
}
