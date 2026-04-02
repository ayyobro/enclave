package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"enclave/internal/client"
	"enclave/internal/config"
	"enclave/internal/crypto"
	"enclave/internal/store"
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

	myPubB64      string
	lastTypingSent time.Time
}

// NewApp creates the root TUI application.
func NewApp(appCore *client.AppCore, identity string, myPubB64 string) App {
	return App{
		screen:      ScreenConnect,
		connectView: NewConnectModel(),
		mainView:    NewMainModel(identity),
		appCore:     appCore,
		myPubB64:    myPubB64,
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
		return a, a.waitForMessage()

	case DisconnectedMsg:
		a.mainView.SetConnected(false)
		a.mainView.SetDisconnected()
		if a.screen == ScreenConnect {
			a.connectView.err = msg.Err
		}
		// Don't call waitForMessage — channel is closed

	// Client events from the WebSocket receive loop
	case *client.IncomingChatEvent:
		a.mainView.AddIncomingMessage(msg.From, msg.FromName, msg.Plaintext, msg.Timestamp)
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

	case SendMessageCmd:
		if a.appCore != nil && a.mainView.ActiveContact() != "" {
			to := a.mainView.ActiveContact()
			if err := a.appCore.SendMessage(to, msg.Text); err != nil {
				a.mainView.ShowError("Send failed: " + err.Error())
			} else {
				a.mainView.AddOwnMessage(to, msg.Text, time.Now())
			}
		}

	case SelectContactMsg:
		// Intercept contact switch to load history before MainModel handles it
		if msg.PublicKey != a.mainView.ActiveContact() {
			a.loadHistoryIntoView(msg.PublicKey)
		}
		// Fall through to let MainModel handle the rest

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

func (a *App) loadHistoryIntoView(peerKey string) {
	if a.appCore == nil {
		return
	}
	history, err := a.appCore.LoadHistory(peerKey, 100)
	if err != nil {
		return
	}
	// Clear existing messages and load from history
	a.mainView.chatView.messages = nil
	for _, h := range history {
		a.mainView.chatView.AddMessage(ChatMessage{
			FromName:  h.FromName,
			Text:      h.Plaintext,
			Timestamp: h.Timestamp,
			IsOwn:     h.Direction == store.Sent,
		})
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

	case "/quit":
		if a.appCore != nil {
			a.appCore.Close()
		}
		// tea.Quit is handled by returning it from Update, not here.
		// We'll set a flag and let Update return tea.Quit.
		a.mainView.chatView.AddSystemMessage("Goodbye!")

	default:
		a.mainView.ShowError(fmt.Sprintf("Unknown command: %s (type /help for available commands)", name))
	}
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
