package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samsar/git-repos/internal/launch"
)

type Config struct {
	Dirs            []string                   `json:"dirs"`
	Hidden          []string                   `json:"hidden,omitempty"`
	AutoRefreshMins int                        `json:"auto_refresh_mins,omitempty"` // 0 = disabled
	BootFetch       bool                       `json:"boot_fetch,omitempty"`        // run git fetch at startup
	Keys            map[string]json.RawMessage `json:"keys,omitempty"`              // binding name → your keys, over its defaults (KeyMap)
	DefaultEditor   string                     `json:"default_editor,omitempty"`    // the editor e opens a repo in
	Editors         []launch.Editor            `json:"editors,omitempty"`           // yours, over the presets of their names
	Terminal        []string                   `json:"terminal,omitempty"`          // opens a window: {path} the repo, {cmd} the editor
}

// KeyList is a binding's keys in config.json: one key as a string
// ("search": "f"), several as a list ("move_down": ["j", "x"]), or none
// as an empty list or null.
type KeyList []string

func (k *KeyList) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*k = KeyList{}
		return nil
	}
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*k = KeyList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return fmt.Errorf("keys are a key, such as \"f\", or a list of them, such as [\"j\", \"down\"]")
	}
	*k = KeyList(many)
	return nil
}

func (k KeyList) MarshalJSON() ([]byte, error) {
	switch len(k) {
	case 0:
		return []byte("[]"), nil
	case 1:
		return json.Marshal(k[0])
	}
	return json.Marshal([]string(k))
}

// KeyMap is c's keys by binding name, as package keys takes them, and a
// problem, in the order of their names, for each binding whose keys are
// neither a key nor a list of them, which it leaves out: config.json
// still loads, so the TUI can start on the default keys and say why.
func (c *Config) KeyMap() (map[string][]string, []string) {
	if c == nil {
		return nil, nil
	}
	out := make(map[string][]string, len(c.Keys))
	var problems []string
	for _, name := range slices.Sorted(maps.Keys(c.Keys)) {
		var ks KeyList
		if err := json.Unmarshal(c.Keys[name], &ks); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v, not %s", name, err, c.Keys[name]))
			continue
		}
		out[name] = ks
	}
	return out, problems
}

// SetKeys makes yours, by binding name, c's keys; none for an empty map.
func (c *Config) SetKeys(yours map[string][]string) {
	c.Keys = nil
	for name, ks := range yours {
		data, err := json.Marshal(KeyList(ks))
		if err != nil {
			continue // a list of strings always marshals
		}
		if c.Keys == nil {
			c.Keys = map[string]json.RawMessage{}
		}
		c.Keys[name] = data
	}
}

func Path() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "git-repos", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "git-repos", "config.json")
}

func Load() (*Config, error) {
	return LoadFrom(Path())
}

// LoadFrom reads the config file at path; nil, with no error, when there
// is none.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

// Save writes cfg to the config file whole, or not at all: to a file
// beside it, then renamed over it, so a write cut short never leaves a
// config git-repos can't read. A config file that is a symlink (into a
// dotfiles repo, say) stays one: the file it points to is the one
// written. The file keeps its mode.
func Save(cfg *Config) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // gone once renamed; else left by a failure
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// PromptSetup asks which directory to scan and saves it into existing,
// keeping everything else it says (such as keys rebound from the TUI
// before any directory was set); existing may be nil.
func PromptSetup(existing *Config) (*Config, error) {
	cfg := &Config{}
	if existing == nil {
		fmt.Printf("\nNo configuration found.\n")
	} else {
		copied := *existing
		cfg = &copied
		fmt.Printf("\nNo directory to scan is configured.\n")
	}
	fmt.Printf("Which directory contains your git repos?\n")
	fmt.Printf("(press Enter to use the current directory each time)\n> ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	line = strings.TrimSpace(line)

	if line == "" {
		fmt.Println()
		return cfg, nil
	}

	abs, err := filepath.Abs(ExpandHome(line))
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("directory not found: %s", abs)
	}

	cfg.Dirs = []string{abs}
	if err := Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save config: %v\n", err)
	} else {
		fmt.Printf("Config saved to %s\n", Path())
		fmt.Printf("Edit that file to add more directories.\n\n")
	}
	return cfg, nil
}

func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
