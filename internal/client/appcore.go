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
	"enclave/internal/store"
)

// Event types returned by ProcessIncoming.

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
	From     string
	FromName string
}

type ErrorEvent struct {
	Err error
}

type ContactEntry struct {
	PublicKey    string
	DisplayName string
	Online      bool
}

// HistoryMessage is a message loaded from local storage.
type HistoryMessage struct {
	Direction store.Direction
	Plaintext string
	Timestamp time.Time
	FromName  string
}

// AppCore is the business logic layer that connects the TUI to the network.
type AppCore struct {
	ws       *WSClient
	presence *PresenceTracker
	msgStore store.MessageStore
	logger   *slog.Logger

	myPub    *[32]byte
	myPriv   *[32]byte
	myPubB64 string
	myName   string

	contactNames map[string]string
	// Cache conversation IDs: peerKeyB64 -> conversation ID
	convoIDs map[string]int64

	ctx    context.Context
	cancel context.CancelFunc
}

func NewAppCore(serverAddr string, useTLS bool, pub, priv *[32]byte, displayName string, msgStore store.MessageStore, logger *slog.Logger) *AppCore {
	ctx, cancel := context.WithCancel(context.Background())
	return &AppCore{
		ws:           NewWSClient(serverAddr, useTLS, logger),
		presence:     NewPresenceTracker(),
		msgStore:     msgStore,
		logger:       logger,
		myPub:        pub,
		myPriv:       priv,
		myPubB64:     base64.StdEncoding.EncodeToString(pub[:]),
		myName:       displayName,
		contactNames: make(map[string]string),
		convoIDs:     make(map[string]int64),
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

		// Ensure conversation exists in local store
		a.ensureConversation(u.PublicKey, u.DisplayName)
	}

	return contacts, nil
}

func (a *AppCore) ensureConversation(peerKey, displayName string) {
	if a.msgStore == nil {
		return
	}
	convo, err := a.msgStore.GetOrCreateConversation(peerKey, displayName)
	if err != nil {
		a.logger.Warn("creating conversation", "error", err)
		return
	}
	a.convoIDs[peerKey] = convo.ID
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

// SendMessage encrypts, sends, and persists a message.
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
	if err := a.ws.Send(data); err != nil {
		return err
	}

	// Persist locally
	a.saveLocal(toPubKeyB64, store.Sent, plaintext, time.Now(), 0)
	return nil
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

	// Deduplicate offline-delivered messages
	if msg.ID > 0 && a.msgStore != nil {
		has, _ := a.msgStore.HasServerMessage(msg.ID)
		if has {
			a.logger.Debug("skipping duplicate message", "server_id", msg.ID)
			return nil
		}
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

	// Persist locally
	a.saveLocal(msg.From, store.Received, string(plaintext), ts, msg.ID)

	return &IncomingChatEvent{
		From:      msg.From,
		FromName:  fromName,
		Plaintext: string(plaintext),
		Timestamp: ts,
	}
}

func (a *AppCore) saveLocal(peerKey string, dir store.Direction, plaintext string, ts time.Time, serverID int64) {
	if a.msgStore == nil {
		return
	}
	convoID, ok := a.convoIDs[peerKey]
	if !ok {
		name := a.contactNames[peerKey]
		if name == "" {
			name = peerKey[:12] + "..."
		}
		a.ensureConversation(peerKey, name)
		convoID = a.convoIDs[peerKey]
	}
	if convoID == 0 {
		return
	}
	_, err := a.msgStore.SaveMessage(convoID, dir, plaintext, ts, serverID)
	if err != nil {
		a.logger.Warn("saving message", "error", err)
	}
}

// LoadHistory returns recent messages for a conversation.
func (a *AppCore) LoadHistory(peerKey string, limit int) ([]HistoryMessage, error) {
	if a.msgStore == nil {
		return nil, nil
	}
	convoID, ok := a.convoIDs[peerKey]
	if !ok {
		return nil, nil
	}
	msgs, err := a.msgStore.GetRecentMessages(convoID, limit)
	if err != nil {
		return nil, err
	}

	result := make([]HistoryMessage, len(msgs))
	for i, m := range msgs {
		fromName := a.myName
		if m.Direction == store.Received {
			fromName = a.contactNames[peerKey]
			if fromName == "" {
				fromName = peerKey[:12] + "..."
			}
		}
		result[i] = HistoryMessage{
			Direction: m.Direction,
			Plaintext: m.Plaintext,
			Timestamp: m.Timestamp,
			FromName:  fromName,
		}
	}
	return result, nil
}

// SearchMessages searches local message history.
func (a *AppCore) SearchMessages(query string, limit int) ([]store.SearchResult, error) {
	if a.msgStore == nil {
		return nil, fmt.Errorf("message store not available")
	}
	return a.msgStore.SearchMessages(query, limit)
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
	fromName := a.contactNames[msg.From]
	if fromName == "" {
		fromName = msg.From[:12] + "..."
	}
	return &TypingEvent{From: msg.From, FromName: fromName}
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

// Close shuts down the connection and the store.
func (a *AppCore) Close() {
	a.cancel()
	a.ws.Close()
	if a.msgStore != nil {
		a.msgStore.Close()
	}
}
