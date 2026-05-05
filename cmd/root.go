package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	flagFetch   bool
	flagNoPRs   bool
	flagNoColor bool
	flagPlain   bool
	flagJSON    bool
)

var rootCmd = &cobra.Command{
	Use:   "git-repos [DIR...]",
	Short: "Bird's-eye view of all your git repos",
	Long: `git-repos gives you a bird's-eye view of all git repositories
in a directory: branch, sync state, working-tree changes, stash,
last commit, and open GitHub PRs.

Installed as git-repos it is auto-discovered as a git subcommand:

  git repos
  git repos ~/projects
  git repos --fetch --plain ~/projects`,
	Args: cobra.ArbitraryArgs,
	RunE: runScan,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagFetch, "fetch", false, "Run git fetch in each repo before scanning")
	rootCmd.PersistentFlags().BoolVar(&flagNoPRs, "no-prs", false, "Skip GitHub PR lookup")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable color output")
	rootCmd.Flags().BoolVar(&flagPlain, "plain", false, "Plain text output instead of TUI")
	rootCmd.Flags().BoolVar(&flagJSON, "json", false, "JSON output (implies --plain)")

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
