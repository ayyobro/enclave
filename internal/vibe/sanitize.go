package vibe

import (
	"strings"
	"unicode"
)

// SanitizePrompt removes invisible characters, control sequences, and other
// potentially dangerous content from a remote user's prompt before it reaches
// Claude Code. This prevents prompt injection via hidden characters.
func SanitizePrompt(input string) (sanitized string, removed int) {
	var b strings.Builder
	b.Grow(len(input))

	for _, r := range input {
		if shouldRemove(r) {
			removed++
			continue
		}
		b.WriteRune(r)
	}

	return b.String(), removed
}

func shouldRemove(r rune) bool {
	// Allow normal printable characters, newlines, and tabs
	if r == '\n' || r == '\r' || r == '\t' {
		return false
	}

	// Remove all control characters (C0 and C1)
	if r < 0x20 {
		return true
	}
	if r >= 0x7F && r <= 0x9F {
		return true
	}

	// Remove zero-width characters
	switch r {
	case '\u200B', // zero-width space
		'\u200C', // zero-width non-joiner
		'\u200D', // zero-width joiner
		'\u200E', // left-to-right mark
		'\u200F', // right-to-left mark
		'\u2060', // word joiner
		'\u2061', // function application
		'\u2062', // invisible times
		'\u2063', // invisible separator
		'\u2064', // invisible plus
		'\uFEFF', // byte order mark / zero-width no-break space
		'\uFFF9', // interlinear annotation anchor
		'\uFFFA', // interlinear annotation separator
		'\uFFFB': // interlinear annotation terminator
		return true
	}

	// Remove Unicode tag characters (U+E0000–U+E007F) — used for invisible text
	if r >= 0xE0000 && r <= 0xE007F {
		return true
	}

	// Remove variation selectors (can alter rendering)
	if r >= 0xFE00 && r <= 0xFE0F {
		return true
	}

	// Remove Unicode format characters
	if unicode.Is(unicode.Cf, r) {
		return true
	}

	return false
}
