// Package keys is git-repos's keys: which keys it takes, what each of the
// TUI's views binds them to, your keys from config.json over the
// defaults, and the conflicts between them. The TUI draws and takes its
// keys from what Resolve gives, and git repos keys check checks a file
// with the same rules. Ported from attn's keys package.
//
// A key is named as Bubble Tea names the key press: its character (a, A,
// ?), a named key (enter, tab, up, f1), or ctrl with a letter (ctrl+d).
package keys

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The reserved keys: no binding takes them, and config.json can't take
// them away.
const (
	// CtrlC always quits.
	CtrlC = "ctrl+c"
	// Esc always goes back.
	Esc = "esc"
)

// ErrNotAKey: a name git-repos doesn't take as a key. The error says why, and
// what to write instead when there is something.
var ErrNotAKey = errors.New("not a key git-repos takes")

// named are the keys with names every terminal sends alike, in the order
// git repos keys --free lists them.
var named = []string{
	"enter", "tab", "shift+tab", "backspace", "space", "delete", "insert",
	"up", "down", "left", "right", "home", "end", "pgup", "pgdown",
	"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12",
}

// spelled are other spellings of named keys, by the one git-repos takes.
var spelled = map[string]string{
	"return": "enter", "escape": Esc, "spacebar": "space", "del": "delete", "ins": "insert",
	"pageup": "pgup", "page_up": "pgup", "pagedown": "pgdown", "page_down": "pgdown",
	"backtab": "shift+tab", "arrowup": "up", "arrowdown": "down", "arrowleft": "left", "arrowright": "right",
}

// terminalKeys are ctrl keys a terminal sends as another key: the same
// byte.
var terminalKeys = map[string]string{"ctrl+i": "tab", "ctrl+m": "enter", "ctrl+[": Esc}

// Parse is the key name names, as git-repos keeps it, or why it doesn't
// take it (ErrNotAKey). git-repos takes the keys every terminal sends in its
// plain encoding, without settings of its own: a printable character, a
// named key, and ctrl with a letter. ctrl+D is ctrl+d: a terminal sends
// no shift with ctrl.
func Parse(name string) (string, error) {
	switch {
	case name == "":
		return "", notAKey(name, "it is empty")
	case utf8.RuneCountInString(name) == 1:
		return printable(name)
	case slices.Contains(named, name), name == Esc, name == CtrlC:
		return name, nil
	case strings.ContainsAny(name, " \t"):
		return "", notAKey(name, "git-repos takes one key at a time, never a sequence of them")
	}
	if want, ok := spelled[strings.ToLower(name)]; ok {
		return "", notAKey(name, "write it "+want)
	}
	if lower := strings.ToLower(name); lower != name && (slices.Contains(named, lower) || lower == Esc) {
		return "", notAKey(name, "write it "+lower)
	}
	mods, base, _ := strings.Cut(name, "+")
	switch strings.ToLower(mods) {
	case "ctrl", "control":
		return ctrlKey(name, base)
	case "shift":
		return shiftKey(name, base)
	case "alt", "option", "opt", "meta":
		return "", notAKey(name, "macOS terminals type a character for Option+x (≈) unless set to send Esc+, so git-repos takes no alt")
	case "cmd", "command", "super", "hyper", "fn":
		return "", notAKey(name, "the terminal keeps "+strings.ToLower(mods)+" for itself")
	}
	return "", notAKey(name, "it isn't a key name; a key is a character (a, A, ?), a named key (enter, tab, up, f1), or ctrl with a letter (ctrl+d)")
}

// printable is the one-character key s, when it is one a terminal types.
func printable(s string) (string, error) {
	r, _ := utf8.DecodeRuneInString(s)
	switch {
	case r == ' ':
		return "", notAKey(s, "write it space")
	case unicode.IsControl(r), !unicode.IsGraphic(r):
		return "", notAKey(s, "it isn't a character a terminal types")
	}
	return s, nil
}

// ctrlKey is ctrl with base, name's: a letter, unless the terminal sends
// it as another key, or the reserved ctrl+c.
func ctrlKey(name, base string) (string, error) {
	key := "ctrl+" + strings.ToLower(base)
	if same, ok := terminalKeys[key]; ok {
		return "", notAKey(name, "a terminal sends it as "+same)
	}
	if len(base) != 1 || !isLetter(base[0]) {
		if strings.Contains(base, "+") {
			return "", notAKey(name, "only terminals that report every modifier send ctrl with another one")
		}
		return "", notAKey(name, "ctrl goes with a letter: terminals send ctrl with anything else differently, or not at all")
	}
	return key, nil
}

// shiftKey is shift with base, name's: shift+tab alone has a name; a
// capital is its character.
func shiftKey(name, base string) (string, error) {
	switch {
	case len(base) == 1 && isLetter(base[0]):
		return "", notAKey(name, "write the capital: "+strings.ToUpper(base))
	case strings.EqualFold(base, "tab"):
		return "", notAKey(name, "write it shift+tab")
	}
	return "", notAKey(name, "shift goes with tab alone: write the character it types instead (? for shift+/)")
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// notAKeyError is a name git-repos doesn't take as a key, and why.
type notAKeyError struct{ name, why string }

func (e *notAKeyError) Error() string {
	return fmt.Sprintf("%q isn't a key git-repos takes: %s", e.name, e.why)
}

func (*notAKeyError) Is(target error) bool { return target == ErrNotAKey }

func notAKey(name, why string) error { return &notAKeyError{name: name, why: why} }

// Reserved reports whether key is one no binding takes.
func Reserved(key string) bool { return key == CtrlC || key == Esc }

// Cap is key as a key cap shows it: an arrow or a symbol for the keys
// that have one, the name otherwise.
func Cap(key string) string {
	switch key {
	case "enter":
		return "⏎"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "backspace":
		return "⌫"
	}
	return key
}

// Universe is every key git-repos takes but the reserved ones, as git repos
// keys --free lists them: the letters, the capitals, the digits and the
// symbols of ASCII, the named keys, then ctrl with a letter. Characters
// past ASCII are keys too, but no list of them would end.
func Universe() []string {
	var out []string
	for _, span := range [][2]byte{{'a', 'z'}, {'A', 'Z'}, {'0', '9'}} {
		for c := span[0]; c <= span[1]; c++ {
			out = append(out, string(c))
		}
	}
	for c := byte('!'); c <= '~'; c++ {
		if !isLetter(c) && (c < '0' || c > '9') {
			out = append(out, string(c))
		}
	}
	out = append(out, named...)
	for c := byte('a'); c <= 'z'; c++ {
		if key := "ctrl+" + string(c); key != CtrlC && terminalKeys[key] == "" {
			out = append(out, key)
		}
	}
	return out
}
