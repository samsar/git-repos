package plain

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/samsar/git-repos/internal/git"
	"golang.org/x/term"
)

// colour vars — zeroed out when noColor is set
var (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cDim     = "\033[2m"
	cRed     = "\033[91m"
	cYellow  = "\033[93m"
	cGreen   = "\033[92m"
	cCyan    = "\033[96m"
	cMagenta = "\033[95m"
)

func disableColor() {
	cReset = ""
	cBold = ""
	cDim = ""
	cRed = ""
	cYellow = ""
	cGreen = ""
	cCyan = ""
	cMagenta = ""
}

// Widths shared with the TUI come from the git package.
// wBranch and wSync are narrower here: plain uses fixed-width output, not
// adaptive terminal width like the TUI.
const (
	wST    = git.ColWidthStatus
	wRepo  = git.ColWidthRepo
	wChg   = git.ColWidthChanges
	wStash = git.ColWidthStash
	wWhen  = git.ColWidthWhen
	wPR    = git.ColWidthPR

	wBranch = 18
	wSync   = 10
	wMsg    = 40
)

func Run(dirs []string, doFetch, fetchPRs, noColor bool, hidden map[string]bool) error {
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if noColor || !isTTY {
		disableColor()
	}

	// Pre-count repos so we can show the total before scanning begins.
	var total int
	for _, dir := range dirs {
		repoDirs, err := git.FindRepoDirs(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot read %s: %v\n", dir, err)
			continue
		}
		total += len(repoDirs)
	}
	if total == 0 {
		fmt.Fprintf(os.Stderr, "No git repos found\n")
		return nil
	}

	fmt.Printf("\n%sScanning %d repos…%s\n", cBold, total, cReset)
	if doFetch {
		fmt.Printf("%s(running git fetch — this may take a moment)%s\n", cDim, cReset)
	}

	repos, err := git.ScanAll(dirs, doFetch)
	if err != nil {
		return err
	}
	if len(hidden) > 0 {
		kept := repos[:0]
		for _, r := range repos {
			if !hidden[r.Name] {
				kept = append(kept, r)
			}
		}
		repos = kept
	}

	if fetchPRs {
		fmt.Printf("%sLooking up open PRs…%s", cDim, cReset)
		git.FetchAndMatchPRs(repos)
		matched := 0
		for _, r := range repos {
			if r.PRNumber > 0 {
				matched++
			}
		}
		fmt.Printf("\r%sPR lookup done (%d matched)          %s\n", cDim, matched, cReset)
	}

	git.SortRepos(repos)
	printTable(repos)
	printSummary(repos)
	printLegend()
	return nil
}

func printTable(repos []git.RepoInfo) {
	header := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %-*s %-*s %-*s %s",
		wST, "ST",
		wRepo, "REPO",
		wBranch, "BRANCH",
		wSync, "SYNC",
		wChg, "CHANGES",
		wStash, "STASH",
		wWhen, "LAST CHANGED",
		wPR, "PR",
		"LAST COMMIT MSG",
	)
	sep := strings.Repeat("─", wST+1+wRepo+1+wBranch+1+wSync+1+wChg+1+wStash+1+wWhen+1+wPR+1+wMsg)
	fmt.Printf("\n%s%s%s\n%s\n", cBold, header, cReset, sep)

	now := time.Now().Unix()
	for _, info := range repos {
		if info.Error != "" {
			fmt.Printf("   %s%-*s ERROR: %s%s\n", cDim, wRepo, trunc(info.Name, wRepo), info.Error, cReset)
			continue
		}

		color, icon := rowColor(info, now)
		stashCol := "-"
		if info.StashCount > 0 {
			stashCol = fmt.Sprintf("%d", info.StashCount)
		}
		prCol := "-"
		prNote := ""
		if info.PRNumber > 0 {
			prCol = fmt.Sprintf("#%d", info.PRNumber)
			prNote = fmt.Sprintf("  %s%s%s", cDim, info.PRUrl, cReset)
		}

		fmt.Printf("%s%s %-*s %-*s %-*s %-*s %-*s %-*s %-*s %s%s%s\n",
			color, icon,
			wRepo, trunc(info.Name, wRepo),
			wBranch, trunc(info.Branch, wBranch),
			wSync, trunc(git.SyncStr(info), wSync),
			wChg, trunc(git.ChangesStr(info), wChg),
			wStash, stashCol,
			wWhen, trunc(info.LastRel, wWhen),
			wPR, prCol,
			trunc(info.LastMsg, wMsg),
			cReset, prNote,
		)
	}
	fmt.Println()
}

