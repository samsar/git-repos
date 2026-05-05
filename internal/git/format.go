package git

import (
	"fmt"
	"strings"
)

// Shared column widths used by both the TUI and plain renderers.
// wBranch and wSync intentionally differ between renderers: the TUI adapts to
// terminal width and can afford wider columns; plain uses fixed-width output.
const (
	ColWidthStatus  = 2
	ColWidthRepo    = 25
	ColWidthChanges = 14
	ColWidthStash   = 5
	ColWidthWhen    = 12
	ColWidthPR      = 6
)

// SyncStr formats the upstream sync state of a repo for single-column display.
func SyncStr(r RepoInfo) string {
	if r.NoUpstream {
		return "no-remote"
	}
	if r.Ahead == 0 && r.Behind == 0 {
		return "✓"
	}
	var b strings.Builder
	if r.Ahead > 0 {
		fmt.Fprintf(&b, "↑%d", r.Ahead)
	}
	if r.Behind > 0 {
		fmt.Fprintf(&b, "↓%d", r.Behind)
	}
	return b.String()
}

// ChangesStr formats the working-tree change counts for single-column display.
func ChangesStr(r RepoInfo) string {
	if r.Staged == 0 && r.Modified == 0 && r.Untracked == 0 {
		return "clean"
	}
	var parts []string
	if r.Staged > 0 {
		parts = append(parts, fmt.Sprintf("S:%d", r.Staged))
	}
	if r.Modified > 0 {
		parts = append(parts, fmt.Sprintf("M:%d", r.Modified))
	}
	if r.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("?:%d", r.Untracked))
	}
	return strings.Join(parts, " ")
}
