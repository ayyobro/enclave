package client

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/nacl/box"

	"enclave/internal/config"
	"enclave/internal/crypto"
	"enclave/internal/protocol"
	"enclave/internal/store"
)

// Event types returned by ProcessIncoming.

type IncomingChatEvent struct {
	DBMessageID int64
	From        string
	FromName    string
	Plaintext   string
	Timestamp   time.Time
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

type GroupCreatedEvent struct {
	GroupID string
	Name    string
	Members []string
	Creator string
}

type GroupMessageEvent struct {
	DBMessageID int64
	GroupID     string
	From        string
	FromName    string
	Plaintext   string
	Timestamp   time.Time
}

type ReadReceiptEvent struct {
	From      string
	MessageTS int64
}

type ReactionEvent struct {
	From      string
	FromName  string
	MessageTS int64
	Emoji     string
}

type FileMetaEvent struct {
	From        string
	FromName    string
	FileID      string
	FileName    string
	FileSize    int64
	TotalChunks int
}

type FileChunkEvent struct {
	FileID     string
	ChunkIndex int
	Plaintext  []byte
}

// FileCompleteEvent is emitted when all chunks of a file have been received.
type FileCompleteEvent struct {
	From     string
	FromName string
	FileID   string
	FileName string
	Data     []byte
	SavedTo  string // path where the file was saved
}

// pendingFile tracks an in-progress file transfer.
type pendingFile struct {
	From        string
	FromName    string
	FileName    string
	FileSize    int64
	TotalChunks int
	Chunks      map[int][]byte
}

type EphemeralEvent struct {
	From     string
	FromName string
	Duration string
}

type VibeStartEvent struct {
	From     string
	FromName string
	To       string // conversation key (host pubkey for DM, group ID for groups)
	RepoName string
}

type VibePromptEvent struct {
	From     string
	FromName string
	Prompt   string
}

type VibeOutputEvent struct {
	From   string
	Text   string
	IsDone bool
}

type VibeEndEvent struct {
	From     string
	FromName string
}

type ContactEntry struct {
	PublicKey   string
	DisplayName string
	Online      bool
}

type GroupEntry struct {
	GroupID string
	Name    string
	Members []string
}

// HistoryMessage is a message loaded from local storage.
type HistoryMessage struct {
	DBMessageID int64
	Direction   store.Direction
	Plaintext   string
	Timestamp   time.Time
	FromName    string
	Reactions   []store.Reaction
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
	// Cache conversation IDs: peerKeyB64 or groupID -> conversation ID
	convoIDs map[string]int64
	// Cache group membership: groupID -> member public keys
	groupMembers map[string][]string
	// In-progress file transfers: fileID -> pendingFile
	pendingFiles map[string]*pendingFile
	fileMu       sync.Mutex

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
		groupMembers: make(map[string][]string),
		pendingFiles: make(map[string]*pendingFile),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// ConnectResult holds the data returned after connecting.
type ConnectResult struct {
	Contacts []ContactEntry
	Groups   []GroupEntry
}

// ConnectAndAuth connects and registers or authenticates.
func (a *AppCore) ConnectAndAuth(token string) (*ConnectResult, error) {
	if err := a.ws.Connect(a.ctx); err != nil {
		return nil, err
	}

	var authOK *protocol.AuthOKMsg
	var err error

	if token != "" {
		authOK, err = a.ws.Register(a.ctx, a.myPubB64, a.myName, token)
	} else {
		authOK, err = a.ws.Authenticate(a.ctx, a.myPubB64, a.solveChallenge)
	}
	if err != nil {
		return nil, err
	}

	go a.ws.RunReadLoop(a.ctx)
	go a.ws.RunWriteLoop(a.ctx)

	contacts := make([]ContactEntry, 0, len(authOK.Users))
	for _, u := range authOK.Users {
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
		a.ensureConversation(u.PublicKey, u.DisplayName)
	}

	groups := make([]GroupEntry, 0, len(authOK.Groups))
	for _, g := range authOK.Groups {
		groups = append(groups, GroupEntry{
			GroupID: g.GroupID,
			Name:    g.Name,
			Members: g.Members,
		})
		a.ensureGroupConversation(g.GroupID, g.Name)
		// Cache member names for group display
		a.groupMembers[g.GroupID] = g.Members
	}

	return &ConnectResult{Contacts: contacts, Groups: groups}, nil
}

func (a *AppCore) ensureGroupConversation(groupID, groupName string) {
	if a.msgStore == nil {
		return
	}
	convo, err := a.msgStore.GetOrCreateGroupConversation(groupID, groupName)
	if err != nil {
		a.logger.Warn("creating group conversation", "error", err)
		return
	}
	a.convoIDs[groupID] = convo.ID
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

// SendMessage encrypts, sends, and persists a message. Returns the local DB message ID.
func (a *AppCore) SendMessage(toPubKeyB64, plaintext string) (int64, error) {
	peerPub, err := crypto.PubKeyFromBase64(toPubKeyB64)
	if err != nil {
		return 0, fmt.Errorf("invalid recipient key: %w", err)
	}

	nonce, ciphertext, err := crypto.SealMessage([]byte(plaintext), peerPub, a.myPriv)
	if err != nil {
		return 0, fmt.Errorf("encrypting message: %w", err)
	}

	msg := protocol.ChatMsg{
		Type:       protocol.TypeMessage,
		To:         toPubKeyB64,
		Nonce:      base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("marshal: %w", err)
	}
	if err := a.ws.Send(data); err != nil {
		return 0, err
	}

	dbID := a.saveLocal(toPubKeyB64, store.Sent, a.myName, plaintext, time.Now(), 0)
	return dbID, nil
}

// SendGroupMessage encrypts and sends a message to a group. Returns the local DB message ID.
func (a *AppCore) SendGroupMessage(groupID, plaintext string) (int64, error) {
	members, ok := a.groupMembers[groupID]
	if !ok {
		return 0, fmt.Errorf("unknown group: %s", groupID)
	}

	var recipients []protocol.GroupChatRecipient
	for _, memberKey := range members {
		if memberKey == a.myPubB64 {
			continue
		}
		peerPub, err := crypto.PubKeyFromBase64(memberKey)
		if err != nil {
			continue
		}
		nonce, ciphertext, err := crypto.SealMessage([]byte(plaintext), peerPub, a.myPriv)
		if err != nil {
			continue
		}
		recipients = append(recipients, protocol.GroupChatRecipient{
			To:         memberKey,
			Nonce:      base64.StdEncoding.EncodeToString(nonce[:]),
			Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		})
	}

	msg := protocol.GroupChatMsg{
		Type:       protocol.TypeGroupMessage,
		GroupID:    groupID,
		Recipients: recipients,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("marshal: %w", err)
	}
	if err := a.ws.Send(data); err != nil {
		return 0, err
	}

	dbID := a.saveLocal(groupID, store.Sent, a.myName, plaintext, time.Now(), 0)
	return dbID, nil
}

// CreateGroup sends a group creation request to the server.
func (a *AppCore) CreateGroup(name string, memberKeys []string) error {
	msg := protocol.GroupCreateMsg{
		Type:    protocol.TypeGroupCreate,
		Name:    name,
		Members: memberKeys,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return a.ws.Send(data)
}

// SendReadReceipt acknowledges a message was read.
func (a *AppCore) SendReadReceipt(toPubKeyB64 string, messageTS int64) {
	a.sendJSON(protocol.ReadReceiptMsg{
		Type: protocol.TypeReadReceipt, To: toPubKeyB64, MessageTS: messageTS,
	})
}

// SendReaction sends an emoji reaction to a message.
func (a *AppCore) SendReaction(toPubKeyB64 string, messageTS int64, emoji string) {
	a.sendJSON(protocol.ReactionMsg{
		Type: protocol.TypeReaction, To: toPubKeyB64, MessageTS: messageTS, Emoji: emoji,
	})
}

// SendTyping sends a typing indicator.
func (a *AppCore) SendTyping(toPubKeyB64 string) {
	a.sendJSON(protocol.TypingMsg{Type: protocol.TypeTyping, To: toPubKeyB64})
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
	case protocol.TypeGroupMessage:
		return a.processGroupChat(data)
	case protocol.TypeGroupCreated:
		return a.processGroupCreated(data)
	case protocol.TypePresence:
		return a.processPresence(data)
	case protocol.TypeTyping:
		return a.processTyping(data)
	case protocol.TypeReadReceipt:
		return a.processReadReceipt(data)
	case protocol.TypeReaction:
		return a.processReaction(data)
	case protocol.TypeFileMeta:
		return a.processFileMeta(data)
	case protocol.TypeFileChunk:
		return a.processFileChunk(data)
	case protocol.TypeEphemeral:
		return a.processEphemeral(data)
	case protocol.TypeVibeStart:
		return a.processVibeStart(data)
	case protocol.TypeVibePrompt:
		return a.processVibePrompt(data)
	case protocol.TypeVibeOutput:
		return a.processVibeOutput(data)
	case protocol.TypeVibeEnd:
		return a.processVibeEnd(data)
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
		if has, _ := a.msgStore.HasServerMessage(msg.ID); has {
			a.logger.Debug("skipping duplicate message", "server_id", msg.ID)
			return nil
		}
	}

	plaintext, err := a.decryptFrom(msg.From, msg.Nonce, msg.Ciphertext)
	if err != nil {
		a.logger.Warn("decryption failed", "from", truncate(msg.From, 12), "error", err)
		return nil
	}

	fromName := a.getDisplayName(msg.From)
	ts := time.Now()
	if msg.Timestamp > 0 {
		ts = time.Unix(msg.Timestamp, 0)
	}

	dbID := a.saveLocal(msg.From, store.Received, fromName, string(plaintext), ts, msg.ID)

	return &IncomingChatEvent{
		DBMessageID: dbID,
		From:        msg.From,
		FromName:    fromName,
		Plaintext:   string(plaintext),
		Timestamp:   ts,
	}
}

func (a *AppCore) processGroupChat(data []byte) interface{} {
	var msg protocol.GroupChatMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}

	// Find our recipient entry
	var myRecipient *protocol.GroupChatRecipient
	for i := range msg.Recipients {
		if msg.Recipients[i].To == a.myPubB64 {
			myRecipient = &msg.Recipients[i]
			break
		}
	}
	if myRecipient == nil {
		return nil
	}

	plaintext, err := a.decryptFrom(msg.From, myRecipient.Nonce, myRecipient.Ciphertext)
	if err != nil {
		a.logger.Warn("group decryption failed", "from", truncate(msg.From, 12), "error", err)
		return nil
	}

	fromName := a.getDisplayName(msg.From)
	ts := time.Now()
	if msg.Timestamp > 0 {
		ts = time.Unix(msg.Timestamp, 0)
	}

	dbID := a.saveLocal(msg.GroupID, store.Received, fromName, string(plaintext), ts, msg.ID)

	return &GroupMessageEvent{
		DBMessageID: dbID,
		GroupID:     msg.GroupID,
		From:        msg.From,
		FromName:    fromName,
		Plaintext:   string(plaintext),
		Timestamp:   ts,
	}
}

func (a *AppCore) processGroupCreated(data []byte) interface{} {
	var msg protocol.GroupCreatedMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}

	a.groupMembers[msg.GroupID] = msg.Members
	a.ensureGroupConversation(msg.GroupID, msg.Name)

	return &GroupCreatedEvent{
		GroupID: msg.GroupID,
		Name:    msg.Name,
		Members: msg.Members,
		Creator: msg.Creator,
	}
}

func (a *AppCore) processReadReceipt(data []byte) interface{} {
	var msg protocol.ReadReceiptMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	return &ReadReceiptEvent{
		From:      msg.From,
		MessageTS: msg.MessageTS,
	}
}

func (a *AppCore) processReaction(data []byte) interface{} {
	var msg protocol.ReactionMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	fromName := a.getDisplayName(msg.From)
	return &ReactionEvent{
		From:      msg.From,
		FromName:  fromName,
		MessageTS: msg.MessageTS,
		Emoji:     msg.Emoji,
	}
}

func (a *AppCore) processFileMeta(data []byte) interface{} {
	var msg protocol.FileMetaMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	fromName := a.getDisplayName(msg.From)

	// Register pending file
	a.pendingFiles[msg.FileID] = &pendingFile{
		From:        msg.From,
		FromName:    fromName,
		FileName:    msg.FileName,
		FileSize:    msg.FileSize,
		TotalChunks: msg.TotalChunks,
		Chunks:      make(map[int][]byte),
	}

	return &FileMetaEvent{
		From:        msg.From,
		FromName:    fromName,
		FileID:      msg.FileID,
		FileName:    msg.FileName,
		FileSize:    msg.FileSize,
		TotalChunks: msg.TotalChunks,
	}
}

func (a *AppCore) processFileChunk(data []byte) interface{} {
	var msg protocol.FileChunkMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}

	plaintext, err := a.decryptFrom(msg.From, msg.Nonce, msg.Ciphertext)
	if err != nil {
		a.logger.Debug("chunk decryption failed", "file", msg.FileID, "chunk", msg.ChunkIndex, "error", err)
		return nil
	}

	// Add chunk to pending file (mutex-protected for concurrent access)
	a.fileMu.Lock()
	pf, ok := a.pendingFiles[msg.FileID]
	if !ok {
		a.fileMu.Unlock()
		return nil
	}
	pf.Chunks[msg.ChunkIndex] = plaintext

	if len(pf.Chunks) < pf.TotalChunks {
		a.fileMu.Unlock()
		return nil // still waiting for more chunks
	}

	// All chunks received — reassemble
	var fullData []byte
	for i := 0; i < pf.TotalChunks; i++ {
		chunk, exists := pf.Chunks[i]
		if !exists {
			a.fileMu.Unlock()
			a.logger.Warn("missing file chunk", "file", pf.FileName, "chunk", i)
			return nil
		}
		fullData = append(fullData, chunk...)
	}
	delete(a.pendingFiles, msg.FileID)
	a.fileMu.Unlock()

	// Sanitize filename — prevent path traversal
	safeFileName := filepath.Base(pf.FileName)
	if safeFileName == "" || safeFileName == "." || safeFileName == ".." {
		safeFileName = "received_file"
	}

	// Save to disk
	filesDir := filepath.Join(config.DataDir(), "files")
	os.MkdirAll(filesDir, 0700)
	savePath := filepath.Join(filesDir, safeFileName)

	// Avoid overwriting — add timestamp suffix if exists
	if _, err := os.Stat(savePath); err == nil {
		ext := filepath.Ext(safeFileName)
		base := safeFileName[:len(safeFileName)-len(ext)]
		savePath = filepath.Join(filesDir, fmt.Sprintf("%s_%d%s", base, time.Now().Unix(), ext))
	}

	if err := os.WriteFile(savePath, fullData, 0600); err != nil {
		a.logger.Warn("saving file", "error", err)
	}

	return &FileCompleteEvent{
		From:     pf.From,
		FromName: pf.FromName,
		FileID:   msg.FileID,
		FileName: pf.FileName,
		Data:     fullData,
		SavedTo:  savePath,
	}
}

