//go:build !unix

package launch

import (
	"errors"
	"os"
	"os/exec"
)

func detach(*exec.Cmd) {}

func runForeground(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func execShell(string, []string) error {
	return errors.New("git repos launch runs on macOS and Linux only")
}
