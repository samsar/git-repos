package tui

import (
	"testing"

	"github.com/samsar/git-repos/internal/git"
)

// A long branch name should display in full in the list view when the terminal
// has room, borrowing from LAST MSG rather than being clipped with an ellipsis.
func TestColWidths_LongBranchFitsByBorrowingFromMsg(t *testing.T) {
	const longBranch = "feat/otel-instrument-and-repoint" // 32 cols
	m := model{
		width: 123, // narrow enough that the old code clipped the branch
		repos: []git.RepoInfo{repo("workspace-management-api", longBranch, "some commit")},
	}

	branch, _, msg := m.colWidths()

	if branch < len(longBranch) {
		t.Errorf("branch column = %d, want >= %d so %q shows in full", branch, len(longBranch), longBranch)
	}
	if msg < 6 {
		t.Errorf("LAST MSG shrank to %d, below the floor of 6", msg)
	}
}

// When there is plenty of width, BRANCH gets its preferred width and LAST MSG
// keeps the remainder (no borrowing needed).
func TestColWidths_WideTerminalKeepsRoomyMsg(t *testing.T) {
	const longBranch = "feat/otel-instrument-and-repoint"
	m := model{
		width: 200,
		repos: []git.RepoInfo{repo("workspace-management-api", longBranch, "some commit")},
	}

	branch, _, msg := m.colWidths()

	if branch < len(longBranch) {
		t.Errorf("branch column = %d, want >= %d", branch, len(longBranch))
	}
	if msg <= 12 {
		t.Errorf("LAST MSG = %d, expected it to grow well past its minimum on a wide terminal", msg)
	}
}
