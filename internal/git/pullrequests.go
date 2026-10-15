package git

import (
	"encoding/json"
	"errors"
	"fmt"
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

// FetchPRs asks GitHub, through gh, for your open PRs and those asking for
// your review, in one request.
func FetchPRs() (PRs, error) {
	out, errOut, rc := RunCmd([]string{"gh", "api", "graphql",
		"-f", "query=" + prQuery,
		"-f", "mine=is:pr is:open archived:false sort:updated-desc author:@me",
		"-f", "review=is:pr is:open archived:false sort:updated-desc review-requested:@me",
		"-F", "limit=" + strconv.Itoa(prLimit),
	}, "", 30*time.Second)
	if rc != 0 && out == "" {
		msg, _, _ := strings.Cut(errOut, "\n")
		if msg == "" {
			msg = fmt.Sprintf("exit %d", rc)
		}
		return PRs{}, fmt.Errorf("gh api graphql: %s", msg)
	}
	return parsePRs([]byte(out))
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
	return PRs{
		Mine:            pullRequests(resp.Data.Mine.Nodes),
		ReviewRequested: pullRequests(resp.Data.Review.Nodes),
		MineTotal:       resp.Data.Mine.IssueCount,
		ReviewTotal:     resp.Data.Review.IssueCount,
	}, nil
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

// ForRepo is the PR whose branch r has checked out, of prs: one merging
// into, or from, a repo r has as a remote (a fork's PR to upstream
// too); the lowest numbered of several. False for none.
func ForRepo(r RepoInfo, prs []PullRequest) (PullRequest, bool) {
	keys := repoKeys(r)
	var best PullRequest
	found := false
	for _, pr := range prs {
		if pr.Branch != r.Branch || !(slices.Contains(keys, pr.Repo) || slices.Contains(keys, pr.HeadRepo)) {
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
// branch checked out first. False for none.
func LocalFor(pr PullRequest, repos []RepoInfo) (int, bool) {
	found := -1
	for i, r := range repos {
		keys := repoKeys(r)
		if !slices.Contains(keys, pr.Repo) && !slices.Contains(keys, pr.HeadRepo) {
			continue
		}
		if r.Branch == pr.Branch {
			return i, true
		}
		if found < 0 {
			found = i
		}
	}
	return found, found >= 0
}
