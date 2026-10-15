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
// back the same way.
func TestKeyList(t *testing.T) {
	var cfg Config
	data := `{"dirs":[],"keys":{"search":"f","move_down":["j","x"],"pull_all":[],"pull":null}}`
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"search": {"f"}, "move_down": {"j", "x"}, "pull_all": {}, "pull": {}}
	for name, ks := range want {
		if got := cfg.Keys[name]; !slices.Equal(got, ks) {
			t.Errorf("%s = %q, want %q", name, got, ks)
		}
	}
	out, err := json.Marshal(cfg.Keys)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != `{"move_down":["j","x"],"pull":[],"pull_all":[],"search":"f"}` {
		t.Errorf("written back as %s", got)
	}
	if err := json.Unmarshal([]byte(`{"keys":{"search":3}}`), &cfg); err == nil {
		t.Error("a number taken as keys")
	}
}

// Save writes the config whole, leaving nothing beside it, and Load reads
// it back; a config it can't read names the file.
func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	want := &Config{Dirs: []string{"/r"}, BootFetch: true, Keys: map[string]KeyList{"search": {"F"}}}
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
	if err != nil || !got.BootFetch || !slices.Equal(got.Keys["search"], KeyList{"F"}) {
		t.Errorf("loaded %+v, %v", got, err)
	}
	if err := os.WriteFile(Path(), []byte(`{"keys":{"search":3}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), Path()+": ") {
		t.Errorf("err %v doesn't name the file", err)
	}
}
