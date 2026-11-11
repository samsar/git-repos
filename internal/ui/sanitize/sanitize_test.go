package sanitize

import (
	"strings"
	"testing"
)

func TestLine(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "fix token refresh", "fix token refresh"},
		{"unicode", "café 👍 日本語", "café 👍 日本語"},
		{"sgr", "a\x1b[31mred\x1b[0m b", "ared b"},
		{"osc hyperlink", "a\x1b]8;;https://evil.example\x1b\\click\x1b]8;;\x1b\\ b", "aclick b"},
		{"clear screen", "a\x1b[2J\x1b[Hb", "ab"},
		{"bare controls", "a\x07b\x00c\x7fd", "abcd"},
		{"c1", "a\u0085b", "ab"},
		{"newline and tab", "a\nb\tc\r\nd", "a b c  d"},
		{"bidi override", "a\u202Eb", "ab"},
		{"bidi isolate", "a\u2067b\u2069c", "abc"},
		{"zero width", "a\u200bb\u2060c\ufeffd", "abcd"},
		{"lone escape", "a\x1bb", "a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Line(c.in); got != c.want {
				t.Errorf("Line(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestWebURL(t *testing.T) {
	good := []string{
		"https://github.com/acme/api/pull/42",
		"http://github.example/acme/api/pull/42",
		"https://github.com/acme/api/pull/42?x=1&y=2",
	}
	for _, raw := range good {
		if got, ok := WebURL(raw); !ok || got == "" {
			t.Errorf("WebURL(%q) = %q, %v, want a URL", raw, got, ok)
		}
	}
	bad := []string{
		"",
		"javascript:alert(1)",
		"file:///etc/passwd",
		"data:text/html,hi",
		"//github.com/acme/api",
		"https://",
		"https://github.com/\x1b]8;;https://evil.example\x1b\\",
		"https://github.com/\x1b[2J",
		"https://github.com/a\nb",
	}
	for _, raw := range bad {
		if got, ok := WebURL(raw); ok {
			t.Errorf("WebURL(%q) = %q, true, want false", raw, got)
		}
	}
}

func TestLineLeavesSpacesAlone(t *testing.T) {
	if got := Line("a  b"); got != "a  b" {
		t.Errorf("Line(%q) = %q", "a  b", got)
	}
	if strings.ContainsRune(Line("x\x1by"), '\x1b') {
		t.Error("a bare escape survived")
	}
}
