package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"enclave/internal/protocol"
)

// WSClient manages the WebSocket connection to the Enclave server.
type WSClient struct {
	serverAddr string
	conn       *websocket.Conn
	mu         sync.Mutex
	logger     *slog.Logger

	RecvCh chan []byte // incoming messages from server
	sendCh chan []byte // outgoing messages to server

	connected  bool
	closedRecv bool // track if RecvCh was already closed
}

func NewWSClient(serverAddr string, logger *slog.Logger) *WSClient {
	return &WSClient{
		serverAddr: serverAddr,
		logger:     logger,
		RecvCh:     make(chan []byte, 256),
		sendCh:     make(chan []byte, 256),
	}
}

// Connect establishes the WebSocket connection.
func (w *WSClient) Connect(ctx context.Context) error {
	url := fmt.Sprintf("ws://%s/ws", w.serverAddr)
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", url, err)
	}

	w.mu.Lock()
	w.conn = conn
	w.connected = true
	w.mu.Unlock()

	return nil
}

// Register sends a registration message and waits for auth_ok.
func (w *WSClient) Register(ctx context.Context, pubKeyB64, displayName, token string) ([]protocol.UserInfo, error) {
	msg := protocol.RegisterMsg{
		Type:        protocol.TypeRegister,
		Token:       token,
		PublicKey:   pubKeyB64,
		DisplayName: displayName,
	}
	data, _ := json.Marshal(msg)
	if err := w.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return nil, fmt.Errorf("sending register: %w", err)
	}

	return w.readAuthOK(ctx)
}

// Authenticate performs challenge-response auth for an already-registered user.
func (w *WSClient) Authenticate(ctx context.Context, pubKeyB64 string, solveChallenge func(challengeNonce []byte, serverPubKey []byte) ([]byte, error)) ([]protocol.UserInfo, error) {
	msg := protocol.AuthMsg{
		Type:      protocol.TypeAuth,
		PublicKey: pubKeyB64,
	}
	data, _ := json.Marshal(msg)
	if err := w.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return nil, fmt.Errorf("sending auth: %w", err)
	}

	// Read challenge
	_, challengeData, err := w.conn.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading challenge: %w", err)
	}

	msgType, _ := protocol.ParseType(challengeData)
	if msgType == protocol.TypeError {
		var errMsg protocol.ErrorMsg
		json.Unmarshal(challengeData, &errMsg)
		return nil, fmt.Errorf("auth error: %s", errMsg.Message)
	}
	if msgType == protocol.TypeAuthFail {
		var fail protocol.AuthFailMsg
		json.Unmarshal(challengeData, &fail)
		return nil, fmt.Errorf("auth failed: %s", fail.Reason)
	}

	var challenge protocol.ChallengeMsg
	if err := json.Unmarshal(challengeData, &challenge); err != nil {
		return nil, fmt.Errorf("parsing challenge: %w", err)
	}

	challengeNonce, err := base64.StdEncoding.DecodeString(challenge.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decoding challenge nonce: %w", err)
	}
	serverKeyBytes, err := base64.StdEncoding.DecodeString(challenge.ServerKey)
	if err != nil {
		return nil, fmt.Errorf("decoding server key: %w", err)
	}

	response, err := solveChallenge(challengeNonce, serverKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("solving challenge: %w", err)
	}

	resp := protocol.AuthRespMsg{
		Type:     protocol.TypeAuthResp,
		Response: base64.StdEncoding.EncodeToString(response),
	}
	respData, _ := json.Marshal(resp)
	if err := w.conn.Write(ctx, websocket.MessageText, respData); err != nil {
		return nil, fmt.Errorf("sending auth response: %w", err)
	}

	return w.readAuthOK(ctx)
}

func (w *WSClient) readAuthOK(ctx context.Context) ([]protocol.UserInfo, error) {
	_, data, err := w.conn.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading auth response: %w", err)
	}

	msgType, _ := protocol.ParseType(data)
	if msgType == protocol.TypeError {
		var errMsg protocol.ErrorMsg
		json.Unmarshal(data, &errMsg)
		return nil, fmt.Errorf("server error: %s - %s", errMsg.Code, errMsg.Message)
	}
	if msgType == protocol.TypeAuthFail {
		var fail protocol.AuthFailMsg
		json.Unmarshal(data, &fail)
		return nil, fmt.Errorf("auth failed: %s", fail.Reason)
	}
	if msgType != protocol.TypeAuthOK {
		return nil, fmt.Errorf("expected auth_ok, got %s", msgType)
	}

	var authOK protocol.AuthOKMsg
	if err := json.Unmarshal(data, &authOK); err != nil {
		return nil, fmt.Errorf("parsing auth_ok: %w", err)
	}

	return authOK.Users, nil
}

// Send queues a message to be sent. Returns error if disconnected.
func (w *WSClient) Send(data []byte) error {
	w.mu.Lock()
	connected := w.connected
	w.mu.Unlock()

	if !connected {
		return fmt.Errorf("not connected to server")
	}

	select {
	case w.sendCh <- data:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// RunReadLoop reads messages from the server and puts them on RecvCh.
// Closes RecvCh when the connection drops so the TUI gets notified.
func (w *WSClient) RunReadLoop(ctx context.Context) {
	defer func() {
		w.mu.Lock()
		w.connected = false
		if !w.closedRecv {
			close(w.RecvCh)
			w.closedRecv = true
		}
		w.mu.Unlock()
	}()

	for {
		_, data, err := w.conn.Read(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
			default:
				w.logger.Debug("read loop error", "error", err)
			}
			return
		}
		select {
		case w.RecvCh <- data:
		case <-ctx.Done():
			return
		}
	}
}

// RunWriteLoop reads from sendCh and writes to the WebSocket.
// Exits when context is canceled or a write fails.
func (w *WSClient) RunWriteLoop(ctx context.Context) {
	for {
		select {
		case data := <-w.sendCh:
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := w.conn.Write(writeCtx, websocket.MessageText, data)
			cancel()
			if err != nil {
				w.logger.Debug("write error", "error", err)
				w.mu.Lock()
				w.connected = false
				w.mu.Unlock()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// IsConnected returns whether the client currently has a connection.
func (w *WSClient) IsConnected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.connected
}

// Close closes the WebSocket connection and signals the TUI.
func (w *WSClient) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn != nil {
		w.conn.Close(websocket.StatusNormalClosure, "bye")
		w.connected = false
	}
	if !w.closedRecv {
		close(w.RecvCh)
		w.closedRecv = true
	}
}
