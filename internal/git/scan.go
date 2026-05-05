package git

import (
	"cmp"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Group classifies a repo's overall status for sorting and display.
type Group int

const (
	GroupAttention Group = iota // dirty working tree or behind upstream
	GroupPush                   // ahead of upstream or on a feature branch
	GroupOK                     // clean and in sync on a main branch
	GroupStale                  // no activity for 6+ months
)

type ScanResult struct {
	Repo  RepoInfo
	Done  int
	Total int
}

// ScanAll scans multiple directories and returns all repos.
func ScanAll(bases []string, doFetch bool) ([]RepoInfo, error) {
	ch, total, err := ScanAllAsync(bases, doFetch)
	if err != nil {
		return nil, err
	}
	repos := make([]RepoInfo, 0, total)
	for r := range ch {
		repos = append(repos, r.Repo)
	}
	return repos, nil
}

// ScanAllAsync finds all repos upfront, then streams results as each is scanned.
// The returned channel is closed when all repos are done. Total is returned so
// callers can show progress before the first result arrives.
func ScanAllAsync(bases []string, doFetch bool) (<-chan ScanResult, int, error) {
	var allDirs []string
	for _, base := range bases {
		dirs, err := FindRepoDirs(base)
		if err != nil {
			return nil, 0, err
		}
		allDirs = append(allDirs, dirs...)
	}

	total := len(allDirs)
	ch := make(chan ScanResult, total)

	go func() {
		defer close(ch)
		var (
			mu   sync.Mutex
			done int
			sem  = make(chan struct{}, 12)
			wg   sync.WaitGroup
		)
		for _, d := range allDirs {
			wg.Add(1)
			go func(dir string) {
				defer wg.Done()
				sem <- struct{}{}
				r := CollectRepo(dir, doFetch)
				<-sem
				mu.Lock()
				done++
				ch <- ScanResult{Repo: r, Done: done, Total: total}
				mu.Unlock()
			}(d)
		}
		wg.Wait()
	}()

	return ch, total, nil
}

func FindRepoDirs(base string) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(base, e.Name())
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			dirs = append(dirs, p)
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func SortRepos(repos []RepoInfo) {
	now := time.Now().Unix()
	sort.Slice(repos, func(i, j int) bool {
		gi, gj := StatusGroup(repos[i], now), StatusGroup(repos[j], now)
		if gi != gj {
			return gi < gj
		}
		if repos[i].LastTS != repos[j].LastTS {
			return repos[i].LastTS > repos[j].LastTS
		}
		return repos[i].Name < repos[j].Name
	})
}

// StatusGroup returns the display/sort priority for a repo.
func StatusGroup(r RepoInfo, now int64) Group {
	if r.Staged > 0 || r.Modified > 0 || r.Behind > 0 {
		return GroupAttention
	}
	if r.Ahead > 0 || r.Untracked > 0 || !IsMainBranch(r.Branch) {
		return GroupPush
	}
	if r.LastTS > 0 && now-r.LastTS > 60*60*24*180 {
		return GroupStale
	}
	return GroupOK
}

func IsMainBranch(b string) bool {
	return b == "main" || b == "master" || b == "develop"
}

// ── Column sorting ────────────────────────────────────────────────────────────

type SortColumn int

const (
	SortStatus      SortColumn = iota
	SortName
	SortBranch
	SortSync
	SortChanges
	SortLastChanged
	SortPR
)

func SortReposByCol(repos []RepoInfo, col SortColumn, desc bool) {
	now := time.Now().Unix()
	sort.Slice(repos, func(i, j int) bool {
		a, b := repos[i], repos[j]
		var c int
		switch col {
		case SortName:
			c = cmp.Compare(a.Name, b.Name)
		case SortBranch:
			c = cmp.Compare(a.Branch, b.Branch)
		case SortSync:
			// higher urgency (more behind/ahead) sorts first
			c = cmp.Compare(b.Behind*1000+b.Ahead, a.Behind*1000+a.Ahead)
		case SortChanges:
			c = cmp.Compare(b.Staged+b.Modified+b.Untracked, a.Staged+a.Modified+a.Untracked)
		case SortLastChanged:
			c = cmp.Compare(b.LastTS, a.LastTS)
		case SortPR:
			hasPRA, hasPRB := a.PRNumber > 0, b.PRNumber > 0
			if hasPRA != hasPRB {
				if hasPRA {
					c = -1
				} else {
					c = 1
				}
			} else {
				c = cmp.Compare(b.PRNumber, a.PRNumber)
			}
		default: // SortStatus
			c = cmp.Compare(StatusGroup(a, now), StatusGroup(b, now))
			if c == 0 {
				c = cmp.Compare(b.LastTS, a.LastTS)
			}
		}

		// Deterministic tie-breaker keeps the comparator strict.
		if c == 0 {
			if c2 := cmp.Compare(a.Name, b.Name); c2 != 0 {
				c = c2
			} else {
				c = cmp.Compare(a.Path, b.Path)
			}
		}
		if desc {
			return c > 0
		}
		return c < 0
	})
}
