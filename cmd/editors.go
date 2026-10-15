package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/samsar/git-repos/internal/config"
	"github.com/samsar/git-repos/internal/launch"
	"github.com/spf13/cobra"
)

var editorsCmd = &cobra.Command{
	Use:   "editors",
	Short: "List the editors e and E open a repo in",
	Long: `List the editors the TUI opens a repo in: yours from config.json, then each
preset whose command is on your PATH, the default (e) marked. E picks one.

Add or change them in config.json:

  "default_editor": "cursor",
  "editors": [
    {"name": "cursor", "cmd": ["cursor", "{path}"]},
    {"name": "nvim",   "cmd": ["nvim", "."], "mode": "inline"},
    {"name": "claude", "cmd": ["claude"],    "mode": "window"}
  ]

{path} is the repo's directory, which is also where the command runs. A
mode is gui (the default: an app beside git-repos), inline (in git-repos's
terminal, which waits for it) or window (a new terminal window, which keeps
a shell in the repo once it exits). Windows open in Ghostty, iTerm2 or
Terminal on macOS, or in tmux; for another terminal, give the command that
opens one, {cmd} standing for the editor's command:

  "terminal": ["wezterm", "start", "--cwd", "{path}", "--", "{cmd}"]`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg == nil {
			cfg = &config.Config{}
		}
		out := cmd.OutOrStdout()
		for _, p := range launch.Check(cfg.Editors) {
			fmt.Fprintf(out, "⚠ config.json: %s, so its editors are left out\n", p)
		}
		editors := launch.Available(cfg.Editors, exec.LookPath)
		def, _ := launch.Pick(editors, cfg.DefaultEditor, os.Getenv)
		var b strings.Builder
		tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		for _, e := range editors {
			mark := ""
			if e.Name == def.Name {
				mark = "default"
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", e.Name, strings.Join(e.Cmd, " "), e.Describe(), mark)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if err := writeTrimmed(out, b.String()); err != nil {
			return err
		}
		if cfg.DefaultEditor != "" && def.Name == "" {
			fmt.Fprintf(out, "⚠ default_editor %q isn't one of these\n", cfg.DefaultEditor)
		}
		if o, ok := launch.DetectOpener(cfg.Terminal, os.Getenv, runtime.GOOS); ok {
			fmt.Fprintf(out, "windows open in %s\n", o.Name)
		} else {
			fmt.Fprintln(out, "windows: this terminal can't open them, so window editors open here instead (set terminal in config.json)")
		}
		return nil
	},
}
