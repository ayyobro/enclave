package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Fingerprint returns a human-readable fingerprint of a public key.
// Format: "a3f8 c2d1 09bb 4e7a f210 d83c 71a5 ee09"
func Fingerprint(pub *[32]byte) string {
	h := sha256.Sum256(pub[:])
	hexStr := hex.EncodeToString(h[:16]) // use first 16 bytes (128 bits)

	var parts []string
	for i := 0; i < len(hexStr); i += 4 {
		end := i + 4
		if end > len(hexStr) {
			end = len(hexStr)
		}
		parts = append(parts, hexStr[i:end])
	}
	return strings.Join(parts, " ")
}

// ShortFingerprint returns a truncated fingerprint for display in tight spaces.
// Format: "a3f8...ee09"
func ShortFingerprint(pub *[32]byte) string {
	fp := Fingerprint(pub)
	parts := strings.Fields(fp)
	if len(parts) < 2 {
		return fp
	}
	return fmt.Sprintf("%s...%s", parts[0], parts[len(parts)-1])
}
