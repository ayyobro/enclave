package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var inviteCmd = &cobra.Command{
	Use:   "invite",
	Short: "Generate an invite token for a new user",
	RunE:  runInvite,
}

var (
	inviteServer   string
	inviteAdminKey string
	inviteExpires  time.Duration
	inviteUses     int
)

func init() {
	inviteCmd.Flags().StringVar(&inviteServer, "server", "localhost:9300", "Address of the running Enclave server")
	inviteCmd.Flags().StringVar(&inviteAdminKey, "admin-key", "", "Admin key (printed when the server starts)")
	inviteCmd.Flags().DurationVar(&inviteExpires, "expires", 72*time.Hour, "Token expiration duration")
	inviteCmd.Flags().IntVar(&inviteUses, "uses", 1, "Maximum uses for this token")
}

func runInvite(cmd *cobra.Command, args []string) error {
	if inviteAdminKey == "" {
		return fmt.Errorf("--admin-key is required (the admin key is printed when the server starts)")
	}

	// Build request
	body, _ := json.Marshal(map[string]interface{}{
		"max_uses":   inviteUses,
		"expires_in": inviteExpires.String(),
	})

	url := fmt.Sprintf("http://%s/api/invite", inviteServer)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+inviteAdminKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to server at %s: %w", inviteServer, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.Unmarshal(respBody, &errResp)
		if errResp.Error != "" {
			return fmt.Errorf("server rejected request: %s", errResp.Error)
		}
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Token     string `json:"token"`
		MaxUses   int    `json:"max_uses"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parsing server response: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Invite token:  %s\n", result.Token)
	fmt.Printf("  Expires:       %s\n", result.ExpiresAt)
	fmt.Printf("  Max uses:      %d\n", result.MaxUses)
	fmt.Println()
	fmt.Println("  Share this token with the person you want to invite.")
	fmt.Println("  They'll use it with: enclave init --token <token>")
	fmt.Println()

	return nil
}
