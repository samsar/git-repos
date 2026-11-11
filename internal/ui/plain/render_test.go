package plain

import (
	"strings"
	"testing"
)

// Commit messages and paths are written by other people; they must not
// reach the terminal as escape sequences.
func TestTruncStripsEscapes(t *testing.T) {
	in := "safe \x1b]8;;https://evil.example\x1b\\click\x1b]8;;\x1b\\ \x1b[2J"
	got := trunc(in, 80)
	if strings.ContainsRune(got, '\x1b') {
		t.Errorf("trunc kept an escape sequence: %q", got)
	}
	if !strings.Contains(got, "click") {
		t.Errorf("trunc lost the text: %q", got)
	}
}
