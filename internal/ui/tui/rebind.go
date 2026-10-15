package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/samsar/git-repos/internal/config"
	"github.com/samsar/git-repos/internal/keys"
)

// The help (?) is also where you rebind the keys, as attn's keys are:
// j/k select a row, the rebind key waits for the new key, and the reset
// key puts the selected binding's defaults back. A new key free wherever
// the binding acts is saved to config.json at once; one another binding
// has there is offered as a swap; a reserved key, or one git-repos
// doesn't take, is refused, saying why, ctrl+c too, while esc gives up.
// The key pressed as the new one is never taken as itself.

// stage is where a rebinding in the help stands.
type stage int

const (
	browsing  stage = iota
	recording       // the next key pressed is the new key
	asking          // swap the key with another binding, or reset both?
)

// keysHelp is the help while open: the row selected, and where a
// rebinding stands.
type keysHelp struct {
	row   int
	stage stage
	// set and reset are the change asked about, saved on y.
	set   map[string][]string
	reset []string
	// note is what the help last had to say: a refusal, a question, or
	// what a change did; warn says it is a refusal.
	note string
	warn bool
}

// helpSelectable is the name of every binding the help shows, in the
// order j and k select them: down each column in turn.
func helpSelectable() []string {
	var out []string
	for _, col := range [][]helpRow{helpNav, helpActions, helpSorts} {
		for _, r := range col {
			if r.name != "" {
				out = append(out, r.name)
			}
		}
	}
	return out
}

// selectedBinding is the binding of the row selected.
func (m model) selectedBinding() (keys.Binding, bool) {
	rows := helpSelectable()
	if m.help.row >= len(rows) {
		return keys.Binding{}, false
	}
	return m.keys.binding(rows[m.help.row])
}

// openHelp shows the help, the first row selected.
func (m model) openHelp() model {
	m.showHelp, m.help = true, keysHelp{}
	return m
}

// handleHelpKey handles a key in the help: y or n while asking about a
// swap or a reset of two; otherwise moving, rebinding, resetting and
// closing. Nothing else acts under it.
func (m model) handleHelpKey(action, key string) (tea.Model, tea.Cmd) {
	if m.help.stage == asking {
		switch key {
		case "y":
			return m.saveKeys(), nil
		case "n", keys.Esc:
			m.help.stage, m.help.note = browsing, ""
		}
		return m, nil
	}
	switch action {
	case keys.Back, keys.Help:
		m.showHelp = false
	case keys.MoveDown:
		m.help.row, m.help.note = min(m.help.row+1, len(helpSelectable())-1), ""
	case keys.MoveUp:
		m.help.row, m.help.note = max(m.help.row-1, 0), ""
	case keys.Rebind:
		if m.canSaveKeys() {
			m.help.stage, m.help.note = recording, ""
		}
	case keys.Reset:
		if m.canSaveKeys() {
			return m.resetSelected(), nil
		}
	}
	return m, nil
}

// canSaveKeys reports whether the help may change the keys: not while
// config.json's keys have problems, which a save would write over; the
// note says so.
func (m *model) canSaveKeys() bool {
	if len(m.keyProblems) > 0 {
		m.help.note, m.help.warn = "config.json's keys have problems, so they can't be changed here: git repos keys check lists them", true
		return false
	}
	return true
}

// record takes key as the selected binding's new key: saved at once when
// it is free wherever the binding acts, asked about when another binding
// has it there; refused, with why, when no binding can take it (ctrl+c,
// reserved, among them), or the swap would leave another key bound twice,
// and then another key can be pressed. Esc gives up.
func (m model) record(key string) model {
	if key == keys.Esc {
		m.help.stage, m.help.note = browsing, ""
		return m
	}
	b, ok := m.selectedBinding()
	if !ok {
		m.help.stage = browsing
		return m
	}
	k, err := keys.Parse(key)
	if err != nil {
		m.help.note, m.help.warn = err.Error(), true
		return m
	}
	set, other, err := keys.Rebinding(m.keys.all(), b.Name, k)
	if err != nil {
		m.help.note, m.help.warn = strings.TrimPrefix(err.Error(), keys.ErrRebind.Error()+": "), true
		return m
	}
	m.help.set, m.help.reset = set, nil
	if other == nil {
		return m.saveKeys()
	}
	m.help.stage, m.help.warn = asking, false
	m.help.note = fmt.Sprintf("%s is already %s: swap them? y/n", keyCap(k), other)
	if len(b.Keys) == 0 {
		m.help.note = fmt.Sprintf("%s is already %s: take it from %s, which keeps no key? y/n", keyCap(k), other, other.Binding.Label)
	}
	return m
}

