package tui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"enclave/internal/client"
	"enclave/internal/config"
	"enclave/internal/crypto"
	"enclave/internal/protocol"
	"enclave/internal/store"
	"enclave/internal/vibe"
)

// Screen identifies which screen is active.
type Screen int

const (
	ScreenConnect Screen = iota
	ScreenMain
)

// App is the root bubbletea model.
type App struct {
	screen      Screen
	connectView ConnectModel
	mainView    MainModel
	appCore     *client.AppCore
	width       int
	height      int

	myPubB64       string
	lastTypingSent time.Time

	// Ephemeral: conversation key -> duration. Zero means off.
	ephemeralDurations map[string]time.Duration

	// Vibe session state
	vibeSession       *vibe.Session // non-nil if we're hosting
	vibeHostKey       string        // pub key of the host (set on both host and participant)
	vibeConvo         string        // conversation key where the vibe is active
	vibePendingPrompt *vibePending  // prompt awaiting host approval
}

type vibePending struct {
	FromName string
	Prompt   string
}

// NewApp creates the root TUI application.
func NewApp(appCore *client.AppCore, identity string, myPubB64 string) App {
	return App{
		screen:             ScreenConnect,
		connectView:        NewConnectModel(),
		mainView:           NewMainModel(identity),
		appCore:            appCore,
		myPubB64:           myPubB64,
		ephemeralDurations: make(map[string]time.Duration),
	}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.connectView.Init(),
	)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.connectView.width = msg.Width
		a.connectView.height = msg.Height
		a.mainView.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		// Handle vibe prompt approval/rejection
		if a.vibePendingPrompt != nil {
			switch msg.String() {
			case "y", "Y":
				pending := a.vibePendingPrompt
				a.vibePendingPrompt = nil
				a.mainView.chatView.AddSystemMessage(fmt.Sprintf("✅ Approved. Running prompt from %s...", pending.FromName))
				if err := a.vibeSession.SendPrompt(pending.Prompt, pending.FromName); err != nil {
					a.mainView.ShowError("Claude Code error: " + err.Error())
				} else {
					return a, a.pollVibeOutput()
				}
				return a, nil
			case "n", "N":
				pending := a.vibePendingPrompt
				a.vibePendingPrompt = nil
				a.mainView.chatView.AddSystemMessage(fmt.Sprintf("❌ Rejected prompt from %s.", pending.FromName))
				// Notify the participant
				if a.vibeConvo != "" {
					a.appCore.SendVibeOutput(a.vibeConvo, fmt.Sprintf("❌ Host rejected the prompt: %s", pending.Prompt), true)
				}
				return a, nil
			}
			// Any other key is ignored while approval is pending
			return a, nil
		}

		switch msg.String() {
		case "ctrl+c":
			if a.appCore != nil {
				a.appCore.Close()
			}
			return a, tea.Quit
		}

	// Connection events (sent via p.Send from the background goroutine)
	case ConnectedMsg:
		a.screen = ScreenMain
		a.mainView.SetConnected(true)
		a.mainView.SetContacts(msg.Users)
		// Load history for the auto-selected contact
		if active := a.mainView.ActiveContact(); active != "" {
			a.loadHistoryIntoView(active)
		}
		return a, tea.Batch(a.waitForMessage(), a.scheduleEphemeralTick())

	case DisconnectedMsg:
		a.mainView.SetConnected(false)
		a.mainView.SetDisconnected()
		if a.screen == ScreenConnect {
			a.connectView.err = msg.Err
		}
		// Don't call waitForMessage — channel is closed

	// Client events from the WebSocket receive loop
	case *client.IncomingChatEvent:
		a.mainView.AddIncomingMessageWithID(msg.From, msg.FromName, msg.Plaintext, msg.Timestamp, msg.DBMessageID)
		if dur := a.ephemeralDuration(); dur > 0 {
			expiry := time.Now().Add(dur)
			a.mainView.chatView.SetLastMessageExpiry(expiry)
			a.appCore.SetMessageExpiry(msg.DBMessageID, expiry)
		}
		return a, a.waitForMessage()

	case *client.PresenceEvent:
		a.mainView.UpdatePresence(msg.PublicKey, msg.Online)
		return a, a.waitForMessage()

	case *client.TypingEvent:
		a.mainView.SetTyping(msg.From, msg.FromName)
		// Schedule a clear after 4 seconds
		return a, tea.Batch(a.waitForMessage(), tea.Tick(4*time.Second, func(t time.Time) tea.Msg {
			return TypingClearTickMsg{}
		}))

	case TypingClearTickMsg:
		a.mainView.ClearTypingIfStale()

	case EphemeralTickMsg:
		a.mainView.chatView.PurgeExpired()
		a.appCore.DeleteExpiredMessages()
		return a, a.scheduleEphemeralTick()

	case *client.ErrorEvent:
		// Could display in a toast/notification — for now just continue
		return a, a.waitForMessage()

	case UserTypingMsg:
		if a.appCore != nil && a.mainView.ActiveContact() != "" {
			if time.Since(a.lastTypingSent) > 2*time.Second {
				a.appCore.SendTyping(a.mainView.ActiveContact())
				a.lastTypingSent = time.Now()
			}
		}

	case SlashCommandMsg:
		if msg.Name == "/quit" {
			if a.appCore != nil {
				a.appCore.Close()
			}
			return a, tea.Quit
		}
		a.handleSlashCommand(msg.Name, msg.Args)

	case *client.GroupMessageEvent:
		a.mainView.AddIncomingMessageWithID(msg.GroupID, msg.FromName, msg.Plaintext, msg.Timestamp, msg.DBMessageID)
		if dur := a.ephemeralDuration(); dur > 0 {
			expiry := time.Now().Add(dur)
			a.mainView.chatView.SetLastMessageExpiry(expiry)
			a.appCore.SetMessageExpiry(msg.DBMessageID, expiry)
		}
		return a, a.waitForMessage()

	case *client.GroupCreatedEvent:
		// Add group to sidebar
		a.mainView.sidebar.contacts = append(a.mainView.sidebar.contacts, ContactInfo{
			PublicKey:   msg.GroupID,
			DisplayName: msg.Name,
			IsGroup:     true,
		})
		a.mainView.chatView.AddSystemMessage(fmt.Sprintf("Group #%s created", msg.Name))
		return a, a.waitForMessage()

	case *client.ReadReceiptEvent:
		a.mainView.chatView.MarkRead(msg.MessageTS)
		return a, a.waitForMessage()

	case *client.ReactionEvent:
		dbID := a.mainView.chatView.AddReaction(msg.MessageTS, msg.FromName, msg.Emoji)
		a.appCore.SaveReaction(dbID, msg.FromName, msg.Emoji)
		return a, a.waitForMessage()

	case *client.EphemeralEvent:
		dur, err := time.ParseDuration(msg.Duration)
		if err != nil || msg.Duration == "off" {
			// Ephemeral disabled by the other party
			delete(a.ephemeralDurations, msg.From)
			a.mainView.chatView.AddSystemMessage(fmt.Sprintf("%s disabled ephemeral mode. Messages will persist.", msg.FromName))
		} else {
			a.ephemeralDurations[msg.From] = dur
			a.mainView.chatView.AddSystemMessage(fmt.Sprintf("%s enabled ephemeral mode. Messages will disappear after %s.", msg.FromName, dur))
		}
		return a, a.waitForMessage()

	case *client.VibeStartEvent:
		a.vibeHostKey = msg.From
		a.vibeConvo = msg.To
		a.mainView.chatView.AddSystemMessage(fmt.Sprintf("🎸 %s started a collaborative coding session on: %s\n   Use @claude <prompt> to send prompts.", msg.FromName, msg.RepoName))
		return a, a.waitForMessage()

	case *client.VibePromptEvent:
		// We're the host — someone sent a prompt, sanitize and queue for approval
		if a.vibeSession != nil && a.vibeSession.IsActive() {
			sanitized, removed := vibe.SanitizePrompt(msg.Prompt)
			warning := ""
			if removed > 0 {
				warning = fmt.Sprintf("\n   ⚠️  %d invisible/control characters were stripped from this prompt!", removed)
			}
			a.vibePendingPrompt = &vibePending{
				FromName: msg.FromName,
				Prompt:   sanitized,
			}
			a.mainView.chatView.AddSystemMessage(fmt.Sprintf(
				"🔒 %s wants to run:\n   @claude %s%s\n\n   Press [y] to approve or [n] to reject",
				msg.FromName, sanitized, warning,
			))
		}
		return a, a.waitForMessage()

	case *client.VibeOutputEvent:
		// We're a participant — output from the host's Claude Code
		if msg.Text != "" {
			a.mainView.chatView.AddMessage(ChatMessage{
				FromName:  "claude",
				Text:      msg.Text,
				Timestamp: time.Now(),
			})
		}
		if msg.IsDone {
			a.mainView.chatView.AddSystemMessage("[claude] Done.")
		}
		return a, a.waitForMessage()

	case *client.VibeEndEvent:
		a.vibeHostKey = ""
		a.vibeConvo = ""
		a.mainView.chatView.AddSystemMessage(fmt.Sprintf("🎸 %s ended the collaborative coding session.", msg.FromName))
		return a, a.waitForMessage()

	case VibeOutputLocalMsg:
		// Output from our local Claude Code subprocess
		to := a.vibeConvo
		if msg.Text != "" {
			a.mainView.chatView.AddMessage(ChatMessage{
				FromName:  "claude",
				Text:      msg.Text,
				Timestamp: time.Now(),
			})
			// Forward to participants
			if to != "" {
				a.appCore.SendVibeOutput(to, msg.Text, false)
			}
		}
		if msg.IsDone {
			a.mainView.chatView.AddSystemMessage("[claude] Done.")
			if to != "" {
				a.appCore.SendVibeOutput(to, "", true)
			}
		} else {
			// Keep polling
			return a, a.pollVibeOutput()
		}

	case *client.FileMetaEvent:
		a.mainView.chatView.AddSystemMessage(fmt.Sprintf("%s is sending file: %s (%d bytes, %d chunks)...", msg.FromName, msg.FileName, msg.FileSize, msg.TotalChunks))
		return a, a.waitForMessage()

	case *client.FileChunkEvent:
		// Chunks are being assembled in AppCore, nothing to show yet
		return a, a.waitForMessage()

	case *client.FileCompleteEvent:
		// File fully received and saved — display it
		a.displayReceivedFile(msg)
		return a, a.waitForMessage()

	case SendMessageCmd:
		if a.appCore != nil && a.mainView.ActiveContact() != "" {
			to := a.mainView.ActiveContact()

			// Check for @claude prompt in an active vibe session
			if a.vibeConvo == to && strings.HasPrefix(strings.ToLower(msg.Text), "@claude ") {
				prompt := msg.Text[8:] // strip "@claude "

				// Send as a normal chat message so everyone can see it
				if a.appCore.IsGroup(to) {
					a.appCore.SendGroupMessage(to, msg.Text)
				} else {
					a.appCore.SendMessage(to, msg.Text)
				}
				a.mainView.AddOwnMessage(to, msg.Text, time.Now())

				if a.vibeSession != nil {
					// We're the host — run it locally
					a.mainView.chatView.AddSystemMessage("[claude] Processing...")
					if err := a.vibeSession.SendPrompt(prompt, a.mainView.statusBar.identity); err != nil {
						a.mainView.ShowError("Claude Code error: " + err.Error())
					} else {
						return a, a.pollVibeOutput()
					}
				} else if a.vibeHostKey != "" {
					// We're a participant — send prompt to the host
					a.appCore.SendVibePrompt(a.vibeHostKey, prompt)
				}
				return a, nil
			}

			var dbID int64
			var err error
			if a.appCore.IsGroup(to) {
				dbID, err = a.appCore.SendGroupMessage(to, msg.Text)
			} else {
				dbID, err = a.appCore.SendMessage(to, msg.Text)
			}
			if err != nil {
				a.mainView.ShowError("Send failed: " + err.Error())
			} else {
				a.mainView.AddOwnMessageWithID(to, msg.Text, time.Now(), dbID)
				if dur := a.ephemeralDuration(); dur > 0 {
					expiry := time.Now().Add(dur)
					a.mainView.chatView.SetLastMessageExpiry(expiry)
					a.appCore.SetMessageExpiry(dbID, expiry)
				}
			}
		}

	case SelectContactMsg:
		// Let MainModel handle the switch first (sets peer, clears view),
		// then load history from the store
		a.mainView, _ = a.mainView.Update(msg)
		if msg.PublicKey != "" {
			a.loadHistoryIntoView(msg.PublicKey)
		}
		return a, tea.Batch(cmds...)

	case TUIErrorMsg:
		if a.screen == ScreenConnect {
			a.connectView.err = msg.Err
		}
	}

	// Delegate to active screen
	var cmd tea.Cmd
	switch a.screen {
	case ScreenConnect:
		a.connectView, cmd = a.connectView.Update(msg)
		cmds = append(cmds, cmd)
	case ScreenMain:
		a.mainView, cmd = a.mainView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

func (a App) pollVibeOutput() tea.Cmd {
	if a.vibeSession == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-a.vibeSession.OutputCh
		if !ok {
			return VibeOutputLocalMsg{IsDone: true}
		}
		return VibeOutputLocalMsg{
			Text:   event.Text,
			IsDone: event.IsDone,
		}
	}
}

