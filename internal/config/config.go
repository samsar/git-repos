package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Dirs            []string `json:"dirs"`
	Hidden          []string `json:"hidden,omitempty"`
	AutoRefreshMins int      `json:"auto_refresh_mins,omitempty"` // 0 = disabled
}

func Path() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "git-repos", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "git-repos", "config.json")
}

func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func PromptSetup() (*Config, error) {
	fmt.Printf("\nNo configuration found.\n")
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
		return &Config{}, nil
	}

	abs, err := filepath.Abs(ExpandHome(line))
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("directory not found: %s", abs)
	}

	cfg := &Config{Dirs: []string{abs}}
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