func (a *AppCore) processEphemeral(data []byte) interface{} {
	var msg protocol.EphemeralMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	fromName := a.getDisplayName(msg.From)
	return &EphemeralEvent{
		From:     msg.From,
		FromName: fromName,
		Duration: msg.Duration,
	}
}

func (a *AppCore) processVibeStart(data []byte) interface{} {
	var msg protocol.VibeStartMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	fromName := a.getDisplayName(msg.From)
	// For DMs, the conversation key on the participant side is the host's pubkey
	// For groups, the conversation key is the group ID
	convoKey := msg.From
	if _, isGroup := a.groupMembers[msg.To]; isGroup {
		convoKey = msg.To
	}

	return &VibeStartEvent{From: msg.From, FromName: fromName, To: convoKey, RepoName: msg.RepoName}
}

func (a *AppCore) processVibePrompt(data []byte) interface{} {
	var msg protocol.VibePromptMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	fromName := a.getDisplayName(msg.From)
	return &VibePromptEvent{From: msg.From, FromName: fromName, Prompt: msg.Prompt}
}

func (a *AppCore) processVibeOutput(data []byte) interface{} {
	var msg protocol.VibeOutputMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	return &VibeOutputEvent{From: msg.From, Text: msg.Text, IsDone: msg.IsDone}
}

