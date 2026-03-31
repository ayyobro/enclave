package client

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/nacl/box"

	"enclave/internal/crypto"
	"enclave/internal/protocol"
)

// Event types returned by ProcessIncoming. The TUI layer reads these
// and converts them to tea.Msg types, breaking the import cycle.

type IncomingChatEvent struct {
	From      string
	FromName  string
	Plaintext string
	Timestamp time.Time
}

type PresenceEvent struct {
	PublicKey string
	Online    bool
}

type TypingEvent struct {
	From string
}

type ErrorEvent struct {
	Err error
}

type ContactEntry struct {
	PublicKey    string
	DisplayName string
	Online      bool
}

// AppCore is the business logic layer that connects the TUI to the network.
type AppCore struct {
	ws       *WSClient
	presence *PresenceTracker
	logger   *slog.Logger

	myPub    *[32]byte
	myPriv   *[32]byte
	myPubB64 string
	myName   string

	contactNames map[string]string

	ctx    context.Context
	cancel context.CancelFunc
}

func NewAppCore(serverAddr string, pub, priv *[32]byte, displayName string, logger *slog.Logger) *AppCore {
	ctx, cancel := context.WithCancel(context.Background())
	return &AppCore{
		ws:           NewWSClient(serverAddr, logger),
		presence:     NewPresenceTracker(),
		logger:       logger,
		myPub:        pub,
		myPriv:       priv,
		myPubB64:     base64.StdEncoding.EncodeToString(pub[:]),
		myName:       displayName,
		contactNames: make(map[string]string),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// ConnectAndAuth connects and registers or authenticates.
func (a *AppCore) ConnectAndAuth(token string) ([]ContactEntry, error) {
	if err := a.ws.Connect(a.ctx); err != nil {
		return nil, err
	}

	var users []protocol.UserInfo
	var err error

	if token != "" {
		users, err = a.ws.Register(a.ctx, a.myPubB64, a.myName, token)
	} else {
		users, err = a.ws.Authenticate(a.ctx, a.myPubB64, a.solveChallenge)
	}
	if err != nil {
		return nil, err
	}

	go a.ws.RunReadLoop(a.ctx)
	go a.ws.RunWriteLoop(a.ctx)

	contacts := make([]ContactEntry, 0, len(users))
	for _, u := range users {
		if u.PublicKey == a.myPubB64 {
			continue
		}
		a.contactNames[u.PublicKey] = u.DisplayName
		a.presence.SetOnline(u.PublicKey, u.Online)
		contacts = append(contacts, ContactEntry{
			PublicKey:   u.PublicKey,
			DisplayName: u.DisplayName,
			Online:      u.Online,
		})
	}

	return contacts, nil
}

func (a *AppCore) solveChallenge(challengeNonce []byte, serverPubKeyBytes []byte) ([]byte, error) {
	var serverPub [32]byte
	if len(serverPubKeyBytes) != 32 {
		return nil, fmt.Errorf("invalid server public key size: %d", len(serverPubKeyBytes))
	}
	copy(serverPub[:], serverPubKeyBytes)

	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}

	sealed := box.Seal(nonce[:], challengeNonce, &nonce, &serverPub, a.myPriv)
	return sealed, nil
}

// SendMessage encrypts and sends a message.
func (a *AppCore) SendMessage(toPubKeyB64, plaintext string) error {
	peerPub, err := crypto.PubKeyFromBase64(toPubKeyB64)
	if err != nil {
		return fmt.Errorf("invalid recipient key: %w", err)
	}

	nonce, ciphertext, err := crypto.SealMessage([]byte(plaintext), peerPub, a.myPriv)
	if err != nil {
		return fmt.Errorf("encrypting message: %w", err)
	}

	msg := protocol.ChatMsg{
		Type:       protocol.TypeMessage,
		To:         toPubKeyB64,
		Nonce:      base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	data, _ := json.Marshal(msg)
	return a.ws.Send(data)
}

// SendTyping sends a typing indicator.
func (a *AppCore) SendTyping(toPubKeyB64 string) {
	msg := protocol.TypingMsg{
		Type: protocol.TypeTyping,
		To:   toPubKeyB64,
	}
	data, _ := json.Marshal(msg)
	a.ws.Send(data)
}

// ProcessIncoming decodes and processes a raw server message.
// Returns one of: *IncomingChatEvent, *PresenceEvent, *TypingEvent, *ErrorEvent, or nil.
func (a *AppCore) ProcessIncoming(data []byte) interface{} {
	msgType, err := protocol.ParseType(data)
	if err != nil {
		a.logger.Warn("invalid incoming message", "error", err)
		return nil
	}

	switch msgType {
	case protocol.TypeMessage:
		return a.processChat(data)
	case protocol.TypePresence:
		return a.processPresence(data)
	case protocol.TypeTyping:
		return a.processTyping(data)
	case protocol.TypeError:
		var errMsg protocol.ErrorMsg
		json.Unmarshal(data, &errMsg)
		return &ErrorEvent{Err: fmt.Errorf("%s: %s", errMsg.Code, errMsg.Message)}
	default:
		a.logger.Debug("unhandled message type", "type", msgType)
		return nil
	}
}

func (a *AppCore) processChat(data []byte) interface{} {
	var msg protocol.ChatMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}

	peerPub, err := crypto.PubKeyFromBase64(msg.From)
	if err != nil {
		return nil
	}

	nonceBytes, err := base64.StdEncoding.DecodeString(msg.Nonce)
	if err != nil || len(nonceBytes) != 24 {
		return nil
	}
	var nonce [24]byte
	copy(nonce[:], nonceBytes)

	ctBytes, err := base64.StdEncoding.DecodeString(msg.Ciphertext)
	if err != nil {
		return nil
	}

	plaintext, err := crypto.OpenMessage(ctBytes, &nonce, peerPub, a.myPriv)
	if err != nil {
		a.logger.Warn("decryption failed", "from", msg.From[:12]+"...", "error", err)
		return nil
	}

	fromName := a.contactNames[msg.From]
	if fromName == "" {
		fromName = msg.From[:12] + "..."
	}

	ts := time.Now()
	if msg.Timestamp > 0 {
		ts = time.Unix(msg.Timestamp, 0)
	}

	return &IncomingChatEvent{
		From:      msg.From,
		FromName:  fromName,
		Plaintext: string(plaintext),
		Timestamp: ts,
	}
}

func (a *AppCore) processPresence(data []byte) interface{} {
	var msg protocol.PresenceMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	a.presence.SetOnline(msg.PublicKey, msg.Online)
	return &PresenceEvent{PublicKey: msg.PublicKey, Online: msg.Online}
}

func (a *AppCore) processTyping(data []byte) interface{} {
	var msg protocol.TypingMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	return &TypingEvent{From: msg.From}
}

// RecvChannel returns the receive channel for incoming messages.
func (a *AppCore) RecvChannel() <-chan []byte {
	return a.ws.RecvCh
}

// IsConnected returns connection status.
func (a *AppCore) IsConnected() bool {
	return a.ws.IsConnected()
}

// ContactName returns the display name for a public key.
func (a *AppCore) ContactName(pubKeyB64 string) string {
	return a.contactNames[pubKeyB64]
}

// Close shuts down the connection.
func (a *AppCore) Close() {
	a.cancel()
	a.ws.Close()
}
