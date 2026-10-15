package config

import (
	"encoding/json"
	"slices"
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