func (a *AppCore) processVibeEnd(data []byte) interface{} {
	var msg protocol.VibeEndMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	fromName := a.getDisplayName(msg.From)
	return &VibeEndEvent{From: msg.From, FromName: fromName}
}

func (a *AppCore) SendVibeStart(to, repoName string) {
	a.sendJSON(protocol.VibeStartMsg{Type: protocol.TypeVibeStart, To: to, RepoName: repoName})
}

func (a *AppCore) SendVibePrompt(to, prompt string) {
	a.sendJSON(protocol.VibePromptMsg{Type: protocol.TypeVibePrompt, To: to, Prompt: prompt})
}

func (a *AppCore) SendVibeOutput(to, text string, isDone bool) {
	a.sendJSON(protocol.VibeOutputMsg{Type: protocol.TypeVibeOutput, To: to, Text: text, IsDone: isDone})
}

func (a *AppCore) SendVibeEnd(to string) {
	a.sendJSON(protocol.VibeEndMsg{Type: protocol.TypeVibeEnd, To: to})
}

func (a *AppCore) SendEphemeralNotice(to, duration string) {
	a.sendJSON(protocol.EphemeralMsg{Type: protocol.TypeEphemeral, To: to, Duration: duration})
}

// GetGroupMembers returns the member keys for a group.
func (a *AppCore) GetGroupMembers(groupID string) []string {
	return a.groupMembers[groupID]
}

