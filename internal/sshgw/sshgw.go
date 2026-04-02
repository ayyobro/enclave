package sshgw

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"

	tea "github.com/charmbracelet/bubbletea"
)

// SSHServer wraps a Wish SSH server that serves the Enclave TUI to SSH clients.
type SSHServer struct {
	srv    *ssh.Server
	wsAddr string // address of the WebSocket relay server
	logger *slog.Logger
}

// NewSSHServer creates a new SSH-served TUI gateway.
func NewSSHServer(sshAddr, wsAddr, dataDir string, logger *slog.Logger) (*SSHServer, error) {
	hostKeyPath := filepath.Join(dataDir, "ssh_host_key")

	s := &SSHServer{
		wsAddr: wsAddr,
		logger: logger,
	}

	srv, err := wish.NewServer(
		wish.WithAddress(sshAddr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithMiddleware(
			bubbletea.Middleware(s.teaHandler),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating SSH server: %w", err)
	}

	s.srv = srv
	return s, nil
}

// Start begins serving SSH connections.
func (s *SSHServer) Start() error {
	s.logger.Info("SSH gateway started", "address", s.srv.Addr)
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the SSH server.
func (s *SSHServer) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// teaHandler creates a bubbletea program for each SSH session.
func (s *SSHServer) teaHandler(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
	// Get the user's SSH public key fingerprint for display
	userName := sess.User()
	if userName == "" {
		userName = "ssh-user"
	}

	pubKey := sess.PublicKey()
	var pubKeyB64 string
	if pubKey != nil {
		pubKeyB64 = base64.StdEncoding.EncodeToString(pubKey.Marshal())
	}

	s.logger.Info("SSH session started",
		"user", userName,
		"remote", sess.RemoteAddr().String(),
	)

	// Create a lightweight TUI that shows a welcome message
	// Full integration with AppCore would require the user to have registered
	// via `enclave init` first. For now, show a read-only info screen.
	model := NewSSHWelcomeModel(userName, pubKeyB64, s.wsAddr)

	return model, []tea.ProgramOption{tea.WithAltScreen()}
}
