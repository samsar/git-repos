package keys

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// Parse takes the keys every terminal sends alike, as Bubble Tea names
// them, and refuses the rest, saying why, and what to write instead when
// there is something.
func TestParse(t *testing.T) {
	good := []struct{ name, want string }{
		{"a", "a"}, {"A", "A"}, {"?", "?"}, {"!", "!"}, {"[", "["}, {"é", "é"}, {"1", "1"},
		{"enter", "enter"}, {"tab", "tab"}, {"shift+tab", "shift+tab"}, {"space", "space"}, {"backspace", "backspace"},
		{"up", "up"}, {"pgdown", "pgdown"}, {"f1", "f1"}, {"f12", "f12"},
		{"ctrl+d", "ctrl+d"}, {"ctrl+D", "ctrl+d"}, {"control+d", "ctrl+d"}, {"ctrl+h", "ctrl+h"},
		{"esc", "esc"}, {"ctrl+c", "ctrl+c"},
	}
	for _, tt := range good {
		if got, err := Parse(tt.name); err != nil || got != tt.want {
			t.Errorf("Parse(%q) = %q, %v; want %q", tt.name, got, err, tt.want)
		}
	}
	bad := []struct{ name, why string }{
		{"", "it is empty"},
		{" ", "write it space"},
		{"\x01", "it isn't a character a terminal types"},
		{"g g", "never a sequence"},
		{"Escape", "write it esc"},
		{"return", "write it enter"},
		{"Enter", "write it enter"},
		{"pageup", "write it pgup"},
		{"ctrl+i", "a terminal sends it as tab"},
		{"ctrl+m", "a terminal sends it as enter"},
		{"ctrl+[", "a terminal sends it as esc"},
		{"ctrl+1", "ctrl goes with a letter"},
		{"ctrl+up", "ctrl goes with a letter"},
		{"ctrl+shift+a", "only terminals that report every modifier"},
		{"shift+a", "write the capital: A"},
		{"Shift+Tab", "write it shift+tab"},
		{"shift+up", "shift goes with tab alone"},
		{"alt+x", "git-repos takes no alt"},
		{"option+x", "git-repos takes no alt"},
		{"cmd+k", "the terminal keeps cmd for itself"},
		{"super+k", "the terminal keeps super for itself"},
		{"f13", "it isn't a key name"},
		{"archived", "it isn't a key name"},
	}
	for _, tt := range bad {
		_, err := Parse(tt.name)
		if !errors.Is(err, ErrNotAKey) || !strings.Contains(err.Error(), tt.why) || !strings.HasPrefix(err.Error(), `"`) {
			t.Errorf("Parse(%q) = %v; want ErrNotAKey saying %q", tt.name, err, tt.why)
		}
	}
}

func TestCap(t *testing.T) {
	for key, want := range map[string]string{"enter": "⏎", "up": "↑", "down": "↓", "left": "←", "right": "→", "backspace": "⌫", "a": "a", "ctrl+d": "ctrl+d"} {
		if got := Cap(key); got != want {
			t.Errorf("Cap(%q) = %q, want %q", key, got, want)
		}
	}
}

// The universe is every key Parse takes, but the reserved ones, each once.
func TestUniverse(t *testing.T) {
	u := Universe()
	if len(u) != 26+26+10+32+len(named)+23 {
		t.Errorf("%d keys", len(u))
	}
	seen := map[string]bool{}
	for _, k := range u {
		if got, err := Parse(k); err != nil || got != k {
			t.Errorf("%q in the universe: Parse = %q, %v", k, got, err)
		}
		if seen[k] || Reserved(k) {
			t.Errorf("%q twice, or reserved", k)
		}
		seen[k] = true
	}
	if !slices.Contains(u, "ctrl+d") || slices.Contains(u, "ctrl+i") {
		t.Error("ctrl keys wrong")
	}
}
