package server

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"enclave/internal/protocol"
)

// Hub maintains the set of active clients and routes messages between them.
type Hub struct {
	// clients maps base64-encoded public keys to their connections.
	clients map[string]*Client
	mu      sync.RWMutex

	store  Store
	logger *slog.Logger

	register   chan *Client
	unregister chan *Client
	incoming   chan clientMessage
}

// clientMessage pairs a raw message with the client that sent it.
type clientMessage struct {
	client *Client
	data   []byte
}

func NewHub(store Store, logger *slog.Logger) *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		store:      store,
		logger:     logger,
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
		incoming:   make(chan clientMessage, 256),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.handleRegister(client)
		case client := <-h.unregister:
			h.handleUnregister(client)
		case msg := <-h.incoming:
			h.handleMessage(msg)
		}
	}
}

func (h *Hub) handleRegister(c *Client) {
	key := base64.StdEncoding.EncodeToString(c.publicKey)
	h.mu.Lock()
	h.clients[key] = c
	h.mu.Unlock()

	h.logger.Info("client connected", "user", c.displayName, "key", key[:12]+"...")

	// Broadcast presence
	h.broadcastPresence(key, true)

	// Deliver pending messages
	h.deliverPending(c)
}

func (h *Hub) handleUnregister(c *Client) {
	key := base64.StdEncoding.EncodeToString(c.publicKey)
	h.mu.Lock()
	if existing, ok := h.clients[key]; ok && existing == c {
		delete(h.clients, key)
	}
	h.mu.Unlock()

	h.logger.Info("client disconnected", "user", c.displayName, "key", key[:12]+"...")
	h.broadcastPresence(key, false)
}

func (h *Hub) handleMessage(msg clientMessage) {
	msgType, err := protocol.ParseType(msg.data)
	if err != nil {
		h.logger.Warn("invalid message envelope", "error", err)
		return
	}

	switch msgType {
	case protocol.TypeMessage:
		h.routeChat(msg)
	case protocol.TypeTyping:
		h.routeTyping(msg)
	default:
		h.logger.Warn("unknown message type in hub", "type", msgType)
	}
}

func (h *Hub) routeChat(msg clientMessage) {
	var chat protocol.ChatMsg
	if err := json.Unmarshal(msg.data, &chat); err != nil {
		h.logger.Warn("invalid chat message", "error", err)
		return
	}

	// Set the from field to the authenticated sender's key
	senderKey := base64.StdEncoding.EncodeToString(msg.client.publicKey)
	chat.From = senderKey
	chat.Timestamp = time.Now().Unix()

	h.mu.RLock()
	recipient, online := h.clients[chat.To]
	h.mu.RUnlock()

	if online {
		// Deliver immediately
		data, err := json.Marshal(chat)
		if err != nil {
			h.logger.Error("marshaling chat message", "error", err)
			return
		}
		select {
		case recipient.send <- data:
		default:
			h.logger.Warn("recipient send buffer full", "to", chat.To[:12]+"...")
		}
	} else {
		// Store for offline delivery
		recipientKeyBytes, err := base64.StdEncoding.DecodeString(chat.To)
		if err != nil {
			h.logger.Warn("invalid recipient key", "error", err)
			return
		}
		nonceBytes, err := base64.StdEncoding.DecodeString(chat.Nonce)
		if err != nil {
			h.logger.Warn("invalid nonce", "error", err)
			return
		}
		ctBytes, err := base64.StdEncoding.DecodeString(chat.Ciphertext)
		if err != nil {
			h.logger.Warn("invalid ciphertext", "error", err)
			return
		}

		id, err := h.store.StorePendingMessage(msg.client.publicKey, recipientKeyBytes, nonceBytes, ctBytes)
		if err != nil {
			h.logger.Error("storing pending message", "error", err)
			return
		}
		h.logger.Debug("stored pending message", "id", id, "to", chat.To[:12]+"...")
	}
}

func (h *Hub) routeTyping(msg clientMessage) {
	var typing protocol.TypingMsg
	if err := json.Unmarshal(msg.data, &typing); err != nil {
		return
	}

	senderKey := base64.StdEncoding.EncodeToString(msg.client.publicKey)
	typing.From = senderKey

	h.mu.RLock()
	recipient, online := h.clients[typing.To]
	h.mu.RUnlock()

	if online {
		data, _ := json.Marshal(typing)
		select {
		case recipient.send <- data:
		default:
		}
	}
}

func (h *Hub) broadcastPresence(pubKey string, online bool) {
	msg := protocol.PresenceMsg{
		Type:      protocol.TypePresence,
		PublicKey: pubKey,
		Online:    online,
	}
	data, _ := json.Marshal(msg)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for key, c := range h.clients {
		if key == pubKey {
			continue
		}
		select {
		case c.send <- data:
		default:
		}
	}
}

func (h *Hub) deliverPending(c *Client) {
	pending, err := h.store.GetPendingMessages(c.publicKey)
	if err != nil {
		h.logger.Error("fetching pending messages", "error", err)
		return
	}

	for _, pm := range pending {
		chat := protocol.ChatMsg{
			Type:       protocol.TypeMessage,
			From:       base64.StdEncoding.EncodeToString(pm.SenderKey),
			Nonce:      base64.StdEncoding.EncodeToString(pm.Nonce),
			Ciphertext: base64.StdEncoding.EncodeToString(pm.Ciphertext),
			ID:         pm.ID,
			Timestamp:  pm.CreatedAt.Unix(),
		}
		data, err := json.Marshal(chat)
		if err != nil {
			continue
		}
		select {
		case c.send <- data:
			h.store.DeletePendingMessage(pm.ID)
		default:
			h.logger.Warn("send buffer full during pending delivery")
			return
		}
	}

	if len(pending) > 0 {
		h.logger.Info("delivered pending messages", "count", len(pending), "user", c.displayName)
	}
}

// OnlineUsers returns a list of UserInfo for all registered users with their online status.
func (h *Hub) OnlineUsers() []protocol.UserInfo {
	users, err := h.store.ListUsers()
	if err != nil {
		return nil
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	infos := make([]protocol.UserInfo, 0, len(users))
	for _, u := range users {
		key := base64.StdEncoding.EncodeToString(u.PublicKey)
		_, online := h.clients[key]
		infos = append(infos, protocol.UserInfo{
			PublicKey:   key,
			DisplayName: u.DisplayName,
			Online:      online,
		})
	}
	return infos
}
