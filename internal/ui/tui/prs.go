package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/keys"
)

// The PR page lists your open PRs and those asking for your review, in a
// section each, both at once; the filter key shows one or the other.
// Enter opens a PR's local clone in the detail view, o opens the PR.

// prFilter is which PRs the PR page shows.
type prFilter int

const (
	prsAll prFilter = iota
	prsMine
	prsReview
)

func (f prFilter) String() string {
	switch f {
	case prsMine:
		return "yours"
	case prsReview:
		return "review requested"
	}
	return "all"
}

// prLine is one line of the PR page: a section's heading, a note that it
// has none, or a PR.
type prLine struct {
	heading, note string
	pr            *git.PullRequest
}

// prMatches reports whether pr has every word of the search, whatever the
// case, in its repo, number, title, branch or author.
func prMatches(pr git.PullRequest, query string) bool {
	text := strings.ToLower(strings.Join([]string{pr.Repo, "#" + strconv.Itoa(pr.Number), pr.Title, pr.Branch, pr.Author}, " "))
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

// prLines is the PR page's lines: each section the filter shows, its
// heading, then its PRs that match the search.
func (m model) prLines() []prLine {
	var lines []prLine
	section := func(title string, prs []git.PullRequest, total int) {
		count := strconv.Itoa(total)
		var shown []git.PullRequest
		for _, pr := range prs {
			if prMatches(pr, m.searchQuery) {
				shown = append(shown, pr)
			}
		}
		switch {
		case m.searchQuery != "":
			count = fmt.Sprintf("%d of %d match", len(shown), len(prs))
		case total > len(prs):
			count = fmt.Sprintf("%d of %d", len(prs), total)
		}
		lines = append(lines, prLine{heading: title + " · " + count})
		if len(shown) == 0 {
			lines = append(lines, prLine{note: "none"})
		}
		for i := range shown {
			lines = append(lines, prLine{pr: &shown[i]})
		}
	}
	if m.prFilter != prsReview {
		section("YOURS", m.prs.Mine, m.prs.MineTotal)
	}
	if m.prFilter != prsMine {
		section("REVIEW REQUESTED", m.prs.ReviewRequested, m.prs.ReviewTotal)
	}
	return lines
}

// displayPRs is the PRs the PR page shows, in order: the ones the cursor
// moves through.
func (m model) displayPRs() []git.PullRequest {
	var out []git.PullRequest
	for _, l := range m.prLines() {
		if l.pr != nil {
			out = append(out, *l.pr)
		}
	}
	return out
}

// selectedPR is the PR under the cursor.
func (m model) selectedPR() (git.PullRequest, bool) {
	prs := m.displayPRs()
	if m.prCursor < len(prs) {
		return prs[m.prCursor], true
	}
	return git.PullRequest{}, false
}

// localRepo is the index in m.repos of pr's local clone.
func (m model) localRepo(pr git.PullRequest) (int, bool) {
	return git.LocalFor(pr, m.repos)
}

// trackPRs scrolls the PR under the cursor into view, its section's
// heading too when it is the first.
func (m model) trackPRs() model {
	lines := m.prLines()
	vis := m.visibleRows()
	at, n := -1, 0
	for i, l := range lines {
		if l.pr == nil {
			continue
		}
		if n == m.prCursor {
			at = i
			break
		}
		n++
	}
	if at < 0 {
		m.prOffset = 0
		return m
	}
	top := at
	if at > 0 && lines[at-1].heading != "" {
		top = at - 1
	}
	if top < m.prOffset {
		m.prOffset = top
	}
	if at >= m.prOffset+vis {
		m.prOffset = at - vis + 1
	}
	m.prOffset = max(0, min(m.prOffset, max(0, len(lines)-vis)))
	return m
}

// showPRs goes to the PR page, a search of the repos cleared.
func (m model) showPRs() model {
	m.state, m.searchQuery, m.prCursor, m.prOffset = statePRs, "", 0, 0
	return m
}

func (m model) handlePRsKey(action string) (tea.Model, tea.Cmd) {
	n := len(m.displayPRs())
	vis := m.visibleRows()
	switch action {
	case keys.MoveDown:
		m.prCursor = min(m.prCursor+1, max(0, n-1))
	case keys.MoveUp:
		m.prCursor = max(m.prCursor-1, 0)
	case keys.PageDown:
		m.prCursor = min(m.prCursor+vis, max(0, n-1))
	case keys.PageUp:
		m.prCursor = max(m.prCursor-vis, 0)
	case keys.First:
		m.prCursor = 0
	case keys.Last:
		m.prCursor = max(0, n-1)

	case keys.FilterPRs:
		m.prFilter = (m.prFilter + 1) % 3
		m.prCursor, m.prOffset = 0, 0

	case keys.Search:
		m.searching = true
		return m, nil

	case keys.OpenPR:
		if pr, ok := m.selectedPR(); ok {
			return m, openURLCmd(pr.URL)
		}

	case keys.Details:
		pr, ok := m.selectedPR()
		if !ok {
			return m, nil
		}
		i, ok := m.localRepo(pr)
		if !ok {
			m.statusMsg = pr.Repo + " isn't cloned in the directories git-repos scans"
			return m, nil
		}
		m.detailFrom, m.listCursor = statePRs, m.cursor
		return m.openDetail(i)

	case keys.OpenEditor:
		return m.openInDefault()

	case keys.PickEditor:
		return m.openPicker(), nil

	case keys.Refresh:
		if !m.noPRs && !m.prsLoading {
			m.prsLoading = true
			return m, tea.Batch(m.spinner.Tick, loadPRsCmd(m.repos))
		}

	case keys.SwitchPage:
		m.state, m.searchQuery = stateList, ""
		return m, nil

	case keys.Back:
		if m.searchQuery != "" {
			m.searchQuery, m.prCursor, m.prOffset = "", 0, 0
			return m, nil
		}
		m.state = stateList
		return m, nil
	}
	return m.trackPRs(), nil
}

// ── PR page view ─────────────────────────────────────────────────────────────

func (m model) viewPRs() string {
	if m.showPicker {
		return m.viewPicker(m.prHeaderLines())
	}
	var b strings.Builder
	b.WriteString(m.renderHeader3Zone(m.prHeaderLines()))
	b.WriteString(m.sep())
	b.WriteString(m.renderPRColHeader())
	b.WriteString(m.sep())

	vis := m.visibleRows()
	rows := m.prRows()
	for i := 0; i < vis; i++ {
		idx := m.prOffset + i
		if idx >= len(rows) {
			b.WriteString(fillBg("", m.width) + "\n")
			continue
		}
		b.WriteString(rows[idx] + "\n")
	}
	b.WriteString(m.sep())
	b.WriteString(m.renderStatusBar())
	return b.String()
}

// prRows is the PR page's lines drawn, or what it says in their place:
// why there are no PRs to show.
func (m model) prRows() []string {
	say := func(text string) []string {
		return []string{dimStyle.Width(m.width).Render("  " + text)}
	}
	switch {
	case m.noPRs:
		return say("PR lookup is off (--no-prs)")
	case m.ghUnavailable:
		return say("install the gh CLI and authenticate (gh auth login) to see your PRs")
	case !m.prsEverLoaded:
		return say("loading PRs…")
	case m.prsErr != nil && len(m.prs.Mine)+len(m.prs.ReviewRequested) == 0:
		return say("couldn't load PRs: " + m.prsErr.Error())
	}
	headingStyle := lipgloss.NewStyle().Background(rowBg).Foreground(colorPurple).Bold(true)
	var out []string
	n := 0
	for _, l := range m.prLines() {
		switch {
		case l.heading != "":
			out = append(out, headingStyle.Width(m.width).Render("  "+l.heading))
		case l.note != "":
			out = append(out, dimStyle.Width(m.width).Render("      "+l.note))
		default:
			out = append(out, m.renderPRRow(*l.pr, n == m.prCursor))
			n++
		}
	}
	return out
}

// prHeaderLines is the PR page's info: how many PRs of each, which the
// filter shows, and how the last lookup went.
func (m model) prHeaderLines() []string {
	label := func(s string) string { return hdrPurpleStyle.Render("  " + s + ": ") }
	count := func(n int) string {
		switch {
		case !m.prsEverLoaded:
			return hdrDimStyle.Render("…")
		case n == 0:
			return hdrDimStyle.Render("—")
		}
		return hdrInfoStyle.Render(strconv.Itoa(n))
	}
	lines := []string{
		label("Yours") + count(m.prs.MineTotal),
		label("Review requested") + count(m.prs.ReviewTotal),
		label("Showing") + hdrInfoStyle.Render(m.prFilter.String()),
	}
	switch {
	case m.prsLoading:
		lines = append(lines, label("PRs")+m.spinnerOn(headerBg)+hdrDimStyle.Render(" loading…"))
	case m.prsErr != nil:
		leftW := max(1, m.width/3-lipgloss.Width(label("Error")))
		lines = append(lines, label("Error")+attentionStyle.Background(headerBg).Render(trunc(m.prsErr.Error(), leftW)))
	}
	return lines
}

// PR page columns, besides REPO and TITLE, which share what is left.
const (
	wPRNum     = 6
	wPRAuthor  = 12
	wPRChecks  = 6
	wPRReview  = 8
	wPRUpdated = 8
	wPRLocal   = 6
)

// prCols is the PR page's resizable columns: REPO's width, TITLE's, and
// whether AUTHOR shows.
type prCols struct {
	repo, title int
	author      bool
}

// prColWidths fits the columns to the terminal: REPO as wide as the
// widest repo shown (12 to 28), TITLE the rest. Short of room for a
// title of 20, AUTHOR goes first, then REPO narrows to 12.
func (m model) prColWidths() prCols {
	const minTitle = 20
	c := prCols{repo: 12, author: true}
	for _, pr := range m.displayPRs() {
		c.repo = max(c.repo, lipgloss.Width(pr.Repo))
	}
	c.repo = min(c.repo, 28)
	// prefix(5) + each column and the space after it.
	fixed := 5 + wPRNum + 1 + wPRChecks + 1 + wPRReview + 1 + wPRUpdated + 1 + wPRLocal + 1
	room := m.width - fixed - c.repo - 1
	if room-(wPRAuthor+1) < minTitle {
		c.author = false
	} else {
		room -= wPRAuthor + 1
	}
	if room < minTitle {
		shrink := min(minTitle-room, c.repo-12)
		c.repo -= shrink
		room += shrink
	}
	c.title = max(1, room)
	return c
}

func (m model) renderPRColHeader() string {
	c := m.prColWidths()
	author := ""
	if c.author {
		author = fmt.Sprintf(" %-*s", wPRAuthor, "AUTHOR")
	}
	text := fmt.Sprintf("  ST %-*s %-*s %-*s%s %-*s %-*s %-*s %-*s",
		c.repo, "REPO", wPRNum, "#", c.title, "TITLE", author,
		wPRChecks, "CHECKS", wPRReview, "REVIEW", wPRUpdated, "UPDATED", wPRLocal, "LOCAL")
	return colHdrStyle.Width(m.width).Render(ansi.Truncate(text, m.width, "")) + "\n"
}

// prGroup is how a PR stands, as the repos' status groups color it: a
// draft is stale; failing checks, or changes asked for, need attention;
// checks running, or a review still to come, wait; the rest are ready.
func prGroup(pr git.PullRequest) git.Group {
	switch {
	case pr.Draft:
		return git.GroupStale
	case pr.Checks == "FAILURE" || pr.Checks == "ERROR" || pr.Review == "CHANGES_REQUESTED":
		return git.GroupAttention
	case pr.Checks == "PENDING" || pr.Checks == "EXPECTED" || pr.Review == "REVIEW_REQUIRED":
		return git.GroupPush
	}
	return git.GroupOK
}

func checksStr(state string) string {
	switch state {
	case "SUCCESS":
		return "✓ pass"
	case "FAILURE", "ERROR":
		return "✗ fail"
	case "PENDING", "EXPECTED":
		return "● run"
	}
	return "-"
}

func reviewStr(pr git.PullRequest) string {
	switch {
	case pr.Draft:
		return "draft"
	case pr.Review == "APPROVED":
		return "approved"
	case pr.Review == "CHANGES_REQUESTED":
		return "changes"
	case pr.Review == "REVIEW_REQUIRED":
		return "required"
	}
	return "-"
}

// localStr says whether pr's code is cloned here: checked out on its
// branch, cloned, or not.
func (m model) localStr(pr git.PullRequest) string {
	i, ok := m.localRepo(pr)
	switch {
	case !ok:
		return "-"
	case m.repos[i].Branch == pr.Branch:
		return "✓ here"
	}
	return "cloned"
}

// relTime is how long ago t was, briefly: 5m ago, 3h ago, 2d ago.
func relTime(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case t.IsZero():
		return "-"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	}
	return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
}

