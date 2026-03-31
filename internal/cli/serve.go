package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"enclave/internal/server"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Enclave relay server",
	RunE:  runServe,
}

var (
	serveBind    string
	serveDB      string
	serveDataDir string
	serveLogLevel string
)

func init() {
	serveCmd.Flags().StringVar(&serveBind, "bind", "0.0.0.0:9300", "Address to bind to")
	serveCmd.Flags().StringVar(&serveDB, "db", "enclave-server.db", "Path to server database")
	serveCmd.Flags().StringVar(&serveDataDir, "data-dir", ".", "Directory for server keys and data")
	serveCmd.Flags().StringVar(&serveLogLevel, "log-level", "info", "Log level: debug, info, warn, error")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Set up logging
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

	// Open store
	store, err := server.NewSQLiteStore(serveDB)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	// Create and start server
	srv, err := server.NewServer(store, logger, serveDataDir)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Admin key: %s\n", srv.AdminKey())
	fmt.Println("  Use this key to generate invite tokens:")
	fmt.Printf("  enclave invite --server %s --admin-key <key>\n", serveBind)
	fmt.Println()

	return srv.Start(serveBind)
}
