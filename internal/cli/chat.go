package cli

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"enclave/internal/client"
	"enclave/internal/config"
	"enclave/internal/crypto"
	"enclave/internal/store"
	"enclave/internal/theme"
	"enclave/internal/tui"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Launch the interactive TUI chat client",
	RunE:  runChat,
}

var (
	chatServer string
	chatTheme  string
)

func init() {
	chatCmd.Flags().StringVar(&chatServer, "server", "", "Override server address from config")
	chatCmd.Flags().StringVar(&chatTheme, "theme", "dark", "Color theme: dark, light, dracula, nord")
}

func runChat(cmd *cobra.Command, args []string) error {
	// Load identity
	pub, priv, err := crypto.LoadKeys()
	if err != nil {
		return fmt.Errorf("loading keys (have you run 'enclave init'?): %w", err)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if !cfg.Registered {
		return fmt.Errorf("not registered with the server yet — run 'enclave init' with an invite token first")
	}

	serverAddr := cfg.Server.Address
	if chatServer != "" {
		serverAddr = chatServer
	}

	displayName := cfg.DisplayName
	if displayName == "" {
		return fmt.Errorf("no display name set (run 'enclave init' first)")
	}

	pubB64 := base64.StdEncoding.EncodeToString(pub[:])

	// Set up logging to a file so it doesn't interfere with the TUI
	logFile, err := os.OpenFile(config.DataDir()+"/enclave.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		logFile, _ = os.Open(os.DevNull)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Open local message store
	msgStore, err := store.NewSQLiteStore(config.MessagesDBPath())
	if err != nil {
		logger.Warn("could not open message store, history will not persist", "error", err)
	}

	// Set color theme
	theme.Set(chatTheme)

	// Create the app core
	appCore := client.NewAppCore(serverAddr, cfg.Server.TLS, pub, priv, displayName, msgStore, logger)

	// Create the TUI app
	app := tui.NewApp(appCore, displayName, pubB64)
	p := tea.NewProgram(app, tea.WithAltScreen())

	// Connect in a background goroutine so the TUI shows the spinner
	go func() {
		// Always authenticate (registration happened during 'enclave init')
		entries, err := appCore.ConnectAndAuth("")
		if err != nil {
			p.Send(tui.DisconnectedMsg{Err: err})
			p.Send(tui.TUIErrorMsg{Err: fmt.Errorf("connection failed: %w", err)})
			return
		}
		contacts := make([]tui.ContactInfo, len(entries))
		for i, e := range entries {
			contacts[i] = tui.ContactInfo{
				PublicKey:   e.PublicKey,
				DisplayName: e.DisplayName,
				Online:      e.Online,
			}
		}
		p.Send(tui.ConnectedMsg{Users: contacts})
	}()

	_, err = p.Run()
	appCore.Close()
	return err
}
