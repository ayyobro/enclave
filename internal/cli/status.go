package cli

import (
	cryptotls "crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"enclave/internal/config"
	"enclave/internal/crypto"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check connectivity and identity status",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	fmt.Println()

	// Check identity
	if crypto.KeysExist() {
		pub, _, err := crypto.LoadKeys()
		if err != nil {
			fmt.Printf("  Identity:    error (%v)\n", err)
		} else {
			fmt.Printf("  Identity:    %s\n", crypto.ShortFingerprint(pub))
		}
	} else {
		fmt.Println("  Identity:    not initialized (run 'enclave init')")
		fmt.Println()
		return nil
	}

	// Check config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("  Config:      error (%v)\n", err)
		fmt.Println()
		return nil
	}

	fmt.Printf("  Display name: %s\n", cfg.DisplayName)
	fmt.Printf("  Server:       %s\n", cfg.Server.Address)
	fmt.Printf("  TLS:          %v\n", cfg.Server.TLS)
	fmt.Printf("  Registered:   %v\n", cfg.Registered)

	// Check server connectivity
	scheme := "http"
	if cfg.Server.TLS {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s/health", scheme, cfg.Server.Address)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	if cfg.Server.TLS {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &cryptotls.Config{InsecureSkipVerify: true},
		}
	}

	start := time.Now()
	resp, err := httpClient.Get(url)
	latency := time.Since(start)

	if err != nil {
		fmt.Printf("  Server:       unreachable (%v)\n", err)
		fmt.Println()
		return nil
	}
	defer resp.Body.Close()

	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	json.NewDecoder(resp.Body).Decode(&health)

	fmt.Printf("  Server:       %s (v%s, %dms latency)\n", health.Status, health.Version, latency.Milliseconds())
	fmt.Println()

	return nil
}
