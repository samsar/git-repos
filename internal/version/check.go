package version

import (
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
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	data, _ := json.Marshal(cacheEntry{Version: version, CheckedAt: time.Now()})
	_ = os.WriteFile(p, data, 0o644)
}

// FetchLatest returns the latest release tag from GitHub.
// It uses the cache if fresh, otherwise calls gh CLI.
func FetchLatest() string {
	if v, ok := readCache(); ok {
		return v
	}
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return ""
	}
	out, err := exec.Command(ghPath, "release", "list",
		"--repo", Repo, "--limit", "1", "--json", "tagName", "-q", ".[0].tagName").Output()
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
