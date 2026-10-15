package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

// PullRequest is an open pull request: one of yours, or one asking for
// your review.
type PullRequest struct {
	// Repo is the repo it merges into, as owner/name; HeadRepo the one its
	// branch is in, a fork's for a PR from one ("" once that is gone).
	Repo     string `json:"repo"`
	HeadRepo string `json:"head_repo,omitempty"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Branch   string `json:"branch"`
	Author   string `json:"author,omitempty"`
	Draft    bool   `json:"draft,omitempty"`
	// Review is GitHub's review decision: APPROVED, CHANGES_REQUESTED or
	// REVIEW_REQUIRED; "" where the repo asks for no review.
	Review string `json:"review,omitempty"`
	// Checks is the state of the checks on its last commit: SUCCESS,
	// FAILURE, ERROR, PENDING or EXPECTED; "" for none.
	Checks    string    `json:"checks,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PRs is your open PRs and those asking for your review, the most
// recently updated first, at most prLimit of each, with how many there
// are of each in all.
type PRs struct {
	Mine            []PullRequest `json:"mine"`
	ReviewRequested []PullRequest `json:"review_requested"`
	MineTotal       int           `json:"mine_total"`
	ReviewTotal     int           `json:"review_total"`
	// Warning is the first of the errors GitHub answered beside its data,
	// about PRs it left out: those of an org whose SSO your gh token isn't
	// authorized for, say. "" for none.
	Warning string `json:"warning,omitempty"`
}

// prLimit is how many of each FetchPRs asks for: one page.
const prLimit = 100

// prQuery finds both lists in one GraphQL request, with what the PR page
// and the PR column need of each: the branch to match a checkout by, its
// review decision and its checks.
const prQuery = `query($mine: String!, $review: String!, $limit: Int!) {
  mine: search(query: $mine, type: ISSUE, first: $limit) { issueCount nodes { ...pr } }
  review: search(query: $review, type: ISSUE, first: $limit) { issueCount nodes { ...pr } }
}
fragment pr on PullRequest {
  number title url isDraft reviewDecision headRefName updatedAt
  author { login }
  repository { nameWithOwner }
  headRepository { nameWithOwner }
  commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
}`

// prTimeout bounds the PR lookup.
const prTimeout = 30 * time.Second

// FetchPRs asks GitHub, through gh, for your open PRs and those asking for
// your review, in one request.
func FetchPRs() (PRs, error) {
	ctx, cancel := context.WithTimeout(context.Background(), prTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api", "graphql",
		"-f", "query="+prQuery,
		"-f", "mine=is:pr is:open archived:false sort:updated-desc author:@me",
		"-f", "review=is:pr is:open archived:false sort:updated-desc review-requested:@me",
		"-F", "limit="+strconv.Itoa(prLimit))
	cmd.WaitDelay = time.Second // a child of gh's holding its output open
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return PRs{}, errors.New("gh isn't installed: see https://cli.github.com")
	case ctx.Err() != nil:
		return PRs{}, fmt.Errorf("gh didn't answer in %s", prTimeout)
	case err != nil && stdout.Len() == 0:
		msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		if msg == "" {
			msg = err.Error()
		}
		return PRs{}, fmt.Errorf("gh api graphql: %s", msg)
	}
	// gh exits non-zero on errors GitHub answers beside its data, which
	// parsePRs reads as a warning.
	return parsePRs(stdout.Bytes())
}

// prNode is a PullRequest as the query returns it.
type prNode struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	ReviewDecision string    `json:"reviewDecision"`
	HeadRefName    string    `json:"headRefName"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Author         *struct {
		Login string `json:"login"`
	} `json:"author"`
	Repository *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	HeadRepository *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"headRepository"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type prSearch struct {
	IssueCount int      `json:"issueCount"`
	Nodes      []prNode `json:"nodes"`
}

// parsePRs reads the query's answer: its data, even beside errors about
// part of it; an error when it has none.
func parsePRs(data []byte) (PRs, error) {
	var resp struct {
		Data *struct {
			Mine   prSearch `json:"mine"`
			Review prSearch `json:"review"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return PRs{}, fmt.Errorf("reading GitHub's answer: %w", err)
	}
	if resp.Data == nil {
		if len(resp.Errors) > 0 {
			return PRs{}, errors.New(resp.Errors[0].Message)
		}
		return PRs{}, errors.New("GitHub answered with no data")
	}
	prs := PRs{
		Mine:            pullRequests(resp.Data.Mine.Nodes),
		ReviewRequested: pullRequests(resp.Data.Review.Nodes),
		MineTotal:       resp.Data.Mine.IssueCount,
		ReviewTotal:     resp.Data.Review.IssueCount,
	}
	if len(resp.Errors) > 0 {
		prs.Warning = resp.Errors[0].Message
		if n := len(resp.Errors) - 1; n > 0 {
			prs.Warning += fmt.Sprintf(" (and %d more)", n)
		}
	}
	return prs, nil
}

