package git

import (
	"strings"
	"testing"
	"time"
)

// answer is the query's answer, as gh api graphql prints it: one PR of
// yours from a fork, one with nothing but its number (a ghost's, with no
// checks, no review asked, and its fork gone), and a search hit that
// isn't a PR; one asking for your review.
const answer = `{"data":{
  "mine":{"issueCount":3,"nodes":[
    {"number":42,"title":"fix token refresh","url":"https://github.com/acme/api/pull/42","isDraft":false,
     "reviewDecision":"CHANGES_REQUESTED","headRefName":"feature/auth","updatedAt":"2026-10-15T10:00:00Z",
     "author":{"login":"me"},"repository":{"nameWithOwner":"acme/api"},"headRepository":{"nameWithOwner":"me/api"},
     "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]}},
    {"number":7,"title":"wip","url":"https://github.com/me/side/pull/7","isDraft":true,
     "reviewDecision":null,"headRefName":"wip","updatedAt":"2026-10-01T10:00:00Z",
     "author":null,"repository":{"nameWithOwner":"me/side"},"headRepository":null,
     "commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]}},
    {}
  ]},
  "review":{"issueCount":1,"nodes":[
    {"number":38,"title":"add skeleton","url":"https://github.com/acme/web/pull/38","isDraft":false,
     "reviewDecision":"REVIEW_REQUIRED","headRefName":"feature/dash","updatedAt":"2026-10-14T10:00:00Z",
     "author":{"login":"pat"},"repository":{"nameWithOwner":"acme/web"},"headRepository":{"nameWithOwner":"acme/web"},
     "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"PENDING"}}}]}}
  ]}
}}`

func TestParsePRs(t *testing.T) {
	prs, err := parsePRs([]byte(answer))
	if err != nil {
		t.Fatal(err)
	}
	if len(prs.Mine) != 2 || len(prs.ReviewRequested) != 1 || prs.MineTotal != 3 || prs.ReviewTotal != 1 {
		t.Fatalf("parsed %+v", prs)
	}
	got := prs.Mine[0]
	want := PullRequest{Repo: "acme/api", HeadRepo: "me/api", Number: 42, Title: "fix token refresh",
		URL: "https://github.com/acme/api/pull/42", Branch: "feature/auth", Author: "me",
		Review: "CHANGES_REQUESTED", Checks: "FAILURE", UpdatedAt: time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)}
	if got != want {
		t.Errorf("mine[0]\n %+v\nwant\n %+v", got, want)
	}
	if bare := prs.Mine[1]; !bare.Draft || bare.Author != "" || bare.HeadRepo != "" || bare.Checks != "" || bare.Review != "" {
		t.Errorf("mine[1] %+v", bare)
	}
	if r := prs.ReviewRequested[0]; r.Author != "pat" || r.Checks != "PENDING" {
		t.Errorf("review[0] %+v", r)
	}
}

// An answer with errors and data is read for its data; with errors
// alone, the first is the error.
func TestParsePRsErrors(t *testing.T) {
	if _, err := parsePRs([]byte(`{"errors":[{"message":"Bad credentials"}]}`)); err == nil || err.Error() != "Bad credentials" {
		t.Errorf("err %v", err)
	}
	partial := `{"data":{"mine":{"issueCount":0,"nodes":[]},"review":{"issueCount":0,"nodes":[]}},"errors":[{"message":"a repo is gone"}]}`
	if _, err := parsePRs([]byte(partial)); err != nil {
		t.Errorf("partial data refused: %v", err)
	}
	if _, err := parsePRs([]byte(`not json`)); err == nil || !strings.Contains(err.Error(), "reading GitHub's answer") {
		t.Errorf("err %v", err)
	}
}

// A PR matches the repo with its branch checked out that has, as any
// remote, the repo it merges into or the fork it comes from; yours or
// one asking for your review; the lowest numbered of several. The rest
// lose a PR they had.
func TestMatchPRs(t *testing.T) {
	prs, _ := parsePRs([]byte(answer))
	prs.Mine = append(prs.Mine, PullRequest{Repo: "acme/api", Number: 50, Branch: "feature/auth"})
	repos := []RepoInfo{
		{Name: "api", Branch: "feature/auth", RemoteURL: "git@github.com:me/api.git",
			Remotes: []Remote{{Name: "origin", URL: "git@github.com:me/api.git"}, {Name: "upstream", URL: "https://github.com/acme/api"}}},
		{Name: "web", Branch: "feature/dash", RemoteURL: "https://github.com/acme/web.git"},
		{Name: "side", Branch: "main", RemoteURL: "git@github.com:me/side.git", PRNumber: 3, PRUrl: "old"},
		{Name: "other", Branch: "feature/auth", RemoteURL: "git@gitlab.com:acme/api.git"},
	}
	MatchPRs(repos, prs)
	want := []int{42, 38, 0, 0}
	for i, r := range repos {
		if r.PRNumber != want[i] || (r.PRNumber > 0) != (r.PRUrl != "") {
			t.Errorf("%s: #%d %q, want #%d", r.Name, r.PRNumber, r.PRUrl, want[i])
		}
	}
}

// A PR's local clone is one with its repo as a remote, the one with its
// branch checked out first.
func TestLocalFor(t *testing.T) {
	pr := PullRequest{Repo: "acme/api", HeadRepo: "me/api", Branch: "feature/auth"}
	repos := []RepoInfo{
		{Name: "web", RemoteURL: "git@github.com:acme/web.git", Branch: "feature/auth"},
		{Name: "api-main", RemoteURL: "git@github.com:acme/api.git", Branch: "main"},
		{Name: "api-fork", RemoteURL: "git@github.com:me/api.git", Branch: "feature/auth"},
	}
	if i, ok := LocalFor(pr, repos); !ok || i != 2 {
		t.Errorf("LocalFor = %d, %v; want the fork with the branch", i, ok)
	}
	if i, ok := LocalFor(pr, repos[:2]); !ok || i != 1 {
		t.Errorf("LocalFor = %d, %v; want the clone on main", i, ok)
	}
	if _, ok := LocalFor(pr, repos[:1]); ok {
		t.Error("found a clone of another repo")
	}
}