func (a App) scheduleEphemeralTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return EphemeralTickMsg{}
	})
}

// ephemeralDuration returns the active ephemeral duration for the current conversation, or 0.
func (a *App) ephemeralDuration() time.Duration {
	active := a.mainView.ActiveContact()
	if active == "" {
		return 0
	}
	return a.ephemeralDurations[active]
}

func (a *App) loadHistoryIntoView(peerKey string) {
	if a.appCore == nil {
		return
	}
	// Purge expired messages from the store before loading
	a.appCore.DeleteExpiredMessages()

	history, err := a.appCore.LoadHistory(peerKey, 100)
	if err != nil {
		return
	}
	// Clear existing messages and load from history
	a.mainView.chatView.messages = nil
	for _, h := range history {
		msg := ChatMessage{
			DBMessageID: h.DBMessageID,
			FromName:    h.FromName,
			Text:        h.Plaintext,
			Timestamp:   h.Timestamp,
			IsOwn:       h.Direction == store.Sent,
		}
		for _, r := range h.Reactions {
			msg.Reactions = append(msg.Reactions, MessageReaction{
				FromName: r.FromName,
				Emoji:    r.Emoji,
			})
		}
		a.mainView.chatView.AddMessage(msg)
	}
}

func (a *App) handleSlashCommand(name, args string) {
	switch name {
	case "/help":
		var lines []string
		lines = append(lines, "Available commands:")
		for _, cmd := range Commands {
			suffix := ""
			if cmd.HasArgs {
				suffix = " <arg>"
			}
			lines = append(lines, fmt.Sprintf("  %s%s — %s", cmd.Name, suffix, cmd.Description))
		}
		a.mainView.chatView.AddSystemMessage(strings.Join(lines, "\n"))

	case "/clear":
		a.mainView.chatView.messages = nil
		a.mainView.chatView.refreshContent()

	case "/whoami":
		pub, _, err := crypto.LoadKeys()
		if err != nil {
			a.mainView.ShowError("Could not load keys: " + err.Error())
			return
		}
		info := fmt.Sprintf("Identity:\n  Name:        %s\n  Public key:  %s\n  Fingerprint: %s",
			a.mainView.statusBar.identity,
			crypto.PubKeyToBase64(pub),
			crypto.Fingerprint(pub),
		)
		a.mainView.chatView.AddSystemMessage(info)

	case "/verify":
		// /verify <name> — verify a specific user
		if args != "" {
			for i := range a.mainView.sidebar.contacts {
				c := &a.mainView.sidebar.contacts[i]
				if c.DisplayName == args && !c.IsGroup {
					a.mainView.ShowContactDetail(c)
					return
				}
			}
			a.mainView.ShowError(fmt.Sprintf("Unknown user: %s", args))
			return
		}

		active := a.mainView.ActiveContact()

		// In a group chat — list all members' fingerprints
		if a.appCore != nil && a.appCore.IsGroup(active) {
			members := a.appCore.GetGroupMembers(active)
			var lines []string
			contact := a.mainView.sidebar.SelectedContact()
			groupName := active
			if contact != nil {
				groupName = contact.DisplayName
			}
			lines = append(lines, fmt.Sprintf("Group #%s — member verification:", groupName))
			lines = append(lines, "")
			for _, memberKey := range members {
				name := a.appCore.ContactName(memberKey)
				if name == "" {
					name = memberKey[:12] + "..."
				}
				key, err := crypto.PubKeyFromBase64(memberKey)
				if err != nil {
					continue
				}
				fp := crypto.Fingerprint(key)
				lines = append(lines, fmt.Sprintf("  %s", name))
				lines = append(lines, fmt.Sprintf("    Fingerprint: %s", fp))
				lines = append(lines, "")
			}
			lines = append(lines, "  Compare fingerprints out-of-band to verify identities.")
			lines = append(lines, "  Use /verify <name> to see full details for a specific user.")
			a.mainView.chatView.AddSystemMessage(strings.Join(lines, "\n"))
			return
		}

		// DM — show contact detail overlay
		contact := a.mainView.sidebar.SelectedContact()
		if contact == nil {
			a.mainView.ShowError("No contact selected")
			return
		}
		a.mainView.ShowContactDetail(contact)

	case "/users":
		var lines []string
		lines = append(lines, "Registered users:")
		for _, c := range a.mainView.sidebar.contacts {
			status := "○ offline"
			if c.Online {
				status = "● online"
			}
			lines = append(lines, fmt.Sprintf("  %s  %s", status, c.DisplayName))
		}
		if len(a.mainView.sidebar.contacts) == 0 {
			lines = append(lines, "  No other users registered")
		}
		a.mainView.chatView.AddSystemMessage(strings.Join(lines, "\n"))

	case "/copy":
		chatView := &a.mainView.chatView
		var block *CodeBlock
		if args == "" {
			count := chatView.CodeBlockCount()
			if count > 0 {
				block = chatView.GetCodeBlock(count)
			}
		} else {
			idx, err := strconv.Atoi(strings.TrimSpace(args))
			if err != nil {
				a.mainView.ShowError("Usage: /copy <number>")
				return
			}
			block = chatView.GetCodeBlock(idx)
		}
		if block == nil {
			a.mainView.ShowError("No matching code block found")
		} else if err := copyToClipboard(block.Code); err != nil {
			a.mainView.ShowError("Clipboard error: " + err.Error())
		} else {
			label := fmt.Sprintf("code block #%d", block.Index)
			if block.Lang != "" {
				label = fmt.Sprintf("%s block #%d", block.Lang, block.Index)
			}
			chatView.AddSystemMessage(fmt.Sprintf("Copied %s to clipboard", label))
		}

	case "/search":
		if args == "" {
			a.mainView.ShowError("Usage: /search <query>")
			return
		}
		results, err := a.appCore.SearchMessages(args, 20)
		if err != nil {
			a.mainView.ShowError("Search error: " + err.Error())
			return
		}
		if len(results) == 0 {
			a.mainView.chatView.AddSystemMessage(fmt.Sprintf("No results for \"%s\"", args))
			return
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("Search results for \"%s\" (%d found):", args, len(results)))
		for _, r := range results {
			dir := "→"
			if r.Message.Direction == store.Received {
				dir = "←"
			}
			ts := r.Message.Timestamp.Format("Jan 2 15:04")
			preview := r.Message.Plaintext
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			// Replace newlines with spaces for preview
			preview = strings.ReplaceAll(preview, "\n", " ")
			lines = append(lines, fmt.Sprintf("  %s [%s] %s %s: %s", dir, ts, r.DisplayName, dir, preview))
		}
		a.mainView.chatView.AddSystemMessage(strings.Join(lines, "\n"))

	case "/export":
		contact := a.mainView.sidebar.SelectedContact()
		if contact == nil {
			a.mainView.ShowError("No contact selected")
			return
		}
		exportDir := filepath.Join(config.DataDir(), "exports")
		os.MkdirAll(exportDir, 0700)
		filename := fmt.Sprintf("%s_%s.txt", contact.DisplayName, time.Now().Format("2006-01-02_150405"))
		path := filepath.Join(exportDir, filename)

		var lines []string
		lines = append(lines, fmt.Sprintf("Enclave conversation with %s", contact.DisplayName))
		lines = append(lines, fmt.Sprintf("Exported: %s", time.Now().Format(time.RFC3339)))
		lines = append(lines, strings.Repeat("─", 60))
		lines = append(lines, "")
		for _, msg := range a.mainView.chatView.messages {
			if msg.IsSystem {
				continue
			}
			name := msg.FromName
			if msg.IsOwn {
				name = a.mainView.statusBar.identity
			}
			ts := msg.Timestamp.Format("2006-01-02 15:04:05")
			lines = append(lines, fmt.Sprintf("[%s] %s:", ts, name))
			lines = append(lines, msg.Text)
			lines = append(lines, "")
		}

		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
			a.mainView.ShowError("Export failed: " + err.Error())
		} else {
			a.mainView.chatView.AddSystemMessage(fmt.Sprintf("Conversation exported to %s", path))
		}

	case "/group":
		if args == "" {
			a.mainView.ShowError("Usage: /group <name> <user1> [user2] ...")
			return
		}
		parts := strings.Fields(args)
		if len(parts) < 2 {
			a.mainView.ShowError("Usage: /group <name> <user1> [user2] ...")
			return
		}
		groupName := parts[0]
		// Resolve member names to public keys
		var memberKeys []string
		for _, name := range parts[1:] {
			found := false
			for _, c := range a.mainView.sidebar.contacts {
				if c.DisplayName == name && !c.IsGroup {
					memberKeys = append(memberKeys, c.PublicKey)
					found = true
					break
				}
			}
			if !found {
				a.mainView.ShowError(fmt.Sprintf("Unknown user: %s", name))
				return
			}
		}
		if err := a.appCore.CreateGroup(groupName, memberKeys); err != nil {
			a.mainView.ShowError("Failed to create group: " + err.Error())
		} else {
			a.mainView.chatView.AddSystemMessage(fmt.Sprintf("Creating group #%s...", groupName))
		}

	case "/invite":
		active := a.mainView.ActiveContact()
		if !a.appCore.IsGroup(active) {
			a.mainView.ShowError("/invite only works in group conversations")
			return
		}
		if args == "" {
			a.mainView.ShowError("Usage: /invite <username>")
			return
		}
		// Resolve name to key
		for _, c := range a.mainView.sidebar.contacts {
			if c.DisplayName == args && !c.IsGroup {
				msg := protocol.GroupInviteMsg{
					Type:    protocol.TypeGroupInvite,
					GroupID: active,
					Member:  c.PublicKey,
				}
				data, _ := json.Marshal(msg)
				a.appCore.SendRaw(data)
				a.mainView.chatView.AddSystemMessage(fmt.Sprintf("Invited %s to the group", args))
				return
			}
		}
		a.mainView.ShowError(fmt.Sprintf("Unknown user: %s", args))

	case "/react":
		emoji := args
		if emoji == "" {
			emoji = "👍"
		}
		lastMsg := a.mainView.chatView.GetLastMessage()
		if lastMsg == nil {
			a.mainView.ShowError("No messages to react to")
			return
		}
		to := a.mainView.ActiveContact()
		a.appCore.SendReaction(to, lastMsg.Timestamp.Unix(), emoji)
		a.mainView.chatView.AddReaction(lastMsg.Timestamp.Unix(), a.mainView.statusBar.identity, emoji)
		a.appCore.SaveReaction(lastMsg.DBMessageID, a.mainView.statusBar.identity, emoji)

	case "/pin":
		lastMsg := a.mainView.chatView.GetLastMessage()
		if lastMsg == nil {
			a.mainView.ShowError("No messages to pin")
			return
		}
		lastMsg.Pinned = true
		a.mainView.chatView.refreshContent()
		a.mainView.chatView.AddSystemMessage("Message pinned")

	case "/pins":
		var lines []string
		lines = append(lines, "Pinned messages:")
		count := 0
		for _, msg := range a.mainView.chatView.messages {
			if msg.Pinned {
				count++
				name := msg.FromName
				if msg.IsOwn {
					name = "you"
				}
				ts := msg.Timestamp.Format("Jan 2 15:04")
				preview := msg.Text
				if len(preview) > 60 {
					preview = preview[:60] + "..."
				}
				preview = strings.ReplaceAll(preview, "\n", " ")
				lines = append(lines, fmt.Sprintf("  [%s] %s: %s", ts, name, preview))
			}
		}
		if count == 0 {
			lines = append(lines, "  No pinned messages")
		}
		a.mainView.chatView.AddSystemMessage(strings.Join(lines, "\n"))

	case "/ephemeral":
		active := a.mainView.ActiveContact()
		if active == "" {
			a.mainView.ShowError("No active conversation")
			return
		}
		if args == "" || args == "off" {
			delete(a.ephemeralDurations, active)
			a.appCore.SendEphemeralNotice(active, "off")
			a.mainView.chatView.AddSystemMessage("Ephemeral mode disabled. New messages will persist.")
		} else {
			dur, err := time.ParseDuration(args)
			if err != nil {
				a.mainView.ShowError(fmt.Sprintf("Invalid duration: %s (use e.g. 30s, 5m, 1h)", args))
				return
			}
			a.ephemeralDurations[active] = dur
			a.appCore.SendEphemeralNotice(active, args)
			a.mainView.chatView.AddSystemMessage(fmt.Sprintf("Ephemeral mode enabled. Messages will disappear after %s.", dur))
		}

	case "/vibe2gether":
		if args == "" {
			a.mainView.ShowError("Usage: /vibe2gether <repo-path>")
			return
		}
		active := a.mainView.ActiveContact()
		if active == "" {
			a.mainView.ShowError("No active conversation")
			return
		}
		if a.vibeSession != nil {
			a.mainView.ShowError("A vibe session is already active. Use /endvibe first.")
			return
		}

		// Expand ~ to home directory
		repoPath := args
		if strings.HasPrefix(repoPath, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				repoPath = filepath.Join(home, repoPath[2:])
			}
		}
		if _, err := os.Stat(repoPath); err != nil {
			a.mainView.ShowError(fmt.Sprintf("Path not found: %s", repoPath))
			return
		}

		// Start the session
		session := vibe.NewSession(repoPath)
		session.Activate()
		a.vibeSession = session
		a.vibeHostKey = a.myPubB64
		a.vibeConvo = active

		repoName := filepath.Base(repoPath)
		a.appCore.SendVibeStart(active, repoName)
		a.mainView.chatView.AddSystemMessage(fmt.Sprintf("🎸 Collaborative coding session started on: %s\n   Others can now use @claude <prompt> to send prompts.\n   Use /endvibe to stop.", repoName))

	case "/endvibe":
		if a.vibeSession == nil {
			a.mainView.ShowError("No active vibe session")
			return
		}
		a.vibeSession.Stop()
		a.vibeSession = nil
		if a.vibeConvo != "" {
			a.appCore.SendVibeEnd(a.vibeConvo)
		}
		a.vibeHostKey = ""
		a.vibeConvo = ""
		a.mainView.chatView.AddSystemMessage("🎸 Collaborative coding session ended.")

	case "/send":
		if args == "" {
			a.mainView.ShowError("Usage: /send <file-path>")
			return
		}
		a.handleFileSend(args)

	case "/quit":
		if a.appCore != nil {
			a.appCore.Close()
		}
		a.mainView.chatView.AddSystemMessage("Goodbye!")

	default:
		a.mainView.ShowError(fmt.Sprintf("Unknown command: %s (type /help for available commands)", name))
	}
}

