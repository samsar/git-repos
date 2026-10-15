package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/samsar/git-repos/internal/config"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/launch"
)

// withEditors is a list of one repo, in a directory of the test's own,
// with editors that touch a file there named for themselves.
func withEditors(t *testing.T) (model, string) {
	t.Helper()
	dir := t.TempDir()
	m := yours(t, nil)
	m.repos = []git.RepoInfo{{Name: "my-api", Path: dir, Branch: "main"}}
	m.editors = []launch.Editor{
		{Name: "first", Cmd: []string{"touch", "{path}/first"}},
		{Name: "second", Cmd: []string{"touch", "second"}},
	}
	return m, dir
}

// run runs cmd, as Bubble Tea would, and gives m what it returns.
func run(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command")
	}
	next, _ := m.Update(cmd())
	return next.(model)
}

// waitFor waits for the file at path, which a detached editor makes.
func waitFor(t *testing.T, path string) {
	t.Helper()
	for range 100 {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("%s never appeared", path)
}

// e opens the selected repo in the default editor, in the repo's
// directory, and says so.
func TestOpenInDefaultEditor(t *testing.T) {
	m, dir := withEditors(t)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = run(t, next.(model), cmd)
	if m.statusMsg != "opened my-api in first" {
		t.Errorf("status %q", m.statusMsg)
	}
	waitFor(t, filepath.Join(dir, "first"))

	m.defaultEditor = "nope"
	if m = press(m, "e"); !strings.Contains(m.statusMsg, `no editor "nope"`) {
		t.Errorf("status %q", m.statusMsg)
	}
}

// E lists the editors, the default selected; a number opens that one,
// in the repo's directory; esc closes it.
func TestPickEditor(t *testing.T) {
	m, dir := withEditors(t)
	m = press(m, "E")
	if !m.showPicker || m.pickerCursor != 0 {
		t.Fatalf("picker %v at %d", m.showPicker, m.pickerCursor)
	}
	view := plain(m.View().Content)
	for _, want := range []string{"Open my-api in…", "1  first", "2  second", "· default"} {
		if !strings.Contains(view, want) {
			t.Errorf("picker doesn't say %q", want)
		}
	}
	if m = press(m, "esc"); m.showPicker {
		t.Error("esc doesn't close the picker")
	}

	next, cmd := press(m, "E").Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if next.(model).showPicker {
		t.Error("the picker stays open")
	}
	m = run(t, next.(model), cmd)
	waitFor(t, filepath.Join(dir, "second"))
	if m.statusMsg != "opened my-api in second" {
		t.Errorf("status %q", m.statusMsg)
	}
}

// An editor that can't start says why.
func TestOpenEditorFails(t *testing.T) {
	m, _ := withEditors(t)
	m.editors = []launch.Editor{{Name: "gone", Cmd: []string{"git-repos-no-such-editor"}}}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = run(t, next.(model), cmd)
	if !strings.HasPrefix(m.statusMsg, "couldn't open: ") {
		t.Errorf("status %q", m.statusMsg)
	}
}

// The settings cycle the default editor, and save it.
func TestSettingsDefaultEditor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m, _ := withEditors(t)
	m.state, m.settingsCursor = stateSettings, 2
	m = press(m, "enter")
	if e, _ := m.theDefaultEditor(); e.Name != "second" {
		t.Errorf("default %q after one press", e.Name)
	}
	m = press(m, "esc")
	if cfg, _ := config.Load(); cfg == nil || cfg.DefaultEditor != "second" {
		t.Errorf("saved %+v", cfg)
	}
}
