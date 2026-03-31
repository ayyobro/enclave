package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"enclave/internal/crypto"
)

var exportKeyCmd = &cobra.Command{
	Use:   "export-key",
	Short: "Print your public key for sharing with contacts",
	RunE:  runExportKey,
}

var exportFormat string

func init() {
	exportKeyCmd.Flags().StringVar(&exportFormat, "format", "base64", "Output format: base64, hex")
}

func runExportKey(cmd *cobra.Command, args []string) error {
	pub, _, err := crypto.LoadKeys()
	if err != nil {
		return fmt.Errorf("loading keys (have you run 'enclave init'?): %w", err)
	}

	fmt.Println()
	switch exportFormat {
	case "hex":
		fmt.Printf("  Public key (hex):    %s\n", crypto.PubKeyToHex(pub))
	case "base64":
		fmt.Printf("  Public key (base64): %s\n", crypto.PubKeyToBase64(pub))
	default:
		return fmt.Errorf("unknown format: %s (use 'base64' or 'hex')", exportFormat)
	}
	fmt.Printf("  Fingerprint:         %s\n", crypto.Fingerprint(pub))
	fmt.Println()

	return nil
}
