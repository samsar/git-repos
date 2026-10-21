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

	_, branch, _, msg := m.colWidths()

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

	_, branch, _, msg := m.colWidths()

	if branch < len(longBranch) {
		t.Errorf("branch column = %d, want >= %d", branch, len(longBranch))
	}
	if msg <= 12 {
		t.Errorf("LAST MSG = %d, expected it to grow well past its minimum on a wide terminal", msg)
	}
}

// A long repo name should display in full instead of being clipped at the
// old fixed REPO width, borrowing from LAST MSG when the terminal is tight.
func TestColWidths_LongRepoFitsByBorrowingFromMsg(t *testing.T) {
	const longRepo = "customer-onboarding-workflow-service" // 36 cols
	m := model{
		width: 123,
		repos: []git.RepoInfo{repo(longRepo, "main", "some commit")},
	}

	repoW, _, _, msg := m.colWidths()

	if repoW < len(longRepo) {
		t.Errorf("repo column = %d, want >= %d so %q shows in full", repoW, len(longRepo), longRepo)
	}
	if msg < 6 {
		t.Errorf("LAST MSG shrank to %d, below the floor of 6", msg)
	}
}

// Every resizable column plus the fixed ones must add up to the terminal
// width, so rows never overflow onto a second line.
func TestColWidths_FillTerminalExactly(t *testing.T) {
	m := model{
		width: 140,
		repos: []git.RepoInfo{repo("customer-onboarding-workflow-service", "feat/otel-instrument-and-repoint", "some commit")},
	}

	repoW, branch, sync, msg := m.colWidths()

	if got := 49 + repoW + branch + sync + msg; got != m.width {
		t.Errorf("columns add up to %d, want the terminal width %d", got, m.width)
	}
}
