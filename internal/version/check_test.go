package version

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestWriteCacheIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows doesn't model Unix permission bits")
	}
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	p := filepath.Join(dir, "git-repos", "latest-version.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeCache("v1.2.3")

	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("cache mode %v, want 0600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("cache directory mode %v, want 0700", got)
	}
}

// A gh that hangs must not keep the check waiting past its deadline.
func TestFetchLatestTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell for the fake gh")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	start := time.Now()
	if got := fetchLatest(100 * time.Millisecond); got != "" {
		t.Errorf("fetchLatest = %q, want no version", got)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Errorf("fetchLatest took %v, want it bounded by the timeout", elapsed)
	}
}
