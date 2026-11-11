package version

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
}
