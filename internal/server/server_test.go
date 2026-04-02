package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/nacl/box"
	"nhooyr.io/websocket"

	"enclave/internal/protocol"
)

func TestEndToEndEncryptedChat(t *testing.T) {
	// Set up temp directory for server data
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	defer store.Close()

	// Create server
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv, err := NewServer(store, logger, tmpDir)
	if err != nil {
		t.Fatalf("creating server: %v", err)
	}

	// Start server on random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	defer listener.Close()

	go srv.hub.Run()
	go http.Serve(listener, srv.mux)

	serverAddr := fmt.Sprintf("ws://%s/ws", listener.Addr().String())
	t.Logf("Test server at %s", serverAddr)

	// Generate keypairs for alice and bob
	alicePub, alicePriv, _ := box.GenerateKey(rand.Reader)
	bobPub, bobPriv, _ := box.GenerateKey(rand.Reader)

	// Generate invite tokens
	token1, hash1, _ := GenerateInviteToken()
	token2, hash2, _ := GenerateInviteToken()
	store.CreateInviteToken(hash1, 1, time.Now().Add(time.Hour))
	store.CreateInviteToken(hash2, 1, time.Now().Add(time.Hour))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// === Alice registers and connects ===
	aliceConn := register(t, ctx, serverAddr, alicePub, alicePriv, "alice", token1)
	defer aliceConn.Close(websocket.StatusNormalClosure, "")

	// === Bob registers and connects ===
	bobConn := register(t, ctx, serverAddr, bobPub, bobPriv, "bob", token2)
	defer bobConn.Close(websocket.StatusNormalClosure, "")

	// Wait for presence updates to propagate
	time.Sleep(100 * time.Millisecond)

	// === Alice sends encrypted message to Bob ===
	plaintext := []byte("Hello Bob, this is a top secret message!")

	// Alice encrypts with Bob's public key
	var nonce [24]byte
	rand.Read(nonce[:])
	ciphertext := box.Seal(nil, plaintext, &nonce, bobPub, alicePriv)

	chatMsg := protocol.ChatMsg{
		Type:       protocol.TypeMessage,
		To:         base64.StdEncoding.EncodeToString(bobPub[:]),
		Nonce:      base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	data, _ := json.Marshal(chatMsg)
	err = aliceConn.Write(ctx, websocket.MessageText, data)
	if err != nil {
		t.Fatalf("alice writing message: %v", err)
	}
	t.Log("Alice sent encrypted message")

	// === Bob receives and decrypts ===
	// Bob might receive presence messages first, loop until we get a chat message
	var receivedChat protocol.ChatMsg
	for {
		_, msgData, err := bobConn.Read(ctx)
		if err != nil {
			t.Fatalf("bob reading: %v", err)
		}

		msgType, _ := protocol.ParseType(msgData)
		if msgType == protocol.TypeMessage {
			json.Unmarshal(msgData, &receivedChat)
			break
		}
		t.Logf("Bob got non-message type: %s", msgType)
	}

	// Verify the from field was set by the server
	if receivedChat.From != base64.StdEncoding.EncodeToString(alicePub[:]) {
		t.Errorf("from field = %s, want alice's key", receivedChat.From)
	}

	// Decrypt the message
	ctBytes, _ := base64.StdEncoding.DecodeString(receivedChat.Ciphertext)
	nonceBytes, _ := base64.StdEncoding.DecodeString(receivedChat.Nonce)
	var recvNonce [24]byte
	copy(recvNonce[:], nonceBytes)

	decrypted, ok := box.Open(nil, ctBytes, &recvNonce, alicePub, bobPriv)
	if !ok {
		t.Fatal("Bob failed to decrypt alice's message")
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}

	t.Logf("Bob decrypted: %q", decrypted)
	t.Log("End-to-end encrypted chat test PASSED")
}

func TestOfflineDelivery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	defer store.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv, err := NewServer(store, logger, tmpDir)
	if err != nil {
		t.Fatalf("creating server: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	defer listener.Close()

	go srv.hub.Run()
	go http.Serve(listener, srv.mux)

	serverAddr := fmt.Sprintf("ws://%s/ws", listener.Addr().String())

	alicePub, alicePriv, _ := box.GenerateKey(rand.Reader)
	bobPub, bobPriv, _ := box.GenerateKey(rand.Reader)

	token1, hash1, _ := GenerateInviteToken()
	token2, hash2, _ := GenerateInviteToken()
	store.CreateInviteToken(hash1, 1, time.Now().Add(time.Hour))
	store.CreateInviteToken(hash2, 1, time.Now().Add(time.Hour))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Register both users, but only alice connects
	aliceConn := register(t, ctx, serverAddr, alicePub, alicePriv, "alice", token1)
	defer aliceConn.Close(websocket.StatusNormalClosure, "")

	// Register bob (connect briefly then disconnect)
	bobConn := register(t, ctx, serverAddr, bobPub, bobPriv, "bob", token2)
	bobConn.Close(websocket.StatusNormalClosure, "done")
	time.Sleep(100 * time.Millisecond) // let server process disconnect

	// Alice sends message to offline Bob
	plaintext := []byte("Bob, check this when you're back online!")
	var nonce [24]byte
	rand.Read(nonce[:])
	ciphertext := box.Seal(nil, plaintext, &nonce, bobPub, alicePriv)

	chatMsg := protocol.ChatMsg{
		Type:       protocol.TypeMessage,
		To:         base64.StdEncoding.EncodeToString(bobPub[:]),
		Nonce:      base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	data, _ := json.Marshal(chatMsg)
	aliceConn.Write(ctx, websocket.MessageText, data)
	t.Log("Alice sent message to offline Bob")
	time.Sleep(500 * time.Millisecond)

	// Verify pending message is stored
	pending, err := store.GetPendingMessages(bobPub[:])
	if err != nil {
		t.Fatalf("getting pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending message, got %d", len(pending))
	}
	t.Log("Message stored for offline delivery")

	// Bob reconnects (auth this time, already registered)
	bobConn2 := authenticate(t, ctx, serverAddr, bobPub, bobPriv, srv.serverPub)
	defer bobConn2.Close(websocket.StatusNormalClosure, "")

	// Bob should receive the pending message
	var receivedChat protocol.ChatMsg
	for {
		_, msgData, err := bobConn2.Read(ctx)
		if err != nil {
			t.Fatalf("bob reading: %v", err)
		}
		msgType, _ := protocol.ParseType(msgData)
		if msgType == protocol.TypeMessage {
			json.Unmarshal(msgData, &receivedChat)
			break
		}
	}

	ctBytes, _ := base64.StdEncoding.DecodeString(receivedChat.Ciphertext)
	nonceBytes, _ := base64.StdEncoding.DecodeString(receivedChat.Nonce)
	var recvNonce [24]byte
	copy(recvNonce[:], nonceBytes)

	decrypted, ok := box.Open(nil, ctBytes, &recvNonce, alicePub, bobPriv)
	if !ok {
		t.Fatal("Bob failed to decrypt offline message")
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
	t.Logf("Bob decrypted offline message: %q", decrypted)
	t.Log("Offline delivery test PASSED")
}

func TestAdminInviteAPI(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv, err := NewServer(store, logger, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go srv.Run(listener)

	baseURL := fmt.Sprintf("http://%s", listener.Addr().String())
	adminKey := srv.AdminKey()
	t.Logf("Admin key: %s", adminKey)

	// Test: valid admin key generates a token
	req, _ := http.NewRequest("POST", baseURL+"/api/invite", nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("invite request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Token     string `json:"token"`
		MaxUses   int    `json:"max_uses"`
		ExpiresAt string `json:"expires_at"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if result.MaxUses != 1 {
		t.Errorf("max_uses = %d, want 1", result.MaxUses)
	}
	t.Logf("Generated token: %s (expires %s)", result.Token, result.ExpiresAt)

	// Test: the generated token actually works for registration
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alicePub, alicePriv, _ := box.GenerateKey(rand.Reader)
	_ = alicePriv // not needed for this test

	wsURL := fmt.Sprintf("ws://%s/ws", listener.Addr().String())
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	reg := protocol.RegisterMsg{
		Type:        protocol.TypeRegister,
		Token:       result.Token,
		PublicKey:   base64.StdEncoding.EncodeToString(alicePub[:]),
		DisplayName: "alice",
	}
	data, _ := json.Marshal(reg)
	conn.Write(ctx, websocket.MessageText, data)

	_, respData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}
	msgType, _ := protocol.ParseType(respData)
	if msgType != protocol.TypeAuthOK {
		t.Fatalf("expected auth_ok, got %s: %s", msgType, string(respData))
	}
	t.Log("Token generated via API was accepted for registration")

	// Test: wrong admin key is rejected
	req2, _ := http.NewRequest("POST", baseURL+"/api/invite", nil)
	req2.Header.Set("Authorization", "Bearer wrong-key")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("bad key request: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for bad key, got %d", resp2.StatusCode)
	}
	t.Log("Bad admin key correctly rejected")

	// Test: no auth header is rejected
	req3, _ := http.NewRequest("POST", baseURL+"/api/invite", nil)
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("no auth request: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for no auth, got %d", resp3.StatusCode)
	}
	t.Log("Missing auth correctly rejected")
}

// register connects, sends a register message, receives auth_ok
func register(t *testing.T, ctx context.Context, addr string, pub, priv *[32]byte, name, token string) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.Dial(ctx, addr, nil)
	if err != nil {
		t.Fatalf("dialing %s for %s: %v", addr, name, err)
	}

	reg := protocol.RegisterMsg{
		Type:        protocol.TypeRegister,
		Token:       token,
		PublicKey:   base64.StdEncoding.EncodeToString(pub[:]),
		DisplayName: name,
	}
	data, _ := json.Marshal(reg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("writing register for %s: %v", name, err)
	}

	// Read auth_ok
	_, respData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading auth response for %s: %v", name, err)
	}

	msgType, _ := protocol.ParseType(respData)
	if msgType == protocol.TypeError {
		var errMsg protocol.ErrorMsg
		json.Unmarshal(respData, &errMsg)
		t.Fatalf("registration error for %s: %s - %s", name, errMsg.Code, errMsg.Message)
	}
	if msgType != protocol.TypeAuthOK {
		t.Fatalf("expected auth_ok for %s, got %s", name, msgType)
	}

	t.Logf("%s registered and connected", name)
	return conn
}

// authenticate connects using challenge-response for an already-registered user
func authenticate(t *testing.T, ctx context.Context, addr string, pub, priv, serverPub *[32]byte) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.Dial(ctx, addr, nil)
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}

	authMsg := protocol.AuthMsg{
		Type:      protocol.TypeAuth,
		PublicKey: base64.StdEncoding.EncodeToString(pub[:]),
	}
	data, _ := json.Marshal(authMsg)
	conn.Write(ctx, websocket.MessageText, data)

	// Read challenge
	_, challengeData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading challenge: %v", err)
	}
	var challenge protocol.ChallengeMsg
	json.Unmarshal(challengeData, &challenge)

	challengeBytes, _ := base64.StdEncoding.DecodeString(challenge.Nonce)

	// Encrypt the challenge with our private key and server's public key
	var nonce [24]byte
	rand.Read(nonce[:])
	sealed := box.Seal(nonce[:], challengeBytes, &nonce, serverPub, priv)

	resp := protocol.AuthRespMsg{
		Type:     protocol.TypeAuthResp,
		Response: base64.StdEncoding.EncodeToString(sealed),
	}
	respData, _ := json.Marshal(resp)
	conn.Write(ctx, websocket.MessageText, respData)

	// Read auth_ok
	_, okData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading auth_ok: %v", err)
	}
	msgType, _ := protocol.ParseType(okData)
	if msgType != protocol.TypeAuthOK {
		t.Fatalf("expected auth_ok, got %s: %s", msgType, string(okData))
	}

	return conn
}
