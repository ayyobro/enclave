package cli

import (
	"fmt"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
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
	exportKeyCmd.Flags().StringVar(&exportFormat, "format", "base64", "Output format: base64, hex, qr")
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
	case "qr":
		fmt.Printf("  Public key (base64): %s\n", crypto.PubKeyToBase64(pub))
		fmt.Println()
		qr, err := renderQR(crypto.PubKeyToBase64(pub))
		if err != nil {
			return fmt.Errorf("generating QR code: %w", err)
		}
		fmt.Println(qr)
	default:
		return fmt.Errorf("unknown format: %s (use 'base64', 'hex', or 'qr')", exportFormat)
	}
	fmt.Printf("  Fingerprint:         %s\n", crypto.Fingerprint(pub))
	fmt.Println()

	return nil
}

// renderQR generates a QR code as a string using unicode half-block characters.
// Each pair of vertical pixels maps to one character: upper half, lower half, both, or empty.
func renderQR(data string) (string, error) {
	qr, err := qrcode.New(data, qrcode.Medium)
	if err != nil {
		return "", err
	}
	qr.DisableBorder = false

	bitmap := qr.Bitmap()
	rows := len(bitmap)
	if rows == 0 {
		return "", nil
	}
	cols := len(bitmap[0])

	var sb strings.Builder

	// Process two rows at a time using half-block characters
	for y := 0; y < rows; y += 2 {
		sb.WriteString("  ") // indent
		for x := 0; x < cols; x++ {
			top := bitmap[y][x]
			bottom := false
			if y+1 < rows {
				bottom = bitmap[y+1][x]
			}

			switch {
			case top && bottom:
				sb.WriteString("█") // full block
			case top && !bottom:
				sb.WriteString("▀") // upper half
			case !top && bottom:
				sb.WriteString("▄") // lower half
			default:
				sb.WriteString(" ") // empty
			}
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
