package git

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func FetchAndMatchPRs(repos []RepoInfo) {
	reposWithPRs := fetchOpenPRs()
	matchPRs(repos, reposWithPRs)
}

// FetchAndMatchPRForRepo fetches open PRs authored by the current user for a
// single repo and updates the repo's PR fields to match its currently
// checked-out branch. It queries the repo directly with `gh pr list` rather than
// the broad cross-repo search, so it stays fast for a single-repo refresh.
func FetchAndMatchPRForRepo(repo *RepoInfo) {
	prs := fetchPRDetails([]RepoInfo{*repo})

	repo.PRNumber = 0
	repo.PRUrl = ""
	repo.PRState = ""

	var best *prDetail
	for i := range prs {
		if prs[i].HeadRefName != repo.Branch {
			continue
		}
		if best == nil || prs[i].Number < best.Number {
			best = &prs[i]
		}
	}
	if best != nil {
		repo.PRNumber = best.Number
		repo.PRUrl = best.URL
		repo.PRState = best.State
	}
}

// fetchOpenPRs does a broad search to find which repos have open PRs authored by
// the current user. It returns only repo keys (owner/repo), not full PR details,
// because gh search prs does not include headRefName — the branch name needed to
// match a PR to the currently checked-out branch. fetchPRDetails is a second pass
// that fetches full PR data per repo.
func fetchOpenPRs() []string {
	cwd, _ := os.Getwd()
	out, _, rc := RunCmd(
		[]string{"gh", "search", "prs", "--author", "@me", "--state", "open",
			"--json", "number,url,title,state,repository", "--limit", "300"},
		cwd, 30*time.Second,
	)
	if rc != 0 || out == "" {
		return nil
	}
	var results []struct {
		Repository struct {
			Name          string `json:"name"`
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	for _, r := range results {
		key := r.Repository.NameWithOwner
		if key == "" {
			key = r.Repository.Name
		}
		if key != "" && !seen[key] {
			seen[key] = true
			names = append(names, key)
		}
	}
	return names
}

type prDetail struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	State       string `json:"state"`
	HeadRefName string `json:"headRefName"`
	repoKey     string
}

func fetchPRDetails(repos []RepoInfo) []prDetail {
	var mu sync.Mutex
	var all []prDetail
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, repo := range repos {
		wg.Add(1)
		go func(r RepoInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			out, _, rc := RunCmd(
				[]string{"gh", "pr", "list", "--author", "@me", "--state", "open",
					"--json", "number,url,title,state,headRefName", "--limit", "20"},
				r.Path, 15*time.Second,
			)
			if rc != 0 || out == "" {
				return
			}
			var prs []prDetail
			if err := json.Unmarshal([]byte(out), &prs); err != nil {
				return
			}
			name := normalizeRepoKey(r.RemoteURL)
			if name == "" {
				name = filepath.Base(r.Path)
			}
			for i := range prs {
				prs[i].repoKey = name
			}
			mu.Lock()
			all = append(all, prs...)
			mu.Unlock()
		}(repo)
	}
	wg.Wait()
	return all
}

func matchPRs(repos []RepoInfo, reposWithPRs []string) {
	repoMap := make(map[string][]int)
	for i, r := range repos {
		key := normalizeRepoKey(r.RemoteURL)
		if key == "" {
			key = r.Name
		}
		repoMap[key] = append(repoMap[key], i)
	}

	var affectedRepos []RepoInfo
	for _, key := range reposWithPRs {
		if idxs, ok := repoMap[key]; ok {
			for _, i := range idxs {
				affectedRepos = append(affectedRepos, repos[i])
			}
		}
	}
	if len(affectedRepos) == 0 {
		return
	}

	prs := fetchPRDetails(affectedRepos)

	type prKey struct{ repo, branch string }
	index := make(map[prKey]prDetail)
	for _, pr := range prs {
		k := prKey{pr.repoKey, pr.HeadRefName}
		if existing, ok := index[k]; !ok || pr.Number < existing.Number {
			index[k] = pr
		}
	}

	for i := range repos {
		repoKey := normalizeRepoKey(repos[i].RemoteURL)
		if repoKey == "" {
			repoKey = repos[i].Name
		}
		k := prKey{repoKey, repos[i].Branch}
		if pr, ok := index[k]; ok {
			repos[i].PRNumber = pr.Number
			repos[i].PRUrl = pr.URL
			repos[i].PRState = pr.State
		}
	}
}

func normalizeRepoKey(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "ssh://")
	s = strings.TrimPrefix(s, "git@")
	s = strings.TrimPrefix(s, "git://")

	if idx := strings.Index(s, "github.com:"); idx >= 0 {
		s = s[idx+len("github.com:"):]
	} else if idx := strings.Index(s, "github.com/"); idx >= 0 {
		s = s[idx+len("github.com/"):]
	} else {
		return ""
	}
	s = strings.TrimPrefix(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}
