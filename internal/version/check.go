package version

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Repo     = "samsar/git-repos"
	CacheTTL = 24 * time.Hour
	// fetchTimeout bounds the release check: a gh that hangs on the network
	// must not leave the TUI with a command waiting for ever.
	fetchTimeout = 5 * time.Second
)

type cacheEntry struct {
	Version   string    `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
}

func cachePath() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "git-repos", "latest-version.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "git-repos", "latest-version.json")
}

func readCache() (string, bool) {
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return "", false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return "", false
	}
	if time.Since(entry.CheckedAt) > CacheTTL {
		return "", false
	}
	return entry.Version, true
}

func writeCache(version string) {
	p := cachePath()
	dir := filepath.Dir(p)
	_ = os.MkdirAll(dir, 0o700)
	if info, err := os.Lstat(dir); err == nil && info.IsDir() && info.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(dir, 0o700)
	}
	data, _ := json.Marshal(cacheEntry{Version: version, CheckedAt: time.Now()})
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	_ = f.Chmod(0o600) // tighten a cache written before
	_, _ = f.Write(data)
	_ = f.Close()
}

// FetchLatest returns the latest release tag from GitHub.
// It uses the cache if fresh, otherwise calls gh CLI.
func FetchLatest() string {
	return fetchLatest(fetchTimeout)
}

func fetchLatest(timeout time.Duration) string {
	if v, ok := readCache(); ok {
		return v
	}
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ghPath, "release", "list",
		"--repo", Repo, "--limit", "1", "--json", "tagName", "-q", ".[0].tagName")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	if v != "" {
		writeCache(v)
	}
	return v
}

// IsNewer returns true if latest is a higher semver than current.
func IsNewer(current, latest string) bool {
	cur := parseSemver(current)
	lat := parseSemver(latest)
	if cur == nil || lat == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if lat[i] > cur[i] {
			return true
		}
		if lat[i] < cur[i] {
			return false
		}
	}
	return false
}

func parseSemver(s string) []int {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}
