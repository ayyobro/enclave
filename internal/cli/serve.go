package cli

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"enclave/internal/server"
	"enclave/internal/sshgw"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Enclave relay server",
	RunE:  runServe,
}

var (
	serveBind     string
	serveDB       string
	serveDataDir  string
	serveLogLevel string
	serveTLS      bool
	serveTLSCert  string
	serveTLSKey   string
	serveSSH      bool
	serveSSHBind  string
)

func init() {
	serveCmd.Flags().StringVar(&serveBind, "bind", "0.0.0.0:9300", "Address to bind to")
	serveCmd.Flags().StringVar(&serveDB, "db", "enclave-server.db", "Path to server database")
	serveCmd.Flags().StringVar(&serveDataDir, "data-dir", ".", "Directory for server keys and data")
	serveCmd.Flags().StringVar(&serveLogLevel, "log-level", "info", "Log level: debug, info, warn, error")
	serveCmd.Flags().BoolVar(&serveTLS, "tls", false, "Enable TLS (auto-generates self-signed cert if none provided)")
	serveCmd.Flags().StringVar(&serveTLSCert, "tls-cert", "", "Path to TLS certificate file")
	serveCmd.Flags().StringVar(&serveTLSKey, "tls-key", "", "Path to TLS private key file")
	serveCmd.Flags().BoolVar(&serveSSH, "ssh", false, "Enable SSH gateway for TUI access")
	serveCmd.Flags().StringVar(&serveSSHBind, "ssh-bind", "0.0.0.0:2222", "SSH gateway listen address")
}

func runServe(cmd *cobra.Command, args []string) error {
	var level slog.Level
	switch serveLogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	store, err := server.NewSQLiteStore(serveDB)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	srv, err := server.NewServer(store, logger, serveDataDir)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	protocol := "ws"
	if serveTLS || serveTLSCert != "" {
		protocol = "wss"
	}

	fmt.Println()
	fmt.Printf("  Admin key: %s\n", srv.AdminKey())
	fmt.Println("  Use this key to generate invite tokens:")
	fmt.Printf("  enclave invite --server %s --admin-key <key>\n", serveBind)
	if protocol == "wss" {
		fmt.Println("  TLS enabled")
	}
	fmt.Println()

	// Start SSH gateway if enabled
	if serveSSH {
		sshSrv, err := sshgw.NewSSHServer(serveSSHBind, serveBind, serveDataDir, logger)
		if err != nil {
			return fmt.Errorf("creating SSH gateway: %w", err)
		}
		fmt.Printf("  SSH gateway: ssh <user>@<host> -p %s\n", serveSSHBind[strings.LastIndex(serveSSHBind, ":")+1:])
		fmt.Println()
		go func() {
			if err := sshSrv.Start(); err != nil {
				logger.Error("SSH gateway error", "error", err)
			}
		}()
	}

	if protocol == "wss" {
		return srv.StartTLS(serveBind, serveTLSCert, serveTLSKey, serveDataDir)
	}
	return srv.Start(serveBind)
}
