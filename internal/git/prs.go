package git

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

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
