package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/samsar/git-repos/internal/git"
)

// withPRs is a list of two repos, one with a PR's branch checked out,
// and PRs of yours and one asking for your review, loaded.
func withPRs(t *testing.T) model {
	t.Helper()
	m := yours(t, nil)
	api := repo("api", "feature/auth", "fix")
	api.RemoteURL = "git@github.com:acme/api.git"
	web := repo("web", "main", "init")
	web.RemoteURL = "git@github.com:acme/web.git"
	m.repos = []git.RepoInfo{web, api}
	now := time.Now()
	m.prs = git.PRs{
		Mine: []git.PullRequest{
			{Repo: "acme/api", Number: 42, Title: "fix token refresh", Branch: "feature/auth", Checks: "FAILURE", UpdatedAt: now.Add(-2 * time.Hour), URL: "https://github.com/acme/api/pull/42"},
			{Repo: "me/side", Number: 7, Title: "wip parser", Branch: "wip", Draft: true, UpdatedAt: now.Add(-72 * time.Hour)},
		},
		ReviewRequested: []git.PullRequest{
			{Repo: "acme/web", Number: 38, Title: "add skeleton", Branch: "feature/dash", Author: "pat", Review: "REVIEW_REQUIRED", Checks: "PENDING", UpdatedAt: now.Add(-24 * time.Hour)},
		},
		MineTotal: 2, ReviewTotal: 1,
	}
	m.prsEverLoaded = true
	return m
}

// prPage is the PR page's text.
func prPage(m model) string { return plain(m.View().Content) }

// tab goes to the PR page and back; both sections show at once, each PR
// with its checks, review and whether it is cloned here.
func TestPRPage(t *testing.T) {
	m := press(withPRs(t), "tab")
	if m.state != statePRs {
		t.Fatal("tab doesn't open the PR page")
	}
	page := prPage(m)
	for _, want := range []string{"YOURS · 2", "REVIEW REQUESTED · 1", "#42", "fix token refresh", "✗ fail", "✓ here",
		"#7", "draft", "#38", "pat", "● run", "required", "cloned", "Yours: 2", "Review requested: 1", "Showing: all"} {
		if !strings.Contains(page, want) {
			t.Errorf("the PR page doesn't say %q", want)
		}
	}
	if m.windowTitle() != "git-repos: PRs" {
		t.Errorf("title %q", m.windowTitle())
	}
	if press(m, "tab").state != stateList {
		t.Error("tab doesn't go back to the repos")
	}
}

// f cycles through yours, those asking for your review, and all.
func TestPRFilter(t *testing.T) {
	m := press(press(withPRs(t), "tab"), "f")
	if page := prPage(m); !strings.Contains(page, "#42") || strings.Contains(page, "#38") || !strings.Contains(page, "Showing: yours") {
		t.Errorf("yours:\n%s", page)
	}
	m = press(m, "f")
	if page := prPage(m); strings.Contains(page, "#42") || !strings.Contains(page, "#38") {
		t.Errorf("review requested:\n%s", page)
	}
	if m = press(m, "f"); m.prFilter != prsAll {
		t.Error("f doesn't come back to all")
	}
}

// / searches the PRs as it does the repos; esc clears it.
func TestPRSearch(t *testing.T) {
	m := press(press(withPRs(t), "tab"), "/")
	for _, k := range []string{"p", "a", "t"} {
		m = press(m, k)
	}
	m = press(m, "enter")
	if prs := m.displayPRs(); len(prs) != 1 || prs[0].Number != 38 {
		t.Errorf("searching pat: %v", prs)
	}
	if page := prPage(m); !strings.Contains(page, "YOURS · 0 of 2 match") || !strings.Contains(page, "none") {
		t.Errorf("an empty section:\n%s", page)
	}
	if m = press(m, "esc"); m.searchQuery != "" || m.state != statePRs {
		t.Error("esc doesn't clear the search, staying on the PR page")
	}
	if m = press(m, "esc"); m.state != stateList {
		t.Error("esc again doesn't go back to the repos")
	}
}

// Enter opens the PR's local clone in the detail view, which goes back
// to the PR page, the repos' cursor where it was; a PR not cloned here
// says so.
func TestPROpensLocalRepo(t *testing.T) {
	m := withPRs(t)
	m.cursor = 0
	m = press(press(m, "tab"), "enter")
	if m.state != stateDetail || m.repos[m.cursor].Name != "api" {
		t.Fatalf("state %d on %s", m.state, m.repos[m.cursor].Name)
	}
	m = press(m, "esc")
	if m.state != statePRs || m.cursor != 0 {
		t.Errorf("back to state %d, cursor %d", m.state, m.cursor)
	}
	m = press(press(m, "j"), "enter")
	if m.state != statePRs || !strings.Contains(m.statusMsg, "me/side isn't cloned") {
		t.Errorf("state %d, status %q", m.state, m.statusMsg)
	}
}

// The page says why it has no PRs: lookup off, gh missing, loading, or
// the lookup's error.
func TestPRPageEmpty(t *testing.T) {
	tests := []struct {
		set  func(*model)
		want string
	}{
		{func(m *model) { m.noPRs = true }, "PR lookup is off"},
		{func(m *model) { m.ghUnavailable = true }, "install the gh CLI"},
		{func(m *model) { m.prsEverLoaded = false }, "loading PRs"},
		{func(m *model) { m.prs, m.prsErr = git.PRs{}, errors.New("gh api graphql: HTTP 401") }, "couldn't load PRs: gh api graphql: HTTP 401"},
	}
	for _, tt := range tests {
		m := press(withPRs(t), "tab")
		tt.set(&m)
		if page := prPage(m); !strings.Contains(page, tt.want) {
			t.Errorf("doesn't say %q:\n%s", tt.want, page)
		}
	}
}