func printSummary(repos []git.RepoInfo) {
	var needsAttention, needsPush, onBranch, hasPR, hasStash []git.RepoInfo
	for _, r := range repos {
		if r.Staged > 0 || r.Modified > 0 || r.Behind > 0 {
			needsAttention = append(needsAttention, r)
		}
		if r.Ahead > 0 {
			needsPush = append(needsPush, r)
		}
		if !git.IsMainBranch(r.Branch) && r.Branch != "?" {
			onBranch = append(onBranch, r)
		}
		if r.PRNumber > 0 {
			hasPR = append(hasPR, r)
		}
		if r.StashCount > 0 {
			hasStash = append(hasStash, r)
		}
	}

	fmt.Printf("%sSummary:%s\n  %d repos total\n", cBold, cReset, len(repos))
	if len(needsAttention) > 0 {
		fmt.Printf("  %s%d need attention%s: %s\n", cRed, len(needsAttention), cReset, repoNameList(needsAttention, 8))
	}
	if len(needsPush) > 0 {
		parts := make([]string, 0, len(needsPush))
		for i, r := range needsPush {
			if i >= 8 {
				parts = append(parts, "…")
				break
			}
			parts = append(parts, fmt.Sprintf("%s(↑%d)", r.Name, r.Ahead))
		}
		fmt.Printf("  %s%d need push%s: %s\n", cYellow, len(needsPush), cReset, strings.Join(parts, ", "))
	}
	if len(onBranch) > 0 {
		parts := make([]string, 0, len(onBranch))
		for i, r := range onBranch {
			if i >= 8 {
				parts = append(parts, "…")
				break
			}
			parts = append(parts, fmt.Sprintf("%s(%s)", r.Name, r.Branch))
		}
		fmt.Printf("  %s%d on non-main branch%s: %s\n", cYellow, len(onBranch), cReset, strings.Join(parts, ", "))
	}
	if len(hasPR) > 0 {
		parts := make([]string, 0, len(hasPR))
		for _, r := range hasPR {
			parts = append(parts, fmt.Sprintf("%s #%d", r.Name, r.PRNumber))
		}
		fmt.Printf("  %s%d with open PR%s: %s\n", cCyan, len(hasPR), cReset, strings.Join(parts, ", "))
	}
	if len(hasStash) > 0 {
		parts := make([]string, 0, len(hasStash))
		for _, r := range hasStash {
			parts = append(parts, fmt.Sprintf("%s(%d)", r.Name, r.StashCount))
		}
		fmt.Printf("  %s%d with stashed changes%s: %s\n", cMagenta, len(hasStash), cReset, strings.Join(parts, ", "))
	}
	fmt.Println()
}

func printLegend() {
	fmt.Printf("  %s !%s  needs attention (dirty working tree or behind upstream)\n", cRed, cReset)
	fmt.Printf("  %s ↑%s  needs push / feature branch\n", cYellow, cReset)
	fmt.Printf("  %s ✓%s  clean and in sync\n", cGreen, cReset)
	fmt.Printf("  %s ·%s  stale (>6 months)\n", cDim, cReset)
	fmt.Printf("\n  SYNC: ✓=in sync  ↑N=push  ↓N=pull  no-remote=no upstream\n")
	fmt.Printf("  CHANGES: S=staged  M=modified  ?=untracked\n\n")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func rowColor(info git.RepoInfo, now int64) (color, icon string) {
	switch git.StatusGroup(info, now) {
	case git.GroupAttention:
		return cRed, " !"
	case git.GroupPush:
		return cYellow, " ↑"
	case git.GroupStale:
		return cDim, " ·"
	default:
		return cGreen, " ✓"
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func repoNameList(repos []git.RepoInfo, max int) string {
	var parts []string
	for i, r := range repos {
		if i >= max {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, r.Name)
	}
	return strings.Join(parts, ", ")
}
