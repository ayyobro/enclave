package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	case protocol.TypeGroupMessage:
		h.routeGroupChat(msg)
	case protocol.TypeTyping:
		h.routeTyping(msg)
	case protocol.TypeReadReceipt:
		h.routeDirectRelay(msg, protocol.TypeReadReceipt)
	case protocol.TypeReaction:
		h.routeDirectRelay(msg, protocol.TypeReaction)
	case protocol.TypeFileMeta:
		h.routeDirectRelay(msg, protocol.TypeFileMeta)
	case protocol.TypeFileChunk:
		h.routeDirectRelay(msg, protocol.TypeFileChunk)
	case protocol.TypeEphemeral:
		h.routeDirectRelay(msg, protocol.TypeEphemeral)
	case protocol.TypeVibeStart:
		h.routeDirectRelay(msg, protocol.TypeVibeStart)
	case protocol.TypeVibePrompt:
		h.routeDirectRelay(msg, protocol.TypeVibePrompt)
	case protocol.TypeVibeOutput:
		h.routeDirectRelay(msg, protocol.TypeVibeOutput)
	case protocol.TypeVibeEnd:
		h.routeDirectRelay(msg, protocol.TypeVibeEnd)
	case protocol.TypeGroupCreate:
		h.handleGroupCreate(msg)
	case protocol.TypeGroupInvite:
		h.handleGroupInvite(msg)
	case protocol.TypeGroupLeave:
		h.handleGroupLeave(msg)
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

// routeGroupChat forwards a group message to all online members.
func (h *Hub) routeGroupChat(msg clientMessage) {
	var groupMsg protocol.GroupChatMsg
	if err := json.Unmarshal(msg.data, &groupMsg); err != nil {
		h.logger.Warn("invalid group message", "error", err)
		return
	}

	senderKey := base64.StdEncoding.EncodeToString(msg.client.publicKey)
	groupMsg.From = senderKey
	groupMsg.Timestamp = time.Now().Unix()

	// Verify sender is a member
	members, err := h.store.GetGroupMembers(groupMsg.GroupID)
	if err != nil {
		h.logger.Warn("group not found", "group", groupMsg.GroupID)
		return
	}
	isMember := false
	for _, m := range members {
		if m == senderKey {
			isMember = true
			break
		}
	}
	if !isMember {
		h.logger.Warn("non-member tried to send to group", "group", groupMsg.GroupID, "sender", senderKey[:12]+"...")
		return
	}

	data, _ := json.Marshal(groupMsg)

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Deliver to each online recipient (they each get the full message and
	// find their own per-recipient ciphertext inside it)
	for _, memberKey := range members {
		if memberKey == senderKey {
			continue
		}
		if client, ok := h.clients[memberKey]; ok {
			select {
			case client.send <- data:
			default:
			}
		}
		// TODO: offline group message queuing
	}
}

// routeDirectRelay is a generic relay for messages with a "to" field (read receipts, reactions, file chunks).
// If "to" is a group ID, fans out to all group members.
func (h *Hub) routeDirectRelay(msg clientMessage, msgType string) {
	var envelope struct {
		To string `json:"to"`
	}
	if err := json.Unmarshal(msg.data, &envelope); err != nil {
		return
	}

	senderKey := base64.StdEncoding.EncodeToString(msg.client.publicKey)

	var raw map[string]interface{}
	json.Unmarshal(msg.data, &raw)
	raw["from"] = senderKey
	data, _ := json.Marshal(raw)

	// Check if "to" is a group ID
	if members, err := h.store.GetGroupMembers(envelope.To); err == nil && len(members) > 0 {
		// Fan out to all group members except sender
		h.mu.RLock()
		for _, memberKey := range members {
			if memberKey == senderKey {
				continue
			}
			if client, ok := h.clients[memberKey]; ok {
				select {
				case client.send <- data:
				default:
				}
			}
		}
		h.mu.RUnlock()
		return
	}

	// Direct message — route to single recipient
	h.mu.RLock()
	recipient, online := h.clients[envelope.To]
	h.mu.RUnlock()

	if online {
		select {
		case recipient.send <- data:
		default:
		}
	}
}

func (h *Hub) handleGroupCreate(msg clientMessage) {
	var create protocol.GroupCreateMsg
	if err := json.Unmarshal(msg.data, &create); err != nil {
		return
	}

	senderKey := base64.StdEncoding.EncodeToString(msg.client.publicKey)
	groupID := fmt.Sprintf("g_%d", time.Now().UnixNano())

	// Ensure creator is in members list
	members := append(create.Members, senderKey)
	seen := make(map[string]bool)
	var unique []string
	for _, m := range members {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}

	if err := h.store.CreateGroup(groupID, create.Name, senderKey, unique); err != nil {
		h.logger.Error("creating group", "error", err)
		return
	}

	h.logger.Info("group created", "id", groupID, "name", create.Name, "members", len(unique))

	// Notify all members
	created := protocol.GroupCreatedMsg{
		Type:    protocol.TypeGroupCreated,
		GroupID: groupID,
		Name:    create.Name,
		Members: unique,
		Creator: senderKey,
	}
	data, _ := json.Marshal(created)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, memberKey := range unique {
		if client, ok := h.clients[memberKey]; ok {
			select {
			case client.send <- data:
			default:
			}
		}
	}
}

func (h *Hub) handleGroupInvite(msg clientMessage) {
	var invite protocol.GroupInviteMsg
	if err := json.Unmarshal(msg.data, &invite); err != nil {
		return
	}

	if err := h.store.AddGroupMember(invite.GroupID, invite.Member); err != nil {
		h.logger.Error("adding group member", "error", err)
		return
	}

	// Get group info to notify the new member
	group, err := h.store.GetGroup(invite.GroupID)
	if err != nil {
		return
	}
	members, _ := h.store.GetGroupMembers(invite.GroupID)

	created := protocol.GroupCreatedMsg{
		Type:    protocol.TypeGroupCreated,
		GroupID: group.ID,
		Name:    group.Name,
		Members: members,
		Creator: group.Creator,
	}
	data, _ := json.Marshal(created)

	h.mu.RLock()
	if client, ok := h.clients[invite.Member]; ok {
		select {
		case client.send <- data:
		default:
		}
	}
	h.mu.RUnlock()
}

func (h *Hub) handleGroupLeave(msg clientMessage) {
	var leave protocol.GroupLeaveMsg
	if err := json.Unmarshal(msg.data, &leave); err != nil {
		return
	}

	senderKey := base64.StdEncoding.EncodeToString(msg.client.publicKey)
	h.store.RemoveGroupMember(leave.GroupID, senderKey)
	h.logger.Info("user left group", "group", leave.GroupID, "user", senderKey[:12]+"...")
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

// UserGroups returns the groups a user belongs to.
func (h *Hub) UserGroups(pubKeyB64 string) []protocol.GroupInfo {
	groups, err := h.store.GetUserGroups(pubKeyB64)
	if err != nil {
		return nil
	}

	var infos []protocol.GroupInfo
	for _, g := range groups {
		members, _ := h.store.GetGroupMembers(g.ID)
		infos = append(infos, protocol.GroupInfo{
			GroupID: g.ID,
			Name:    g.Name,
			Members: members,
		})
	}
	return infos
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