// IsGroup returns true if the given ID is a group conversation.
func (a *AppCore) IsGroup(id string) bool {
	_, ok := a.groupMembers[id]
	return ok
}

func (a *AppCore) saveLocal(peerKey string, dir store.Direction, senderName, plaintext string, ts time.Time, serverID int64) int64 {
	if a.msgStore == nil {
		return 0
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
		return 0
	}
	msg, err := a.msgStore.SaveMessage(convoID, dir, senderName, plaintext, ts, serverID)
	if err != nil {
		a.logger.Warn("saving message", "error", err)
		return 0
	}
	return msg.ID
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

	// Collect message IDs for reaction lookup
	msgIDs := make([]int64, len(msgs))
	for i, m := range msgs {
		msgIDs[i] = m.ID
	}
	reactions, _ := a.msgStore.GetReactions(msgIDs)

	result := make([]HistoryMessage, len(msgs))
	for i, m := range msgs {
		fromName := m.SenderName
		if fromName == "" {
			if m.Direction == store.Sent {
				fromName = a.myName
			} else {
				fromName = a.contactNames[peerKey]
				if fromName == "" {
					fromName = peerKey[:12] + "..."
				}
			}
		}
		result[i] = HistoryMessage{
			DBMessageID: m.ID,
			Direction:   m.Direction,
			Plaintext:   m.Plaintext,
			Timestamp:   m.Timestamp,
			FromName:    fromName,
			Reactions:   reactions[m.ID],
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
	fromName := a.getDisplayName(msg.From)
	return &TypingEvent{From: msg.From, FromName: fromName}
}

// SetMessageExpiry marks a message to expire at the given time.
func (a *AppCore) SetMessageExpiry(messageID int64, expiresAt time.Time) {
	if a.msgStore == nil || messageID == 0 {
		return
	}
	a.msgStore.SetMessageExpiry(messageID, expiresAt)
}

// DeleteExpiredMessages purges expired messages from the store. Returns count deleted.
func (a *AppCore) DeleteExpiredMessages() int {
	if a.msgStore == nil {
		return 0
	}
	count, _ := a.msgStore.DeleteExpiredMessages()
	return count
}

// SaveReaction persists a reaction to the local store.
func (a *AppCore) SaveReaction(messageID int64, fromName, emoji string) {
	if a.msgStore == nil || messageID == 0 {
		return
	}
	a.msgStore.SaveReaction(messageID, fromName, emoji)
}

// SendRaw sends raw JSON data over the WebSocket.
func (a *AppCore) SendRaw(data []byte) error {
	return a.ws.Send(data)
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

// --- Helpers ---

// getDisplayName resolves a public key to a display name, with a safe truncated fallback.
func (a *AppCore) getDisplayName(pubKeyB64 string) string {
	if name := a.contactNames[pubKeyB64]; name != "" {
		return name
	}
	if len(pubKeyB64) > 12 {
		return pubKeyB64[:12] + "..."
	}
	return pubKeyB64
}

// sendJSON marshals and sends a protocol message, logging errors instead of silently discarding.
func (a *AppCore) sendJSON(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		a.logger.Warn("marshal failed", "error", err)
		return
	}
	if err := a.ws.Send(data); err != nil {
		a.logger.Debug("send failed", "error", err)
	}
}

// decodeNonce decodes a base64 nonce string and validates its length.
func decodeNonce(nonceB64 string) (*[24]byte, error) {
	nonceBytes, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, fmt.Errorf("invalid nonce encoding: %w", err)
	}
	if len(nonceBytes) != 24 {
		return nil, fmt.Errorf("invalid nonce size: got %d, want 24", len(nonceBytes))
	}
	var nonce [24]byte
	copy(nonce[:], nonceBytes)
	return &nonce, nil
}

// decryptFrom decrypts a message from a sender identified by their base64 public key.
func (a *AppCore) decryptFrom(fromB64, nonceB64, ciphertextB64 string) ([]byte, error) {
	peerPub, err := crypto.PubKeyFromBase64(fromB64)
	if err != nil {
		return nil, fmt.Errorf("invalid sender key: %w", err)
	}
	nonce, err := decodeNonce(nonceB64)
	if err != nil {
		return nil, err
	}
	ctBytes, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("invalid ciphertext: %w", err)
	}
	return crypto.OpenMessage(ctBytes, nonce, peerPub, a.myPriv)
}

// truncate safely truncates a string to maxLen with ellipsis.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// validateDisplayName checks if a display name is safe (no control chars, reasonable length).
func validateDisplayName(name string) bool {
	if len(name) == 0 || len(name) > 32 {
		return false
	}
	for _, r := range name {
		if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
			return false
		}
	}
	return true
}