func TestPRGroup(t *testing.T) {
	tests := []struct {
		pr   git.PullRequest
		want git.Group
	}{
		{git.PullRequest{Draft: true, Checks: "FAILURE"}, git.GroupStale},
		{git.PullRequest{Checks: "ERROR"}, git.GroupAttention},
		{git.PullRequest{Checks: "SUCCESS", Review: "CHANGES_REQUESTED"}, git.GroupAttention},
		{git.PullRequest{Checks: "PENDING", Review: "APPROVED"}, git.GroupPush},
		{git.PullRequest{Checks: "SUCCESS", Review: "REVIEW_REQUIRED"}, git.GroupPush},
		{git.PullRequest{Checks: "SUCCESS", Review: "APPROVED"}, git.GroupOK},
		{git.PullRequest{}, git.GroupOK},
	}
	for _, tt := range tests {
		if got := prGroup(tt.pr); got != tt.want {
			t.Errorf("prGroup(%+v) = %d, want %d", tt.pr, got, tt.want)
		}
	}
}

func TestRelTime(t *testing.T) {
	now := time.Now()
	for d, want := range map[time.Duration]string{
		10 * time.Second: "just now", 5 * time.Minute: "5m ago", 3 * time.Hour: "3h ago",
		50 * time.Hour: "2d ago", 20 * 24 * time.Hour: "2w ago", 90 * 24 * time.Hour: "3mo ago", 800 * 24 * time.Hour: "2y ago",
	} {
		if got := relTime(now.Add(-d), now); got != want {
			t.Errorf("relTime(-%v) = %q, want %q", d, got, want)
		}
	}
}

// The PR page's search is its own: a search there, then a PR's detail
// view, the settings and back, leaves the repos unsearched and every key
// safe, the settings going back to the detail view they were opened from.
func TestPRSearchStaysOnThePRPage(t *testing.T) {
	m := press(withPRs(t), "tab")
	m = press(m, "/")
	for _, k := range []string{"f", "i", "x"} {
		m = press(m, k)
	}
	m = press(press(m, "enter"), "enter")
	if m.state != stateDetail {
		t.Fatalf("state %d, want the detail view", m.state)
	}
	m = press(press(m, "z"), "esc")
	if m.state != stateDetail {
		t.Fatalf("settings went back to state %d, not the detail view", m.state)
	}
	m = press(press(m, "esc"), "tab")
	if m.state != stateList || m.searchQuery != "" || len(m.displayRepos()) != 2 {
		t.Fatalf("the repos: state %d, search %q, %d shown", m.state, m.searchQuery, len(m.displayRepos()))
	}
	for _, k := range []string{"v", "p", "s", "e"} {
		press(m, k) // none may panic
	}
	if m = press(press(m, "tab"), "tab"); m.prQuery != "fix" {
		t.Errorf("the PR page's search is %q after going round, want fix", m.prQuery)
	}
}

// The repos' cursor, kept while a PR's detail view is open, stays on a
// repo when repos go meanwhile; and the list's keys keep it on one.
func TestListCursorStaysOnARepo(t *testing.T) {
	m := withPRs(t)
	m.cursor = 1
	m = press(press(m, "tab"), "enter")
	m.repos = m.repos[:1] // one went while the detail view was open
	m.cursor = 0
	m = press(press(m, "esc"), "tab")
	if m.cursor != 0 {
		t.Errorf("cursor %d with 1 repo", m.cursor)
	}
	press(m, "enter") // may not panic

	m = withPRs(t)
	m.cursor = 5
	for _, k := range []string{"v", "p", "s", "enter"} {
		press(m, k) // none may panic
	}
}

// A pull or refresh keeps a repo's PR, one asking for your review too,
// from the PRs already looked up.
func TestRefreshKeepsReviewRequestedPR(t *testing.T) {
	m := withPRs(t)
	web := m.repos[0]
	web.Branch = "feature/dash" // the review-requested PR's branch
	m.repos[0] = web
	m = m.matchPRs()
	fresh := web
	fresh.PRNumber, fresh.PRUrl = 0, "" // as git collects it
	next, _ := m.Update(fetchDoneMsg(fresh))
	m = next.(model)
	for _, r := range m.repos {
		if r.Name == "web" && r.PRNumber != 38 {
			t.Errorf("web's PR after a pull: #%d, want #38", r.PRNumber)
		}
	}
}

// PRs looked up again keep the PR page's cursor on a PR, and the detail
// view on its repo, however they re-sort the repos.
func TestPRsLoadedKeepCursors(t *testing.T) {
	m := press(withPRs(t), "tab")
	m = press(press(m, "j"), "j")
	if m.prCursor != 2 {
		t.Fatalf("prCursor %d", m.prCursor)
	}
	one := git.PRs{ReviewRequested: m.prs.ReviewRequested, ReviewTotal: 1}
	next, _ := m.Update(prsLoadedMsg{prs: one})
	m = next.(model)
	if pr, ok := m.selectedPR(); !ok || pr.Number != 38 {
		t.Errorf("selected %v, %v after the reload; want #38", pr.Number, ok)
	}

	m = withPRs(t)
	m.sortCol = git.SortPR
	m, _ = mustOpenDetail(m, "web")
	next, _ = m.Update(prsLoadedMsg{prs: m.prs})
	if m = next.(model); m.repos[m.cursor].Name != "web" {
		t.Errorf("the detail view shows %s after the reload, want web", m.repos[m.cursor].Name)
	}
}

// mustOpenDetail opens the detail view of the repo named name.
func mustOpenDetail(m model, name string) (model, bool) {
	for i, r := range m.repos {
		if r.Name == name {
			next, _ := m.openDetail(i)
			return next.(model), true
		}
	}
	return m, false
}
