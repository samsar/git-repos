package cmd

import (
	"bufio"
	"fmt"
	"os"

	"github.com/samsar/git-repos/internal/launch"
	"github.com/spf13/cobra"
)

// launchCmd is what a terminal window git-repos opens runs: the editor
// handed off to it, in the repo, then a shell there. A failure stays on
// screen until Enter, rather than closing the window as it shows.
var launchCmd = &cobra.Command{
	Use:           "launch <token>",
	Short:         "Run the editor git-repos handed off to a new terminal window",
	Hidden:        true,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		h, err := launch.TakeHandoff(args[0])
		if err == nil {
			err = launch.Run(h)
		}
		fmt.Fprintf(os.Stderr, "git-repos: %v\npress Enter to close\n", err)
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		return err
	},
}
