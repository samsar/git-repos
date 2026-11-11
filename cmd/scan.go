package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/samsar/git-repos/internal/config"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/launch"
	"github.com/samsar/git-repos/internal/ui/plain"
	"github.com/samsar/git-repos/internal/ui/sanitize"
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
			fmt.Fprintf(os.Stderr, "warning: no config found; defaulting scan directory to current working directory: %s\n", sanitize.Line(cwd))
			scanDirs = []string{cwd}
		default:
			cfg, err = config.PromptSetup(cfg)
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
			prs, err := git.FetchAndMatchPRs(repos)
			warnPRs(prs, err)
		}
		git.SortRepos(repos)
		return json.NewEncoder(os.Stdout).Encode(redactRepoURLs(repos))
	}

	// Plain mode: non-TTY, --plain, or --no-color
	if !isTTY || flagPlain || flagNoColor {
		return plain.Run(scanDirs, flagFetch, !flagNoPRs, flagNoColor || !isTTY, hidden)
	}

	// TUI mode
	if cfg == nil {
		cfg = &config.Config{}
	}
	bindings, keyProblems := resolveKeys(cfg)
	opener, openerOK := launch.DetectOpener(cfg.Terminal, os.Getenv, runtime.GOOS)
	return tui.Run(tui.Options{
		Dirs:            scanDirs,
		Fetch:           flagFetch || cfg.BootFetch,
		PRs:             !flagNoPRs,
		Hidden:          hidden,
		AutoRefreshMins: cfg.AutoRefreshMins,
		BootFetch:       cfg.BootFetch,
		ConfigPath:      config.Path(),
		Version:         Version,
		Bindings:        bindings,
		KeyProblems:     keyProblems,
		Editors:         launch.Available(cfg.Editors, exec.LookPath),
		DefaultEditor:   cfg.DefaultEditor,
		EditorProblems:  launch.Check(cfg.Editors),
		Opener:          opener,
		OpenerOK:        openerOK,
	})
}

// warnPRs says on stderr why PRs are missing: the lookup failed, or GitHub
// left some out. The repos still print, without them.
func warnPRs(prs git.PRs, err error) {
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "warning: couldn't look up PRs: %v\n", sanitize.Line(err.Error()))
	case prs.Warning != "":
		fmt.Fprintf(os.Stderr, "warning: GitHub left some PRs out: %s\n", sanitize.Line(prs.Warning))
	}
}

// redactRepoURLs is repos with credentials in remote and PR URLs masked:
// --json can end up in a log, a ticket or a CI artifact.
func redactRepoURLs(repos []git.RepoInfo) []git.RepoInfo {
	out := make([]git.RepoInfo, len(repos))
	for i, r := range repos {
		out[i] = r
		out[i].RemoteURL = git.RedactURL(r.RemoteURL)
		out[i].PRUrl = git.RedactURL(r.PRUrl)
		if len(r.Remotes) > 0 {
			out[i].Remotes = make([]git.Remote, len(r.Remotes))
			for j, rem := range r.Remotes {
				out[i].Remotes[j] = rem
				out[i].Remotes[j].URL = git.RedactURL(rem.URL)
			}
		}
	}
	return out
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
