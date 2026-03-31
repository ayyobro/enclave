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
	"time"

	"golang.org/x/crypto/nacl/box"
	"nhooyr.io/websocket"

	"enclave/internal/protocol"
)

// Server is the Enclave relay server.
type Server struct {
	hub    *Hub
	store  Store
	logger *slog.Logger
	mux    *http.ServeMux

	// Server's own keypair for challenge-response auth
	serverPub  *[32]byte
	serverPriv *[32]byte

	// Admin key for authenticated API endpoints (invite generation)
	adminKey string

	// Pending auth challenges (clientPubKey -> challenge)
	challenges map[string][32]byte
}

// NewServer creates a new Enclave server.
func NewServer(store Store, logger *slog.Logger, dataDir string) (*Server, error) {
	pub, priv, err := loadOrGenerateServerKeys(dataDir)
	if err != nil {
		return nil, fmt.Errorf("server keys: %w", err)
	}

	adminKey, err := loadOrGenerateAdminKey(dataDir)
	if err != nil {
		return nil, fmt.Errorf("admin key: %w", err)
	}

	hub := NewHub(store, logger)

	s := &Server{
		hub:        hub,
		store:      store,
		logger:     logger,
		mux:        http.NewServeMux(),
		serverPub:  pub,
		serverPriv: priv,
		adminKey:   adminKey,
		challenges: make(map[string][32]byte),
	}

	s.mux.HandleFunc("/ws", s.handleWebSocket)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/invite", s.handleCreateInvite)

	return s, nil
}

// AdminKey returns the admin key so it can be displayed on startup.
func (s *Server) AdminKey() string {
	return s.adminKey
}

// Start begins listening and serving on the given address.
func (s *Server) Start(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	s.logger.Info("enclave server started",
		"address", listener.Addr().String(),
		"server_key", base64.StdEncoding.EncodeToString(s.serverPub[:])[:16]+"...",
	)

	return s.Run(listener)
}

// Run starts the hub and serves on an existing listener.
func (s *Server) Run(listener net.Listener) error {
	go s.hub.Run()
	return http.Serve(listener, s.mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": "0.1.0",
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.logger.Error("websocket accept", "error", err)
		return
	}
	defer conn.CloseNow()

	ctx := r.Context()
	s.logger.Debug("new websocket connection", "remote", r.RemoteAddr)

	// First message must be auth or register
	_, data, err := conn.Read(ctx)
	if err != nil {
		s.logger.Debug("reading initial message", "error", err)
		return
	}

	msgType, err := protocol.ParseType(data)
	if err != nil {
		s.sendError(ctx, conn, "invalid_message", "could not parse message type")
		return
	}

	switch msgType {
	case protocol.TypeRegister:
		s.handleRegister(ctx, conn, data)
	case protocol.TypeAuth:
		s.handleAuth(ctx, conn, data)
	default:
		s.sendError(ctx, conn, "auth_required", "first message must be 'register' or 'auth'")
	}
}

func (s *Server) handleRegister(ctx context.Context, conn *websocket.Conn, data []byte) {
	var msg protocol.RegisterMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		s.sendError(ctx, conn, "invalid_message", "invalid register message")
		return
	}

	if len(msg.DisplayName) == 0 || len(msg.DisplayName) > protocol.MaxDisplayName {
		s.sendError(ctx, conn, "invalid_name", "display name must be 1-32 characters")
		return
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(msg.PublicKey)
	if err != nil || len(pubKeyBytes) != 32 {
		s.sendError(ctx, conn, "invalid_key", "invalid public key")
		return
	}

	// Validate invite token
	tokenHash, err := HashInviteToken(msg.Token)
	if err != nil {
		s.sendError(ctx, conn, "invalid_token", "invalid invite token format")
		return
	}

	if err := s.store.ValidateAndUseInvite(tokenHash); err != nil {
		s.sendError(ctx, conn, "invalid_token", err.Error())
		return
	}

	// Create user
	user, err := s.store.CreateUser(pubKeyBytes, msg.DisplayName)
	if err != nil {
		s.sendError(ctx, conn, "registration_failed", "could not create user: "+err.Error())
		return
	}

	s.logger.Info("new user registered", "name", user.DisplayName)

	// Now proceed to authenticate this connection
	s.authenticateConnection(ctx, conn, pubKeyBytes, msg.DisplayName)
}

