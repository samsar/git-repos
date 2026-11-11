package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func requirePOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell for the hook scripts")
	}
}

// isolatedGit keeps the user's global config out of a test.
func isolatedGit(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func execGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// A repository's core.fsmonitor command must not run when it is scanned:
// merely running git status in a repo is enough to execute it otherwise.
func TestCollectRepoDoesNotRunFSMonitor(t *testing.T) {
	requirePOSIX(t)
	isolatedGit(t)

	repo := t.TempDir()
	execGit(t, repo, "init", "-q")
	execGit(t, repo, "config", "user.email", "test@example.com")
	execGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execGit(t, repo, "add", "file.txt")
	execGit(t, repo, "commit", "-qm", "init")

	script := filepath.Join(repo, "fsmonitor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch \"$0.ran\"\nprintf '{\"version\":1,\"watchman\":\"\",\"clock\":\"c:0:0\"}\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	execGit(t, repo, "config", "core.fsmonitor", script)
	marker := script + ".ran"
	_ = os.Remove(marker)

	CollectRepo(repo, false)

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("core.fsmonitor ran during the scan")
	}
}

// A repository's hooks must not run when git-repos fetches it.
func TestCollectRepoDoesNotRunHooks(t *testing.T) {
	requirePOSIX(t)
	isolatedGit(t)

	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	execGit(t, base, "init", "-q", "--bare", remote)
	execGit(t, remote, "symbolic-ref", "HEAD", "refs/heads/main")

	seed := filepath.Join(base, "seed")
	execGit(t, base, "clone", "-q", remote, seed)
	execGit(t, seed, "config", "user.email", "test@example.com")
	execGit(t, seed, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(seed, "file.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execGit(t, seed, "add", "file.txt")
	execGit(t, seed, "commit", "-qm", "one")
	execGit(t, seed, "push", "-q", "origin", "HEAD:refs/heads/main")

	repo := filepath.Join(base, "repo")
	execGit(t, base, "clone", "-q", remote, repo)

	marker := filepath.Join(base, "hook-ran")
	hook := filepath.Join(repo, ".git", "hooks", "reference-transaction")
	if err := os.WriteFile(hook, []byte(fmt.Sprintf("#!/bin/sh\ntouch %s\n", strconv.Quote(marker))), 0o755); err != nil {
		t.Fatal(err)
	}

	// A new commit on the remote makes the fetch update a ref, which is when
	// the reference-transaction hook would run.
	if err := os.WriteFile(filepath.Join(seed, "file.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execGit(t, seed, "commit", "-qam", "two")
	execGit(t, seed, "push", "-q", "origin", "HEAD:refs/heads/main")

	CollectRepo(repo, true)

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a repository hook ran during the fetch")
	}
}

// Every git command carries the config that keeps a repository from running
// code, and the environment that keeps git from prompting or locking.
func TestRunCmdHardensGit(t *testing.T) {
	requirePOSIX(t)

	bin := t.TempDir()
	argsFile := filepath.Join(bin, "args")
	envFile := filepath.Join(bin, "env")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %s\nprintf '%%s %%s' \"$GIT_TERMINAL_PROMPT\" \"$GIT_OPTIONAL_LOCKS\" > %s\n",
		strconv.Quote(argsFile), strconv.Quote(envFile))
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	RunCmd([]string{"git", "status", "--porcelain"}, "", 5*time.Second)

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	got := string(args)
	wants := []string{
		"core.fsmonitor=false",
		"core.hooksPath=" + os.DevNull,
		"protocol.ext.allow=never",
		"status",
		"--porcelain",
	}
	statusAt := strings.Index(got, "status")
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("git arguments %q don't include %q", got, want)
		}
	}
	if at := strings.Index(got, "core.fsmonitor=false"); at < 0 || at > statusAt {
		t.Errorf("the safe config follows the command: %q", got)
	}

	env, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(env) != "0 0" {
		t.Errorf("git environment = %q, want GIT_TERMINAL_PROMPT and GIT_OPTIONAL_LOCKS 0", env)
	}
}
