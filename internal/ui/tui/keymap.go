package tui

import (
	"slices"
	"strings"

	"github.com/samsar/git-repos/internal/keys"
)

// defaultBindings are the bindings of a keymap given none.
var defaultBindings = keys.Defaults()

// keymap is every key the TUI takes, by binding: yours from config.json
// over the defaults. Every key the TUI shows or takes comes from it. The
// zero keymap is the defaults.
type keymap struct {
	bindings []keys.Binding
}

func (k keymap) all() []keys.Binding {
	if k.bindings == nil {
		return defaultBindings
	}
	return k.bindings
}

// in is the binding pressed is a key of in view; "" for none. Esc is
// always Back.
func (k keymap) in(view keys.View, pressed string) string {
	if pressed == keys.Esc {
		return keys.Back
	}
	for _, b := range k.all() {
		if slices.Contains(b.Views, view) && slices.Contains(b.Keys, pressed) {
			return b.Name
		}
	}
	return ""
}

// keysOf is every key of binding name, esc first for Back, which it
// always has.
func (k keymap) keysOf(name string) []string {
	var out []string
	if name == keys.Back {
		out = append(out, keys.Esc)
	}
	if i := slices.IndexFunc(k.all(), func(b keys.Binding) bool { return b.Name == name }); i >= 0 {
		out = append(out, k.all()[i].Keys...)
	}
	return out
}

// label is binding name's label; name itself for one there isn't.
func (k keymap) label(name string) string {
	if i := slices.IndexFunc(k.all(), func(b keys.Binding) bool { return b.Name == name }); i >= 0 {
		return k.all()[i].Label
	}
	return name
}

// first is the first key of binding name, as a hint shows it; "" when it
// has none.
func (k keymap) first(name string) string {
	if ks := k.keysOf(name); len(ks) > 0 {
		return keyCap(ks[0])
	}
	return ""
}

// caps is every key of binding name, as the help shows them; "–" for
// none.
func (k keymap) caps(name string) string {
	ks := k.keysOf(name)
	if len(ks) == 0 {
		return "–"
	}
	out := make([]string, len(ks))
	for i, key := range ks {
		out[i] = keyCap(key)
	}
	return strings.Join(out, " ")
}

// keyCap is key as the TUI shows it: an arrow for the arrow keys, its
// name, as config.json writes it, otherwise.
func keyCap(key string) string {
	switch key {
	case "up", "down", "left", "right":
		return keys.Cap(key)
	}
	return key
}

// keyView is the view the keys act in now.
func (m model) keyView() keys.View {
	switch m.state {
	case stateDetail:
		return keys.ViewDetail
	case stateSettings:
		return keys.ViewSettings
	}
	return keys.ViewList
}
