package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RepoInfo struct {
	Path       string `json:"path,omitempty"`
	Name       string `json:"name"`
	Branch     string `json:"branch"`
	Ahead      int    `json:"ahead,omitempty"`
	Behind     int    `json:"behind,omitempty"`
	NoUpstream bool   `json:"no_upstream,omitempty"`
	Staged     int    `json:"staged,omitempty"`
	Modified   int    `json:"modified,omitempty"`
	Untracked  int    `json:"untracked,omitempty"`
	StashCount int    `json:"stash_count,omitempty"`
	LastTS     int64  `json:"last_ts,omitempty"`
	LastRel    string `json:"last_rel,omitempty"`
	LastMsg    string `json:"last_msg,omitempty"`
	RemoteURL  string `json:"remote_url,omitempty"`
	PRNumber   int    `json:"pr_number,omitempty"`
	PRUrl      string `json:"pr_url,omitempty"`
	PRState    string `json:"pr_state,omitempty"`
	Error      string `json:"error,omitempty"`
}

func RunCmd(args []string, cwd string, timeout time.Duration) (stdout, stderr string, code int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Dir = cwd
	var ob, eb bytes.Buffer
	c.Stdout = &ob
	c.Stderr = &eb
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	return strings.TrimSpace(ob.String()), strings.TrimSpace(eb.String()), code
}

func CollectRepo(path string, doFetch bool) RepoInfo {
	info := RepoInfo{
		Path:    path,
		Name:    filepath.Base(path),
		Branch:  "?",
		LastRel: "?",
	}

	if doFetch {
		RunCmd([]string{"git", "fetch", "--quiet", "--prune", "--recurse-submodules"}, path, 30*time.Second)
	}

	out, _, rc := RunCmd([]string{"git", "symbolic-ref", "--short", "HEAD"}, path, 10*time.Second)
	if rc == 0 && out != "" {
		info.Branch = out
	} else {
		out, _, _ = RunCmd([]string{"git", "rev-parse", "--short", "HEAD"}, path, 10*time.Second)
		if out != "" {
			info.Branch = "(detached:" + out + ")"
		}
	}

	out, _, rc = RunCmd([]string{"git", "rev-list", "--left-right", "--count", "HEAD...@{upstream}"}, path, 10*time.Second)
	if rc == 0 && out != "" {
		if parts := strings.Fields(out); len(parts) == 2 {
			info.Ahead, _ = strconv.Atoi(parts[0])
			info.Behind, _ = strconv.Atoi(parts[1])
		}
	} else {
		info.NoUpstream = true
	}

	out, _, _ = RunCmd([]string{"git", "status", "--porcelain"}, path, 10*time.Second)
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		xy := line[:2]
		if xy == "??" {
			info.Untracked++
		} else {
			if xy[0] != ' ' && xy[0] != '?' {
				info.Staged++
			}
			if xy[1] != ' ' && xy[1] != '?' {
				info.Modified++
			}
		}
	}

	out, _, _ = RunCmd([]string{"git", "stash", "list"}, path, 10*time.Second)
	if out != "" {
		info.StashCount = len(strings.Split(out, "\n"))
	}

	out, _, _ = RunCmd([]string{"git", "log", "-1", "--format=%ct%x00%cr%x00%s"}, path, 10*time.Second)
	if out != "" {
		if parts := strings.SplitN(out, "\x00", 3); len(parts) == 3 {
			if ts, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
				info.LastTS = ts
			}
			info.LastRel = parts[1]
			info.LastMsg = parts[2]
		}
	}

	out, _, _ = RunCmd([]string{"git", "config", "--get", "remote.origin.url"}, path, 5*time.Second)
	info.RemoteURL = out

	return info
}

// RecentCommits returns the last n commit lines as "hash  time  subject".
func RecentCommits(path string, n int) []string {
	out, _, _ := RunCmd(
		[]string{"git", "log", fmt.Sprintf("-%d", n), "--format=%h  %cr  %s"},
		path, 10*time.Second,
	)
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// CommitsBehind returns up to n commits that are in the upstream but not in HEAD.
func CommitsBehind(path string, n int) []string {
	out, _, _ := RunCmd(
		[]string{"git", "log", fmt.Sprintf("-%d", n), "--format=%h  %cr  %s", "HEAD..@{upstream}"},
		path, 10*time.Second,
	)
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}
