// Package sanitize makes text from repositories and GitHub safe to draw:
// commit messages, branch names, file paths and PR titles are written by
// other people, and a terminal reads escape sequences in them.
package sanitize

import (
	"net/url"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Line makes s safe to draw on one terminal line: ANSI escape sequences,
// control characters (bare ESC and BEL included), bidirectional overrides
// that can disguise text, and zero-width formatting characters are removed,
// and a line break or tab becomes a space. Printable text, including emoji
// and other scripts, is kept.
func Line(s string) string {
	s = ansi.Strip(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t' || r == '\u2028' || r == '\u2029':
			b.WriteByte(' ')
		case unsafe(r):
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unsafe reports whether r is a control or formatting rune that has no
// place in a line of terminal text.
func unsafe(r rune) bool {
	if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
		return true
	}
	switch r {
	case '\u061c', // Arabic letter mark
		'\u200b',           // zero-width space
		'\u200e', '\u200f', // left-to-right, right-to-left mark
		'\u202a', '\u202b', '\u202c', '\u202d', '\u202e', // bidi embeddings and overrides
		'\u2060',                               // word joiner
		'\u2066', '\u2067', '\u2068', '\u2069', // bidi isolates
		'\ufeff': // byte order mark
		return true
	}
	return false
}

// WebURL is raw as a URL worth opening or putting in a terminal hyperlink:
// an http or https URL with a host and no control characters. False for
// anything else, such as a javascript: or file: URL, or one with an escape
// sequence in it.
func WebURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.IndexFunc(raw, unsafe) >= 0 ||
		strings.ContainsAny(raw, "\n\r\t") {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	return u.String(), true
}
