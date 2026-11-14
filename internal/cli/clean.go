package cli

import (
	"strings"
	"unicode"
)

// clean drops the control characters (and bidirectional overrides) from text
// that came from a server or the environment, so it cannot move the cursor,
// retitle the terminal or reorder what a person reads.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			return -1
		}
		return r
	}, s)
}

// cleanLines is clean for text of several lines: the newlines stay.
func cleanLines(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = clean(lines[i])
	}
	return strings.Join(lines, "\n")
}
