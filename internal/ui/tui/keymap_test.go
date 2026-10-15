package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/keys"
)

// press sends m the key named key, as a terminal would type it.
func press(m model, key string) model {
	msg := tea.KeyPressMsg{Text: key}
	switch key {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		msg.Code = []rune(key)[0]
	}
	next, _ := m.Update(msg)
	return next.(model)
}

// yours is a list view with config.json's keys over the defaults.
func yours(t *testing.T, given map[string][]string) model {
	t.Helper()
	bindings, problems := keys.FromConfig(given)
	if len(problems) > 0 {
		t.Fatalf("problems: %q", problems)
	}
	return resize(model{
		state:   stateList,
		spinner: newSpinner(),
		keys:    keymap{bindings: bindings},
		repos:   []git.RepoInfo{repo("my-api", "main", "init")},
	}, 120, 40)
}

// Your keys act in place of the defaults, which then do nothing.
func TestYourKeysAct(t *testing.T) {
	m := yours(t, map[string][]string{keys.Search: {"f"}, keys.Details: {"l"}})

	if !press(m, "f").searching {
		t.Error("f doesn't search")
	}
	if press(m, "/").searching {
		t.Error("/ still searches")
	}
	if press(m, "enter").state == stateDetail {
		t.Error("enter still opens the detail view")
	}
	detail := press(m, "l")
	if detail.state != stateDetail {
		t.Fatal("l doesn't open the detail view")
	}
	if press(detail, "esc").state != stateList {
		t.Error("esc doesn't go back")
	}
}

// The header's hints, the help and the search's hint show your keys.
func TestHintsShowYourKeys(t *testing.T) {
	m := yours(t, map[string][]string{keys.Search: {"f"}, keys.PullAll: {}})

	header := strings.Join(strings.Fields(plain(strings.Join(m.actionLines(), " "))), " ")
	if !strings.Contains(header, "<f> Search") || strings.Contains(header, "</>") {
		t.Errorf("header hints:\n%s", header)
	}
	if strings.Contains(header, "Pull all") {
		t.Errorf("a binding without keys hinted:\n%s", header)
	}

	m.showHelp = true
	help := plain(m.View().Content)
	if !strings.Contains(help, "<f>") || !strings.Contains(help, "<–>") {
		t.Errorf("help doesn't show f for search and – for pull all:\n%s", help)
	}

	m.showHelp = false
	m = press(press(m, "f"), "x")
	m = press(m, "enter")
	if bar := plain(m.renderStatusBar()); !strings.Contains(bar, "(f to edit") {
		t.Errorf("status bar: %q", bar)
	}
}

// The number being edited in settings takes every key: q types nothing,
// and doesn't quit.
func TestSettingsFieldTakesEveryKey(t *testing.T) {
	m := yours(t, nil)
	m.state = stateSettings
	m = press(m, "e")
	if !m.settingsEditing {
		t.Fatal("e doesn't edit")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Error("q quit while editing")
		}
	}
	if !next.(model).settingsEditing {
		t.Error("q closed the field")
	}
}
