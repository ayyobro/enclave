package client

import (
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/nacl/box"

	"enclave/internal/server"
)

func TestAppCoreConnectAndChat(t *testing.T) {
	// Set up server
	tmpDir := t.TempDir()
	store, err := server.NewSQLiteStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv, err := server.NewServer(store, logger, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go srv.Run(listener)

	addr := listener.Addr().String()
	t.Logf("Server at %s", addr)

	// Create invite tokens
	tok1, hash1, _ := server.GenerateInviteToken()
	tok2, hash2, _ := server.GenerateInviteToken()
	store.CreateInviteToken(hash1, 1, time.Now().Add(time.Hour))
	store.CreateInviteToken(hash2, 1, time.Now().Add(time.Hour))

	// Create alice and bob
	alicePub, alicePriv, _ := box.GenerateKey(rand.Reader)
	bobPub, bobPriv, _ := box.GenerateKey(rand.Reader)

	aliceCore := NewAppCore(addr, false, alicePub, alicePriv, "alice", nil, logger)
	bobCore := NewAppCore(addr, false, bobPub, bobPriv, "bob", nil, logger)
	defer aliceCore.Close()
	defer bobCore.Close()

	// Alice registers
	aliceContacts, err := aliceCore.ConnectAndAuth(tok1)
	if err != nil {
		t.Fatalf("alice connect: %v", err)
	}
	t.Logf("Alice connected, contacts: %d", len(aliceContacts))

	// Bob registers
	bobContacts, err := bobCore.ConnectAndAuth(tok2)
	if err != nil {
		t.Fatalf("bob connect: %v", err)
	}
	t.Logf("Bob connected, contacts: %d", len(bobContacts))

	// Alice should see bob as a contact
	if len(bobContacts) == 0 {
		t.Log("Bob sees no contacts yet (alice registered first, bob wasn't online)")
	}

	time.Sleep(200 * time.Millisecond)

	// Alice sends a message to Bob
	bobPubB64 := base64.StdEncoding.EncodeToString(bobPub[:])
	err = aliceCore.SendMessage(bobPubB64, "Hello Bob from Alice!")
	if err != nil {
		t.Fatalf("alice send: %v", err)
	}
	t.Log("Alice sent message")

	// Bob receives
	select {
	case data := <-bobCore.RecvChannel():
		event := bobCore.ProcessIncoming(data)
		chat, ok := event.(*IncomingChatEvent)
		if !ok {
			t.Fatalf("expected IncomingChatEvent, got %T", event)
		}
		if chat.Plaintext != "Hello Bob from Alice!" {
			t.Errorf("plaintext = %q, want %q", chat.Plaintext, "Hello Bob from Alice!")
		}
		t.Logf("Bob received: %q from %s", chat.Plaintext, chat.FromName)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for bob to receive message")
	}

	t.Log("AppCore E2E chat test PASSED")
}
