package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samsar/git-repos/internal/config"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/ui/plain"
	"github.com/samsar/git-repos/internal/ui/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func runScan(cmd *cobra.Command, args []string) error {
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))

	// Always load config so we can apply the hidden list regardless of how
	// dirs were specified.
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	hidden := buildHiddenSet(cfg)

	// Resolve scan dirs: args override config
	var scanDirs []string
	if len(args) > 0 {
		for _, a := range args {
			abs, err := filepath.Abs(config.ExpandHome(a))
			if err != nil {
				return err
			}
			scanDirs = append(scanDirs, abs)
		}
	} else {
		switch {
		case cfg != nil && len(cfg.Dirs) > 0:
			scanDirs = cfg.Dirs
		case flagJSON || flagPlain || !stdinTTY:
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "warning: no config found; defaulting scan directory to current working directory: %s\n", cwd)
			scanDirs = []string{cwd}
		default:
			cfg, err = config.PromptSetup()
			if err != nil {
				return err
			}
			scanDirs = cfg.Dirs
			if len(scanDirs) == 0 {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				scanDirs = []string{cwd}
			}
		}
	}

	// JSON mode
	if flagJSON {
		repos, err := git.ScanAll(scanDirs, flagFetch)
		if err != nil {
			return err
		}
		repos = filterHidden(repos, hidden)
		if !flagNoPRs {
			git.FetchAndMatchPRs(repos)
		}
		git.SortRepos(repos)
		return json.NewEncoder(os.Stdout).Encode(repos)
	}

	// Plain mode: non-TTY, --plain, or --no-color
	if !isTTY || flagPlain || flagNoColor {
		return plain.Run(scanDirs, flagFetch, !flagNoPRs, flagNoColor || !isTTY, hidden)
	}

	// TUI mode
	autoRefreshMins := 0
	bootFetch := false
	if cfg != nil {
		autoRefreshMins = cfg.AutoRefreshMins
		bootFetch = cfg.BootFetch
	}
	doFetch := flagFetch || bootFetch
	return tui.Run(scanDirs, doFetch, !flagNoPRs, hidden, autoRefreshMins, bootFetch, config.Path())
}

func buildHiddenSet(cfg *config.Config) map[string]bool {
	if cfg == nil || len(cfg.Hidden) == 0 {
		return nil
	}
	h := make(map[string]bool, len(cfg.Hidden))
	for _, name := range cfg.Hidden {
		h[name] = true
	}
	return h
}

func filterHidden(repos []git.RepoInfo, hidden map[string]bool) []git.RepoInfo {
	if len(hidden) == 0 {
		return repos
	}
	out := make([]git.RepoInfo, 0, len(repos))
	for _, r := range repos {
		if !hidden[r.Name] {
			out = append(out, r)
		}
	}
	return out
}