func (a *App) displayReceivedFile(msg *client.FileCompleteEvent) {
	ext := filepath.Ext(msg.FileName)
	isText := isTextFile(ext)

	if isText && len(msg.Data) <= 50*1024 { // show inline if text and under 50KB
		// Determine language hint from extension
		lang := ""
		if len(ext) > 1 {
			lang = ext[1:] // strip the dot
		}
		// Build a code-fenced message
		content := fmt.Sprintf("📎 **%s** from %s\n```%s\n%s\n```\nSaved to: %s",
			msg.FileName, msg.FromName, lang, string(msg.Data), msg.SavedTo)
		a.mainView.chatView.AddMessage(ChatMessage{
			FromName:  msg.FromName,
			Text:      content,
			Timestamp: time.Now(),
			IsOwn:     false,
		})
	} else {
		// Binary or large file — just show the save path
		size := len(msg.Data)
		a.mainView.chatView.AddSystemMessage(
			fmt.Sprintf("📎 Received file from %s: %s (%d bytes)\n   Saved to: %s",
				msg.FromName, msg.FileName, size, msg.SavedTo))
	}
}

func isTextFile(ext string) bool {
	textExts := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".rs": true, ".c": true, ".h": true, ".cpp": true, ".java": true, ".kt": true,
		".rb": true, ".php": true, ".sh": true, ".bash": true, ".zsh": true, ".fish": true,
		".sql": true, ".html": true, ".css": true, ".scss": true, ".less": true,
		".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
		".md": true, ".txt": true, ".csv": true, ".log": true, ".env": true,
		".dockerfile": true, ".makefile": true, ".gitignore": true,
		".conf": true, ".cfg": true, ".ini": true, ".properties": true,
		".lua": true, ".vim": true, ".el": true, ".ex": true, ".exs": true,
		".hs": true, ".ml": true, ".swift": true, ".dart": true, ".r": true,
		".tf": true, ".hcl": true, ".proto": true, ".graphql": true,
	}
	return textExts[strings.ToLower(ext)]
}