func (m model) renderPRRow(pr git.PullRequest, selected bool) string {
	group := prGroup(pr)
	icon, rowStyle := groupIconStyle(group)
	c := m.prColWidths()

	num := "#" + strconv.Itoa(pr.Number)
	numStr := makeHyperlink(pr.URL, num) + strings.Repeat(" ", max(0, wPRNum-len(num)))
	left := fmt.Sprintf("%-*s ", c.repo, trunc(pr.Repo, c.repo))
	author := ""
	if c.author {
		author = fmt.Sprintf(" %-*s", wPRAuthor, trunc(pr.Author, wPRAuthor))
	}
	right := fmt.Sprintf(" %-*s%s %-*s %-*s %-*s %-*s",
		c.title, trunc(pr.Title, c.title),
		author,
		wPRChecks, checksStr(pr.Checks),
		wPRReview, reviewStr(pr),
		wPRUpdated, relTime(pr.UpdatedAt, time.Now()),
		wPRLocal, m.localStr(pr))
	content := left + numStr + right

	if selected {
		var selStyle lipgloss.Style
		switch group {
		case git.GroupAttention:
			selStyle = selectedAttentionStyle
		case git.GroupPush:
			selStyle = selectedPushStyle
		case git.GroupStale:
			selStyle = selectedStaleStyle
		default:
			selStyle = selectedOkStyle
		}
		return selStyle.Width(m.width).Render(ansi.Truncate(fmt.Sprintf("▶ %-2s %s", icon, content), m.width, ""))
	}
	return rowStyle.Width(m.width).Render(ansi.Truncate(fmt.Sprintf("  %-2s %s", icon, content), m.width, ""))
}