func pullRequests(nodes []prNode) []PullRequest {
	out := make([]PullRequest, 0, len(nodes))
	for _, n := range nodes {
		if n.Number == 0 {
			continue // a search hit that isn't a PR
		}
		pr := PullRequest{
			Number:    n.Number,
			Title:     n.Title,
			URL:       n.URL,
			Branch:    n.HeadRefName,
			Draft:     n.IsDraft,
			Review:    n.ReviewDecision,
			UpdatedAt: n.UpdatedAt,
		}
		if n.Author != nil {
			pr.Author = n.Author.Login
		}
		if n.Repository != nil {
			pr.Repo = n.Repository.NameWithOwner
		}
		if n.HeadRepository != nil {
			pr.HeadRepo = n.HeadRepository.NameWithOwner
		}
		if c := n.Commits.Nodes; len(c) > 0 && c[0].Commit.StatusCheckRollup != nil {
			pr.Checks = c[0].Commit.StatusCheckRollup.State
		}
		out = append(out, pr)
	}
	return out
}

// repoKeys is every GitHub repo, as owner/name, that r has as a remote.
func repoKeys(r RepoInfo) []string {
	var keys []string
	add := func(url string) {
		if k := normalizeRepoKey(url); k != "" && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	add(r.RemoteURL)
	for _, rem := range r.Remotes {
		add(rem.URL)
	}
	return keys
}

// OnPRBranch reports whether r has pr's branch checked out: a branch of
// its name, in a clone with the repo that branch is in as a remote, the
// fork's for a PR from one. A branch of the same name in a clone of the
// repo it merges into alone (your own main, beside a fork's main) isn't.
func OnPRBranch(r RepoInfo, pr PullRequest) bool {
	return r.Branch == pr.Branch && slices.Contains(repoKeys(r), headRepo(pr))
}

// headRepo is the repo pr's branch is in: its fork's, for a PR from one;
// the repo it merges into otherwise, or when the fork is gone.
func headRepo(pr PullRequest) string {
	if pr.HeadRepo != "" {
		return pr.HeadRepo
	}
	return pr.Repo
}

// ForRepo is the PR of prs whose branch r has checked out (OnPRBranch);
// the lowest numbered of several. False for none.
func ForRepo(r RepoInfo, prs []PullRequest) (PullRequest, bool) {
	var best PullRequest
	found := false
	for _, pr := range prs {
		if !OnPRBranch(r, pr) {
			continue
		}
		if !found || pr.Number < best.Number {
			best, found = pr, true
		}
	}
	return best, found
}

// MatchPRs sets each repo's PR to the open one, yours or asking for your
// review, of the branch it has checked out; none for the others.
func MatchPRs(repos []RepoInfo, prs PRs) {
	all := append(append([]PullRequest{}, prs.Mine...), prs.ReviewRequested...)
	for i := range repos {
		repos[i].PRNumber, repos[i].PRUrl, repos[i].PRState = 0, "", ""
		if pr, ok := ForRepo(repos[i], all); ok {
			repos[i].PRNumber, repos[i].PRUrl, repos[i].PRState = pr.Number, pr.URL, "OPEN"
		}
	}
}

// FetchAndMatchPRs looks up your open PRs and those asking for your
// review, and matches them to repos (MatchPRs). On an error the repos are
// left as they were.
func FetchAndMatchPRs(repos []RepoInfo) (PRs, error) {
	prs, err := FetchPRs()
	if err != nil {
		return PRs{}, err
	}
	MatchPRs(repos, prs)
	return prs, nil
}

// LocalFor is the index in repos of the clone pr's code is in: a repo
// with the repo it merges into, or comes from, as a remote; one with its
// branch checked out (OnPRBranch) first. False for none.
func LocalFor(pr PullRequest, repos []RepoInfo) (int, bool) {
	found := -1
	for i, r := range repos {
		keys := repoKeys(r)
		if !slices.Contains(keys, pr.Repo) && !slices.Contains(keys, pr.HeadRepo) {
			continue
		}
		if OnPRBranch(r, pr) {
			return i, true
		}
		if found < 0 {
			found = i
		}
	}
	return found, found >= 0
}
