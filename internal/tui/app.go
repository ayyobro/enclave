package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"enclave/internal/client"
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

	myPubB64 string
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
		a.mainView.SetTyping(msg.From)
		return a, a.waitForMessage()

	case *client.ErrorEvent:
		// Could display in a toast/notification — for now just continue
		return a, a.waitForMessage()

	case CopyCodeBlockMsg:
		chatView := &a.mainView.chatView
		var block *CodeBlock
		if msg.Index == 0 {
			// Copy latest
			count := chatView.CodeBlockCount()
			if count > 0 {
				block = chatView.GetCodeBlock(count)
			}
		} else {
			block = chatView.GetCodeBlock(msg.Index)
		}
		if block == nil {
			a.mainView.ShowError(fmt.Sprintf("No code block #%d found", msg.Index))
		} else if err := copyToClipboard(block.Code); err != nil {
			a.mainView.ShowError("Clipboard error: " + err.Error())
		} else {
			label := fmt.Sprintf("code block #%d", block.Index)
			if block.Lang != "" {
				label = fmt.Sprintf("%s block #%d", block.Lang, block.Index)
			}
			a.mainView.chatView.AddSystemMessage(fmt.Sprintf("Copied %s to clipboard", label))
		}

	case SendMessageCmd:
		if a.appCore != nil && a.mainView.ActiveContact() != "" {
			to := a.mainView.ActiveContact()
			if err := a.appCore.SendMessage(to, msg.Text); err != nil {
				a.mainView.ShowError("Send failed: " + err.Error())
			} else {
				a.mainView.AddOwnMessage(to, msg.Text, time.Now())
			}
		}

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
