package tui

import (
	"strings"
	"testing"

	"github.com/samsar/git-repos/internal/git"
)

// helpers

func repo(name, branch, lastMsg string) git.RepoInfo {
	return git.RepoInfo{
		Path:    "/repos/" + name,
		Name:    name,
		Branch:  branch,
		LastMsg: lastMsg,
	}
}

func modelWith(repos []git.RepoInfo, query string) model {
	return model{repos: repos, searchQuery: query}
}

// ── repoMatchesSearch ─────────────────────────────────────────────────────────

func TestRepoMatchesSearch_Name(t *testing.T) {
	r := repo("my-api", "main", "fix bug")
	if !repoMatchesSearch(r, "api") {
		t.Error("expected match on name")
	}
}

func TestRepoMatchesSearch_Branch(t *testing.T) {
	r := repo("svc", "feature/login", "wip")
	if !repoMatchesSearch(r, "login") {
		t.Error("expected match on branch")
	}
}

func TestRepoMatchesSearch_LastMsg(t *testing.T) {
	r := repo("svc", "main", "refactor auth middleware")
	if !repoMatchesSearch(r, "auth") {
		t.Error("expected match on lastMsg")
	}
}

func TestRepoMatchesSearch_CaseInsensitive(t *testing.T) {
	// repoMatchesSearch receives a pre-lowercased query (caller's responsibility,
	// matching how displayRepos uses it). The repo field values may be any case.
	r := repo("MyRepo", "Main", "Fix Bug")
	for _, raw := range []string{"myrepo", "MYREPO", "main", "fix bug", "FIX"} {
		if !repoMatchesSearch(r, strings.ToLower(raw)) {
			t.Errorf("expected case-insensitive match for query %q", raw)
		}
	}
}

func TestRepoMatchesSearch_NoMatch(t *testing.T) {
	r := repo("alpha", "main", "initial commit")
	if repoMatchesSearch(r, "zzz") {
		t.Error("expected no match")
	}
}

func TestRepoMatchesSearch_EmptyQuery(t *testing.T) {
	r := repo("alpha", "main", "initial commit")
	if !repoMatchesSearch(r, "") {
		t.Error("empty query should match everything")
	}
}

// ── displayRepos ──────────────────────────────────────────────────────────────

func TestDisplayRepos_NoQuery_ReturnsSameSlice(t *testing.T) {
	repos := []git.RepoInfo{repo("a", "main", ""), repo("b", "main", "")}
	m := modelWith(repos, "")
	got := m.displayRepos()
	if &got[0] != &m.repos[0] {
		t.Error("expected the same backing slice when query is empty")
	}
}

func TestDisplayRepos_FiltersCorrectly(t *testing.T) {
	repos := []git.RepoInfo{
		repo("alpha", "main", ""),
		repo("beta", "main", ""),
		repo("gamma", "main", ""),
	}
	m := modelWith(repos, "alp")
	got := m.displayRepos()
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Errorf("expected [alpha], got %v", got)
	}
}

func TestDisplayRepos_NoMatches(t *testing.T) {
	repos := []git.RepoInfo{repo("alpha", "main", ""), repo("beta", "main", "")}
	m := modelWith(repos, "zzz")
	got := m.displayRepos()
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestDisplayRepos_AllMatch(t *testing.T) {
	repos := []git.RepoInfo{
		repo("svc-api", "main", ""),
		repo("svc-worker", "main", ""),
	}
	m := modelWith(repos, "svc")
	got := m.displayRepos()
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}
}

// ── cursor translation ────────────────────────────────────────────────────────

// repos: [apple(0), banana(1), cherry(2), apricot(3)]
// query "ap" matches apple(0) and apricot(3) only.
// filtered view: [apple(0→full 0), apricot(1→full 3)]

func makeRepos() []git.RepoInfo {
	return []git.RepoInfo{
		repo("apple", "main", ""),
		repo("banana", "main", ""),
		repo("cherry", "main", ""),
		repo("apricot", "main", ""),
	}
}

func TestFilteredToFullIdx_NoQuery(t *testing.T) {
	m := modelWith(makeRepos(), "")
	for i := range makeRepos() {
		if got := m.filteredToFullIdx(i); got != i {
			t.Errorf("filteredToFullIdx(%d) = %d, want %d (no query)", i, got, i)
		}
	}
}

func TestFilteredToFullIdx_WithQuery(t *testing.T) {
	// "ap" matches apple(full 0) and apricot(full 3)
	m := modelWith(makeRepos(), "ap")
	if got := m.filteredToFullIdx(0); got != 0 {
		t.Errorf("filteredToFullIdx(0) = %d, want 0 (apple)", got)
	}
	if got := m.filteredToFullIdx(1); got != 3 {
		t.Errorf("filteredToFullIdx(1) = %d, want 3 (apricot)", got)
	}
}

func TestFullToFilteredIdx_NoQuery(t *testing.T) {
	m := modelWith(makeRepos(), "")
	for i := range makeRepos() {
		if got := m.fullToFilteredIdx(i); got != i {
			t.Errorf("fullToFilteredIdx(%d) = %d, want %d (no query)", i, got, i)
		}
	}
}

func TestFullToFilteredIdx_WithQuery(t *testing.T) {
	// "ap" matches apple(full 0) and apricot(full 3)
	m := modelWith(makeRepos(), "ap")
	if got := m.fullToFilteredIdx(0); got != 0 {
		t.Errorf("fullToFilteredIdx(0) = %d, want 0 (apple → filtered 0)", got)
	}
	if got := m.fullToFilteredIdx(3); got != 1 {
		t.Errorf("fullToFilteredIdx(3) = %d, want 1 (apricot → filtered 1)", got)
	}
}

func TestCursorTranslation_RoundTrip(t *testing.T) {
	// For each matching repo, filtered→full→filtered should be identity.
	m := modelWith(makeRepos(), "ap") // matches apple(0), apricot(3)
	for filteredIdx := range 2 {
		full := m.filteredToFullIdx(filteredIdx)
		back := m.fullToFilteredIdx(full)
		if back != filteredIdx {
			t.Errorf("round-trip failed: filtered %d → full %d → filtered %d", filteredIdx, full, back)
		}
	}
}

func TestFullToFilteredIdx_RepoNotInFilter_ReturnsZero(t *testing.T) {
	// banana (full index 1) is not matched by "ap"; should fall back to 0.
	m := modelWith(makeRepos(), "ap")
	if got := m.fullToFilteredIdx(1); got != 0 {
		t.Errorf("fullToFilteredIdx(1) = %d, want 0 (banana not in filter)", got)
	}
}
