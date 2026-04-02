package cli

import (
	"context"
	cryptotls "crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"nhooyr.io/websocket"

	"enclave/internal/config"
	"enclave/internal/crypto"
	"enclave/internal/protocol"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Enclave: generate keypair, register with server",
	RunE:  runInit,
}

var (
	initDisplayName string
	initServer      string
	initToken       string
	initTLS         bool
	initForce       bool
)

func init() {
	initCmd.Flags().StringVar(&initDisplayName, "display-name", "", "Your display name")
	initCmd.Flags().StringVar(&initServer, "server", "localhost:9300", "Server address to connect to")
	initCmd.Flags().StringVar(&initToken, "token", "", "Invite token from the server admin")
	initCmd.Flags().BoolVar(&initTLS, "tls", false, "Connect to server using TLS")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing config and keys")
}

func runInit(cmd *cobra.Command, args []string) error {
	if crypto.KeysExist() && !initForce {
		return fmt.Errorf("enclave is already initialized at %s (use --force to overwrite)", config.DataDir())
	}

	if initDisplayName == "" {
		return fmt.Errorf("--display-name is required")
	}
	if initToken == "" {
		return fmt.Errorf("--token is required (get an invite token from the server admin)")
	}

	fmt.Println("Initializing Enclave...")

	// Ensure data directory exists
	if err := config.EnsureDataDir(); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	// Generate keypair
	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		return fmt.Errorf("generating keypair: %w", err)
	}

	if err := crypto.SaveKeys(pub, priv); err != nil {
		return fmt.Errorf("saving keys: %w", err)
	}

	fmt.Println("  Keypair generated.")

	// Register with the server
	fmt.Printf("  Registering with %s...\n", initServer)
	if err := registerWithServer(initServer, initTLS, pub, initDisplayName, initToken); err != nil {
		// Save config without registered=true so they can retry
		cfg := config.Config{
			DisplayName: initDisplayName,
			Server:      config.ServerConfig{Address: initServer, TLS: initTLS},
			Registered:  false,
		}
		config.Save(cfg)
		return fmt.Errorf("registration failed: %w\n\n  Your keys were saved. Fix the issue and re-run with --force to try again.", err)
	}

	// Save config with registered=true
	cfg := config.Config{
		DisplayName: initDisplayName,
		Server:      config.ServerConfig{Address: initServer, TLS: initTLS},
		Registered:  true,
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("  Registered successfully!")
	fmt.Println()
	fmt.Printf("  Data directory:  %s\n", config.DataDir())
	fmt.Printf("  Display name:    %s\n", initDisplayName)
	fmt.Printf("  Server:          %s\n", initServer)
	fmt.Printf("  Public key:      %s\n", crypto.PubKeyToBase64(pub))
	fmt.Printf("  Fingerprint:     %s\n", crypto.Fingerprint(pub))
	fmt.Println()
	fmt.Println("  You're all set! Run 'enclave chat' to start chatting.")

	return nil
}

// registerWithServer connects via WebSocket and sends a register message.
func registerWithServer(serverAddr string, useTLS bool, pub *[32]byte, displayName, token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scheme := "ws"
	opts := &websocket.DialOptions{}
	if useTLS {
		scheme = "wss"
		opts.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &cryptotls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}
	url := fmt.Sprintf("%s://%s/ws", scheme, serverAddr)
	conn, _, err := websocket.Dial(ctx, url, opts)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", serverAddr, err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Send register message
	reg := protocol.RegisterMsg{
		Type:        protocol.TypeRegister,
		Token:       token,
		PublicKey:   base64.StdEncoding.EncodeToString(pub[:]),
		DisplayName: displayName,
	}
	data, _ := json.Marshal(reg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("sending registration: %w", err)
	}

	// Read response
	_, respData, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading server response: %w", err)
	}

	msgType, _ := protocol.ParseType(respData)
	switch msgType {
	case protocol.TypeAuthOK:
		return nil // success
	case protocol.TypeError:
		var errMsg protocol.ErrorMsg
		json.Unmarshal(respData, &errMsg)
		return fmt.Errorf("%s: %s", errMsg.Code, errMsg.Message)
	case protocol.TypeAuthFail:
		var fail protocol.AuthFailMsg
		json.Unmarshal(respData, &fail)
		return fmt.Errorf("rejected: %s", fail.Reason)
	default:
		return fmt.Errorf("unexpected response type: %s", msgType)
	}
}
