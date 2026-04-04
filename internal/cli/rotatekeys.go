package cli

import (
	"context"
	"crypto/rand"
	cryptotls "crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/nacl/box"
	"nhooyr.io/websocket"

	"enclave/internal/config"
	"enclave/internal/crypto"
	"enclave/internal/protocol"
)

var rotateKeysCmd = &cobra.Command{
	Use:   "rotate-keys",
	Short: "Generate a new keypair and re-register with the server",
	Long:  "Rotates your identity keypair. The old key is revoked on the server and all contacts are notified of the change.",
	RunE:  runRotateKeys,
}

func init() {
	rootCmd.AddCommand(rotateKeysCmd)
}

func runRotateKeys(cmd *cobra.Command, args []string) error {
	oldPub, _, err := crypto.LoadKeys()
	if err != nil {
		return fmt.Errorf("loading current keys: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if !cfg.Registered {
		return fmt.Errorf("not registered — run 'enclave init' first")
	}

	fmt.Println()
	fmt.Println("  Rotating keys...")
	fmt.Printf("  Old fingerprint: %s\n", crypto.Fingerprint(oldPub))

	// Generate new keypair
	newPub, newPriv, err := crypto.GenerateKeypair()
	if err != nil {
		return fmt.Errorf("generating new keypair: %w", err)
	}

	fmt.Printf("  New fingerprint: %s\n", crypto.Fingerprint(newPub))

	// Connect and rotate
	fmt.Printf("  Notifying server at %s...\n", cfg.Server.Address)
	if err := sendKeyRotation(cfg, newPub); err != nil {
		return fmt.Errorf("key rotation failed: %w", err)
	}

	// Save new keys
	if err := crypto.SaveKeys(newPub, newPriv); err != nil {
		return fmt.Errorf("saving new keys: %w", err)
	}

	fmt.Println("  Keys rotated successfully!")
	fmt.Println()
	fmt.Printf("  New public key:  %s\n", crypto.PubKeyToBase64(newPub))
	fmt.Printf("  New fingerprint: %s\n", crypto.Fingerprint(newPub))
	fmt.Println()
	fmt.Println("  Your contacts will see a key change notification.")
	fmt.Println("  Run 'enclave chat' to reconnect with your new identity.")

	return nil
}

func sendKeyRotation(cfg config.Config, newPub *[32]byte) error {
	// Load current keys for auth
	oldPub, oldPriv, err := crypto.LoadKeys()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scheme := "ws"
	opts := &websocket.DialOptions{}
	if cfg.Server.TLS {
		scheme = "wss"
		opts.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &cryptotls.Config{InsecureSkipVerify: true},
			},
		}
	}

	url := fmt.Sprintf("%s://%s/ws", scheme, cfg.Server.Address)
	conn, _, err := websocket.Dial(ctx, url, opts)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	oldPubB64 := base64.StdEncoding.EncodeToString(oldPub[:])
	newPubB64 := base64.StdEncoding.EncodeToString(newPub[:])

	// Authenticate with old key
	authMsg := protocol.AuthMsg{Type: protocol.TypeAuth, PublicKey: oldPubB64}
	authData, _ := json.Marshal(authMsg)
	conn.Write(ctx, websocket.MessageText, authData)

	// Read challenge
	_, challengeData, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading challenge: %w", err)
	}
	msgType, _ := protocol.ParseType(challengeData)
	if msgType != protocol.TypeChallenge {
		return fmt.Errorf("expected challenge, got %s: %s", msgType, string(challengeData))
	}

	var challenge protocol.ChallengeMsg
	json.Unmarshal(challengeData, &challenge)

	challengeNonce, _ := base64.StdEncoding.DecodeString(challenge.Nonce)
	serverKeyBytes, _ := base64.StdEncoding.DecodeString(challenge.ServerKey)
	var serverPub [32]byte
	copy(serverPub[:], serverKeyBytes)

	// Solve challenge
	var nonce [24]byte
	rand.Read(nonce[:])
	sealed := box.Seal(nonce[:], challengeNonce, &nonce, &serverPub, oldPriv)

	resp := protocol.AuthRespMsg{
		Type:     protocol.TypeAuthResp,
		Response: base64.StdEncoding.EncodeToString(sealed),
	}
	respData, _ := json.Marshal(resp)
	conn.Write(ctx, websocket.MessageText, respData)

	// Read auth_ok
	_, okData, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("auth response: %w", err)
	}
	if okType, _ := protocol.ParseType(okData); okType != protocol.TypeAuthOK {
		return fmt.Errorf("auth failed: %s", string(okData))
	}

	// Send key rotation
	rotateMsg := protocol.KeyRotateMsg{
		Type:   protocol.TypeKeyRotate,
		OldKey: oldPubB64,
		NewKey: newPubB64,
	}
	rotateData, _ := json.Marshal(rotateMsg)
	conn.Write(ctx, websocket.MessageText, rotateData)

	time.Sleep(500 * time.Millisecond)
	return nil
}
