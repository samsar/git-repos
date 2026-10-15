package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A binding's keys read as one key, a list of them, or none, and write
// back the same way; keys of another type are a problem, and config.json
// still loads.
func TestKeyMap(t *testing.T) {
	var cfg Config
	data := `{"dirs":[],"keys":{"search":"f","move_down":["j","x"],"pull_all":[],"pull":null,"quit":1}}`
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	yours, problems := cfg.KeyMap()
	want := map[string][]string{"search": {"f"}, "move_down": {"j", "x"}, "pull_all": {}, "pull": {}}
	if len(yours) != len(want) {
		t.Errorf("keys %q, want %q", yours, want)
	}
	for name, ks := range want {
		if got := yours[name]; !slices.Equal(got, ks) {
			t.Errorf("%s = %q, want %q", name, got, ks)
		}
	}
	if len(problems) != 1 || !strings.HasPrefix(problems[0], "quit: keys are a key") || !strings.HasSuffix(problems[0], "not 1") {
		t.Errorf("problems %q", problems)
	}
	cfg.SetKeys(map[string][]string{"search": {"f"}, "move_down": {"j", "x"}, "pull_all": {}})
	out, err := json.Marshal(cfg.Keys)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != `{"move_down":["j","x"],"pull_all":[],"search":"f"}` {
		t.Errorf("written back as %s", got)
	}
}

// Save writes the config whole, leaving nothing beside it, and Load reads
// it back; a config it can't read names the file.
func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	want := &Config{Dirs: []string{"/r"}, BootFetch: true}
	want.SetKeys(map[string][]string{"search": {"F"}})
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "git-repos"))
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Errorf("left beside the config: %v", entries)
	}
	got, err := Load()
	yours, _ := got.KeyMap()
	if err != nil || !got.BootFetch || !slices.Equal(yours["search"], []string{"F"}) {
		t.Errorf("loaded %+v, %v", got, err)
	}
	if err := os.WriteFile(Path(), []byte(`{"keys":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), Path()+": ") {
		t.Errorf("err %v doesn't name the file", err)
	}
}

// A config file that is a symlink stays one, the file it points to
// written; and a config file keeps its mode.
func TestSaveKeepsSymlinkAndMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	dotfiles := filepath.Join(dir, "dotfiles", "config.json")
	if err := os.MkdirAll(filepath.Dir(dotfiles), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dotfiles, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dotfiles, Path()); err != nil {
		t.Fatal(err)
	}
	if err := Save(&Config{BootFetch: true}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(Path()); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the config is no longer a symlink: %v, %v", info, err)
	}
	if got, _ := os.ReadFile(dotfiles); !strings.Contains(string(got), "boot_fetch") {
		t.Errorf("the dotfiles copy wasn't written: %s", got)
	}
	if info, _ := os.Stat(dotfiles); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(dotfiles))
	if len(entries) != 1 {
		t.Errorf("left beside the config: %v", entries)
	}
}