func (a *App) handleFileSend(filePath string) {
	to := a.mainView.ActiveContact()
	if to == "" {
		a.mainView.ShowError("No active conversation")
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		a.mainView.ShowError("Could not read file: " + err.Error())
		return
	}

	if int64(len(data)) > protocol.MaxFileSize {
		a.mainView.ShowError(fmt.Sprintf("File too large (%d bytes, max %d)", len(data), protocol.MaxFileSize))
		return
	}

	fileName := filepath.Base(filePath)
	fileID := uuid.New().String()

	peerPub, err := crypto.PubKeyFromBase64(to)
	if err != nil {
		a.mainView.ShowError("Invalid recipient")
		return
	}

	// Load our private key
	_, priv, err := crypto.LoadKeys()
	if err != nil {
		a.mainView.ShowError("Could not load keys")
		return
	}

	// Send file metadata
	totalChunks := (len(data) + protocol.FileChunkSize - 1) / protocol.FileChunkSize
	meta := protocol.FileMetaMsg{
		Type:        protocol.TypeFileMeta,
		To:          to,
		FileID:      fileID,
		FileName:    fileName,
		FileSize:    int64(len(data)),
		TotalChunks: totalChunks,
	}
	metaJSON, _ := json.Marshal(meta)
	a.appCore.SendRaw(metaJSON)

	// Send chunks
	for i := 0; i < totalChunks; i++ {
		start := i * protocol.FileChunkSize
		end := start + protocol.FileChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[start:end]

		nonce, ciphertext, err := crypto.SealMessage(chunk, peerPub, priv)
		if err != nil {
			a.mainView.ShowError(fmt.Sprintf("Encryption failed on chunk %d: %v", i, err))
			return
		}

		chunkMsg := protocol.FileChunkMsg{
			Type:       protocol.TypeFileChunk,
			To:         to,
			FileID:     fileID,
			ChunkIndex: i,
			Nonce:      base64.StdEncoding.EncodeToString(nonce[:]),
			Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		}
		chunkJSON, _ := json.Marshal(chunkMsg)
		a.appCore.SendRaw(chunkJSON)
	}

	a.mainView.chatView.AddSystemMessage(fmt.Sprintf("Sent file: %s (%d bytes, %d chunks)", fileName, len(data), totalChunks))
}

// waitForMessage returns a tea.Cmd that waits for the next message from the server.
func (a App) waitForMessage() tea.Cmd {
	return func() tea.Msg {
		data, ok := <-a.appCore.RecvChannel()
		if !ok {
			return DisconnectedMsg{Err: nil}
		}
		result := a.appCore.ProcessIncoming(data)
		if result == nil {
			// Unhandled message type — keep waiting
			return a.waitForMessage()()
		}
		return result
	}
}

func (a App) View() string {
	switch a.screen {
	case ScreenConnect:
		return a.connectView.View()
	case ScreenMain:
		return a.mainView.View()
	default:
		return ""
	}
}

// copyToClipboard writes text to the system clipboard.
// Handles WSL (clip.exe), macOS (pbcopy), and Linux (xclip/xsel).
func copyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Check for WSL first
		if isWSL() {
			cmd = exec.Command("clip.exe")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip or xsel)")
		}
	case "windows":
		cmd = exec.Command("clip.exe")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func isWSL() bool {
	data, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}
