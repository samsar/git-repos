package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/samsar/git-repos/internal/config"
	"github.com/samsar/git-repos/internal/keys"
)

// helpOn is the help open on the row of binding name, with config.json
// in a home of the test's own.
func helpOn(t *testing.T, m model, name string) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m = press(m, "?")
	if !m.showHelp {
		t.Fatal("? doesn't open the help")
	}
	m.help.row = slices.Index(helpSelectable(), name)
	if m.help.row < 0 {
		t.Fatalf("%s isn't in the help", name)
	}
	return m
}

// yourKeysOnDisk is config.json's keys, as saved.
func yourKeysOnDisk(t *testing.T) map[string]config.KeyList {
	t.Helper()
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		t.Fatalf("config.json: %v, %v", cfg, err)
	}
	return cfg.Keys
}

// A free key is saved at once, acts at once, and says so.
func TestRebindFreeKey(t *testing.T) {
	m := helpOn(t, yours(t, nil), keys.Search)
	m = press(m, "r")
	if m.help.stage != recording {
		t.Fatal("r doesn't wait for the new key")
	}
	m = press(m, "F")
	if m.help.stage != browsing || m.help.note != "Search is F now, saved in config.json" {
		t.Errorf("stage %d, note %q", m.help.stage, m.help.note)
	}
	if got := yourKeysOnDisk(t); len(got) != 1 || !slices.Equal(got[keys.Search], config.KeyList{"F"}) {
		t.Errorf("saved %v", got)
	}
	m = press(m, "esc")
	if m.showHelp || !press(m, "F").searching {
		t.Error("F doesn't search after the help closes")
	}
}

// A key another binding has where this one acts is offered as a swap;
// putting one back to default puts both back, asked first.
func TestRebindSwapAndReset(t *testing.T) {
	m := helpOn(t, yours(t, nil), keys.Pull)
	m = press(press(m, "r"), "s")
	if m.help.stage != asking || m.help.note != "s is already Open shell in the list and in the detail view: swap them? y/n" {
		t.Fatalf("stage %d, note %q", m.help.stage, m.help.note)
	}
	m = press(m, "y")
	if got := yourKeysOnDisk(t); !slices.Equal(got[keys.Pull], config.KeyList{"s"}) || !slices.Equal(got[keys.Shell], config.KeyList{"p"}) {
		t.Errorf("saved %v", got)
	}
	if m.help.note != "Pull repo is s and Open shell is p now, saved in config.json" {
		t.Errorf("note %q", m.help.note)
	}

	m = press(m, "backspace")
	if m.help.stage != asking || m.help.note != "p is Open shell now: put both back to default? y/n" {
		t.Fatalf("stage %d, note %q", m.help.stage, m.help.note)
	}
	m = press(m, "y")
	if got := yourKeysOnDisk(t); len(got) != 0 {
		t.Errorf("still saved %v", got)
	}
	if m.keys.first(keys.Pull) != "p" || m.keys.first(keys.Shell) != "s" {
		t.Error("not back to the defaults")
	}
}

// A key no binding takes is refused, saying why, and another can be
// pressed: ctrl+c too, which doesn't quit meanwhile. Esc gives up.
func TestRebindRefusals(t *testing.T) {
	m := helpOn(t, yours(t, nil), keys.Search)
	m = press(m, "r")
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(model)
	if cmd != nil {
		t.Error("ctrl+c acted while waiting for the new key")
	}
	if m.help.stage != recording || !m.help.warn || !strings.Contains(m.help.note, "ctrl+c always quits") {
		t.Errorf("stage %d, note %q", m.help.stage, m.help.note)
	}
	m = press(m, "esc")
	if m.help.stage != browsing || !m.showHelp {
		t.Error("esc doesn't give up, leaving the help open")
	}
}

// With problems in config.json's keys, the help changes none of them,
// so a save can't write over what you wrote.
func TestNoRebindWithProblems(t *testing.T) {
	m := yours(t, nil)
	m.keyProblems = []string{`there is no "serch" to give keys to`}
	m = helpOn(t, m, keys.Search)
	m = press(m, "r")
	if m.help.stage != browsing || !m.help.warn {
		t.Errorf("stage %d, note %q", m.help.stage, m.help.note)
	}
}

// Saving the settings keeps config.json's keys.
func TestSaveSettingsKeepsKeys(t *testing.T) {
	m := helpOn(t, yours(t, nil), keys.Search)
	m = press(press(m, "r"), "F")
	m.bootFetch = true
	if err := m.saveConfig(); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load()
	if !cfg.BootFetch || !slices.Equal(cfg.Keys[keys.Search], config.KeyList{"F"}) {
		t.Errorf("saved %+v", cfg)
	}
}
