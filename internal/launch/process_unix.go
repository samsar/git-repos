//go:build unix

package launch

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
)

// detach makes cmd a process group of its own, without a terminal, so
// that git-repos's signals and exit leave it running.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// runForeground runs args on this terminal until they exit. Meanwhile
// ctrl+c and ctrl+\ are theirs: git-repos catches and drops them, which,
// unlike ignoring them, leaves them their default in the command.
func runForeground(args []string) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT)
	defer signal.Stop(sigs)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// execShell becomes shell, as a login shell, as a terminal starts one.
func execShell(shell string, env []string) error {
	return syscall.Exec(shell, []string{"-" + filepath.Base(shell)}, env)
}