// resetSelected puts the selected binding's defaults back: at once when
// no other binding has one of them now; asked about when the one that has
// it can go back to its own defaults too (a swap, undone); refused,
// saying why, when the binding has its defaults already, or putting them
// back would bind a key twice otherwise.
func (m model) resetSelected() model {
	b, ok := m.selectedBinding()
	switch {
	case !ok:
		return m
	case !b.Yours:
		m.help.note, m.help.warn = b.Label+" has its default keys already", true
		return m
	}
	reset, key, err := keys.Resetting(m.keys.all(), b.Name)
	if err != nil {
		m.help.note, m.help.warn = strings.TrimPrefix(err.Error(), keys.ErrReset.Error()+": "), true
		return m
	}
	m.help.set, m.help.reset = nil, reset
	if len(reset) == 1 {
		return m.saveKeys()
	}
	m.help.stage, m.help.warn = asking, false
	m.help.note = fmt.Sprintf("%s is %s now: put both back to default? y/n", keyCap(key), m.keys.label(reset[1]))
	return m
}

// saveKeys writes the change asked about to config.json and takes it in,
// saying what it did; or says why it couldn't.
func (m model) saveKeys() model {
	m.help.stage = browsing
	bindings := keys.With(m.keys.all(), m.help.set, m.help.reset)
	if err := saveYourKeys(bindings); err != nil {
		m.help.note, m.help.warn = "couldn't save config.json: "+err.Error(), true
		return m
	}
	before := m.keys
	m.keys = keymap{bindings: bindings}
	m.help.note, m.help.warn = m.savedText(before), false
	return m
}

// savedText says what the change saved made of the keys: the selected
// binding's first, then the other's of a swap.
func (m model) savedText(before keymap) string {
	b, _ := m.selectedBinding()
	names := []string{b.Name}
	for _, name := range slices.Sorted(maps.Keys(m.help.set)) {
		if name != b.Name {
			names = append(names, name)
		}
	}
	for _, name := range m.help.reset {
		if name != b.Name {
			names = append(names, name)
		}
	}
	var changed []string
	for _, name := range names {
		if m.keys.caps(name) != before.caps(name) {
			changed = append(changed, m.keys.label(name)+" is "+m.keys.caps(name))
		}
	}
	end := "saved in config.json"
	if len(m.help.reset) > 0 {
		end = "back to default"
	}
	return strings.Join(changed, " and ") + " now, " + end
}

// aboutBinding is what the help's top line says of b, selected: its keys,
// where they act, and whether they are yours.
func (m model) aboutBinding(b keys.Binding) string {
	var views []keys.View
	for _, v := range b.Views {
		if v != keys.ViewHelp || len(b.Views) == 1 {
			views = append(views, v)
		}
	}
	source := "default"
	if b.Yours {
		source = "yours; " + strings.Join(b.Defaults, " ") + " by default"
		if len(b.Defaults) == 0 {
			source = "yours; none by default"
		}
	}
	return fmt.Sprintf("%s: %s %s · %s", b.Label, m.keys.caps(b.Name), keys.ViewsWords(views), source)
}

// saveYourKeys writes the keys of bindings that are yours to
// config.json's keys, keeping everything else it says.
func saveYourKeys(bindings []keys.Binding) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	yours := map[string][]string{}
	for _, b := range bindings {
		if b.Yours {
			yours[b.Name] = b.Keys
		}
	}
	cfg.SetKeys(yours)
	return config.Save(cfg)
}