func (s *Server) handleAuth(ctx context.Context, conn *websocket.Conn, data []byte) {
	var msg protocol.AuthMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		s.sendError(ctx, conn, "invalid_message", "invalid auth message")
		return
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(msg.PublicKey)
	if err != nil || len(pubKeyBytes) != 32 {
		s.sendError(ctx, conn, "invalid_key", "invalid public key")
		return
	}

	// Look up user
	user, err := s.store.GetUserByKey(pubKeyBytes)
	if err != nil {
		s.sendError(ctx, conn, "unknown_user", "public key not registered")
		return
	}

	// Send challenge
	challenge, err := GenerateChallenge()
	if err != nil {
		s.sendError(ctx, conn, "server_error", "could not generate challenge")
		return
	}

	challengeMsg := protocol.ChallengeMsg{
		Type:      protocol.TypeChallenge,
		Nonce:     base64.StdEncoding.EncodeToString(challenge[:]),
		ServerKey: base64.StdEncoding.EncodeToString(s.serverPub[:]),
	}
	challengeData, _ := json.Marshal(challengeMsg)
	if err := conn.Write(ctx, websocket.MessageText, challengeData); err != nil {
		return
	}

	// Wait for auth response
	_, respData, err := conn.Read(ctx)
	if err != nil {
		return
	}

	var resp protocol.AuthRespMsg
	if err := json.Unmarshal(respData, &resp); err != nil {
		s.sendError(ctx, conn, "invalid_message", "invalid auth response")
		return
	}

	responseBytes, err := base64.StdEncoding.DecodeString(resp.Response)
	if err != nil {
		s.sendError(ctx, conn, "invalid_response", "could not decode response")
		return
	}

	var clientPub [32]byte
	copy(clientPub[:], pubKeyBytes)

	ok, err := VerifyChallengeResponse(responseBytes, challenge, &clientPub, s.serverPriv)
	if err != nil || !ok {
		s.sendAuthFail(ctx, conn, "challenge verification failed")
		return
	}

	s.authenticateConnection(ctx, conn, pubKeyBytes, user.DisplayName)
}

func (s *Server) authenticateConnection(ctx context.Context, conn *websocket.Conn, pubKey []byte, displayName string) {
	// Send auth_ok with user list
	authOK := protocol.AuthOKMsg{
		Type:  protocol.TypeAuthOK,
		Users: s.hub.OnlineUsers(),
	}
	data, _ := json.Marshal(authOK)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return
	}

	// Create client and register with hub
	client := NewClient(s.hub, conn, pubKey, displayName, s.logger)
	s.hub.register <- client

	// Run read/write pumps
	go client.WritePump(ctx)
	client.ReadPump(ctx) // blocks until disconnect
}

func (s *Server) sendError(ctx context.Context, conn *websocket.Conn, code, message string) {
	msg := protocol.ErrorMsg{
		Type:    protocol.TypeError,
		Code:    code,
		Message: message,
	}
	data, _ := json.Marshal(msg)
	conn.Write(ctx, websocket.MessageText, data)
}

func (s *Server) sendAuthFail(ctx context.Context, conn *websocket.Conn, reason string) {
	msg := protocol.AuthFailMsg{
		Type:   protocol.TypeAuthFail,
		Reason: reason,
	}
	data, _ := json.Marshal(msg)
	conn.Write(ctx, websocket.MessageText, data)
}

// handleCreateInvite generates an invite token via the admin API.
// Requires: Authorization: Bearer <admin-key>
func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	// Validate admin key
	auth := r.Header.Get("Authorization")
	if auth == "" || len(auth) < 8 || auth[:7] != "Bearer " {
		http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
		return
	}
	if auth[7:] != s.adminKey {
		http.Error(w, `{"error":"invalid admin key"}`, http.StatusForbidden)
		return
	}

	// Parse request body
	var req struct {
		MaxUses   int    `json:"max_uses"`
		ExpiresIn string `json:"expires_in"` // duration string like "72h"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use defaults if no body
		req.MaxUses = 1
		req.ExpiresIn = "72h"
	}
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	if req.ExpiresIn == "" {
		req.ExpiresIn = "72h"
	}

	duration, err := time.ParseDuration(req.ExpiresIn)
	if err != nil {
		http.Error(w, `{"error":"invalid expires_in duration"}`, http.StatusBadRequest)
		return
	}

	token, tokenHash, err := GenerateInviteToken()
	if err != nil {
		s.logger.Error("generating invite token", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(duration)
	if err := s.store.CreateInviteToken(tokenHash, req.MaxUses, expiresAt); err != nil {
		s.logger.Error("storing invite token", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	s.logger.Info("invite token generated via API", "max_uses", req.MaxUses, "expires", expiresAt.Format(time.RFC3339))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"max_uses":   req.MaxUses,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

func loadOrGenerateAdminKey(dataDir string) (string, error) {
	keyPath := filepath.Join(dataDir, "admin.key")

	data, err := os.ReadFile(keyPath)
	if err == nil && len(data) > 0 {
		return string(data), nil
	}

	// Generate a new admin key
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := base64.URLEncoding.EncodeToString(raw)

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(keyPath, []byte(key), 0600); err != nil {
		return "", err
	}

	return key, nil
}

func loadOrGenerateServerKeys(dataDir string) (*[32]byte, *[32]byte, error) {
	pubPath := filepath.Join(dataDir, "server.pub")
	privPath := filepath.Join(dataDir, "server.key")

	// Try to load existing keys
	privBytes, err := os.ReadFile(privPath)
	if err == nil {
		pubBytes, err := os.ReadFile(pubPath)
		if err == nil && len(privBytes) == 32 && len(pubBytes) == 32 {
			priv := new([32]byte)
			pub := new([32]byte)
			copy(priv[:], privBytes)
			copy(pub[:], pubBytes)
			return pub, priv, nil
		}
	}

	// Generate new keypair
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(privPath, priv[:], 0600); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(pubPath, pub[:], 0644); err != nil {
		return nil, nil, err
	}

	return pub, priv, nil
}
