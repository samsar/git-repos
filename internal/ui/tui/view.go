package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/samsar/git-repos/internal/git"
)

// logoLines is the ASCII-art logo rendered in the top-right corner.
var logoLines = []string{
	"   __   _ __  ",
	" /'_ `\\/\\`'__\\",
	"/\\ \\L\\ \\ \\ \\/ ",
	"\\ \\____ \\ \\_\\ ",
	" \\/___L\\ \\/_/ ",
	"   /\\____/    ",
	"   \\_/__/     ",
}

// logoColW is the total width reserved for the logo column (art + padding).
const logoColW = 18

// ── Help screen static data ───────────────────────────────────────────────────

type helpEntry struct{ key, desc string }

var helpNav = []helpEntry{
	{"↑↓/jk", "Navigate"},
	{"ctrl-f", "Page Down"},
	{"ctrl-b", "Page Up"},
	{"g", "Top"},
	{"G", "Bottom"},
	{"enter", "Detail view"},
	{"esc", "Back / Close"},
}

var helpActions = []helpEntry{
	{"/", "Search / filter list"},
	{"o", "Open PR in browser"},
	{"g", "Open repo in GitHub"},
	{"p", "Pull repo"},
	{"ctrl-p", "Pull all repos"},
	{"s", "Open shell in repo"},
	{"r", "Refresh all"},
	{"ctrl-d", "Delete repo from disk"},
	{"z", "Settings"},
	{"q", "Quit"},
	{"?", "Help"},
}

var helpSorts = []helpEntry{
	{"0", "Status (default)"},
	{"shift-N", "Name"},
	{"shift-B", "Branch"},
	{"shift-S", "Sync"},
	{"shift-C", "Changes"},
	{"shift-T", "Time"},
	{"shift-P", "PR"},
	{"", "↵ again to reverse ▲▼"},
}

type iconDesc struct{ icon, meaning string }

var helpIcons = []iconDesc{
	{"!", "staged changes, modified files, or behind upstream — act on this now"},
	{"↑", "commits to push, untracked files, or on a feature branch"},
	{"✓", "clean, in sync, on main / master / develop"},
	{"·", "no activity in 6+ months"},
}

type colDesc struct{ name, vals string }

var helpCols = []colDesc{
	{"REPO", "repository directory name"},
	{"BRANCH", "active git branch name"},
	{"SYNC", "✓ in sync  ↑N ahead  ↓N behind  no-remote"},
	{"CHANGES", "clean  S:N staged  M:N modified  ?:N untracked"},
	{"STASH", "number of stash entries, - if none"},
	{"LAST CHANGED", "age of the most recent commit"},
	{"PR", "#N open pull request, - if none"},
	{"LAST MSG", "most recent commit message"},
}

// ── Top-level dispatch ────────────────────────────────────────────────────────

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.showHelp {
		return m.viewHelp()
	}
	switch m.state {
	case stateScanning:
		return m.viewScanning()
	case stateList:
		return m.viewList()
	case stateDetail:
		return m.viewDetail()
	case stateSettings:
		return m.viewSettings()
	}
	return ""
}

// ── Scanning view ─────────────────────────────────────────────────────────────

func (m model) viewScanning() string {
	var b strings.Builder
	b.WriteString(m.renderHeader3Zone(m.listHeaderLines()))
	b.WriteString(m.sep())

	// Fill blank lines so the status bar lands at the bottom.
	// overhead: headerHeight + 1 sep + 1 status bar = headerHeight + 2
	blankLines := m.height - m.headerHeight() - 2
	if blankLines < 0 {
		blankLines = 0
	}
	for i := 0; i < blankLines; i++ {
		b.WriteString(fillBg("", m.width) + "\n")
	}

	var msg string
	if m.scanTotal > 0 {
		msg = fmt.Sprintf("  %s  Scanning repos… (%d / %d)", m.spinner.View(), m.scanDone, m.scanTotal)
	} else {
		msg = fmt.Sprintf("  %s  Scanning repos…", m.spinner.View())
	}

	style := lipgloss.NewStyle().
		Background(colorStatusBarBg).
		Foreground(staleFg)
	b.WriteString(style.Width(m.width).Render(msg))
	return b.String()
}

// ── List view ─────────────────────────────────────────────────────────────────

func (m model) viewList() string {
	if m.showDeleteConfirm {
		return m.viewConfirmDelete()
	}
	var b strings.Builder
	b.WriteString(m.renderHeader3Zone(m.listHeaderLines()))
	b.WriteString(m.sep())
	b.WriteString(m.renderColHeader())
	b.WriteString(m.sep())

	visRows := m.visibleRows()
	filtered := m.displayRepos()
	for i := 0; i < visRows; i++ {
		idx := m.offset + i
		if idx >= len(filtered) {
			b.WriteString(fillBg("", m.width) + "\n")
			continue
		}
		b.WriteString(m.renderRow(filtered[idx], idx == m.cursor) + "\n")
	}
	b.WriteString(m.sep())
	b.WriteString(m.renderStatusBar())
	return b.String()
}

// ── Detail view ───────────────────────────────────────────────────────────────

func (m model) viewDetail() string {
	if len(m.repos) == 0 || m.cursor >= len(m.repos) {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.renderHeader3Zone(m.detailInfoLines()))
	b.WriteString(m.sep())
	b.WriteString(m.detailVP.View())
	b.WriteString("\n")
	b.WriteString(m.sep())
	b.WriteString(m.renderStatusBar())
	return b.String()
}

// ── Help view (full-screen, k9s style) ───────────────────────────────────────

func (m model) viewHelp() string {
	bg := headerBg
	fill := lipgloss.NewStyle().Background(bg)
	kStyle := lipgloss.NewStyle().Background(bg).Foreground(colorCyan)
	hStyle := lipgloss.NewStyle().Background(bg).Foreground(colorBrightWhite).Bold(true)
	dStyle := lipgloss.NewStyle().Background(bg).Foreground(colorText)
	noteStyle := lipgloss.NewStyle().Background(bg).Foreground(staleFg)

	key := func(k string) string { return kStyle.Render("<" + k + ">") }

	colW := m.width / 3

	renderEntry := func(e helpEntry, w int) string {
		if e.key == "" {
			return fill.Width(w).Render(fill.Render("    ") + noteStyle.Render(e.desc))
		}
		k := key(e.key)
		kw := lipgloss.Width(k)
		pad := max(0, 16-kw)
		return fill.Width(w).Render(fill.Render(" ") + k + fill.Render(strings.Repeat(" ", pad)) + dStyle.Render(e.desc))
	}

	// Top bar
	right := kStyle.Render("<esc>") + dStyle.Render(" Back ")
	gapW := max(1, m.width-lipgloss.Width(right))
	topBar := fill.Width(gapW).Render("") + right

	// Separator with "Help" centered
	const helpLabel = " Help "
	sideW := (m.width - len(helpLabel)) / 2
	separator := sepStyle.Render(strings.Repeat("─", sideW)) +
		boldStyle.Background(bg).Render(helpLabel) +
		sepStyle.Render(strings.Repeat("─", m.width-sideW-len(helpLabel)))

	// Column headers + underlines
	colHeaders := hStyle.Width(colW).Render(" NAVIGATION") +
		hStyle.Width(colW).Render(" ACTIONS") +
		hStyle.Width(m.width-2*colW).Render(" SORT")
	colUnders := fill.Width(colW).Render(" "+noteStyle.Render(strings.Repeat("─", 10))) +
		fill.Width(colW).Render(" "+noteStyle.Render(strings.Repeat("─", 7))) +
		fill.Width(m.width-2*colW).Render(" "+noteStyle.Render(strings.Repeat("─", 4)))

	var b strings.Builder
	b.WriteString(topBar + "\n")
	b.WriteString(separator + "\n")
	b.WriteString(colHeaders + "\n")
	b.WriteString(colUnders + "\n")

	maxRows := max(max(len(helpNav), len(helpActions)), len(helpSorts))
	for i := 0; i < maxRows; i++ {
		var e1, e2, e3 helpEntry
		if i < len(helpNav) {
			e1 = helpNav[i]
		}
		if i < len(helpActions) {
			e2 = helpActions[i]
		}
		if i < len(helpSorts) {
			e3 = helpSorts[i]
		}
		b.WriteString(renderEntry(e1, colW) + renderEntry(e2, colW) + renderEntry(e3, m.width-2*colW) + "\n")
	}

	descStyle := lipgloss.NewStyle().Background(bg).Foreground(colorSubduedGray)
	nameStyle := lipgloss.NewStyle().Background(bg).Foreground(colorCyan).Bold(true)

	blank := fill.Width(m.width).Render("") + "\n"
	renderSection := func(label string) {
		b.WriteString(blank)
		sideW := (m.width - len(label)) / 2
		b.WriteString(sepStyle.Render(strings.Repeat("─", sideW)) +
			boldStyle.Background(bg).Render(label) +
			sepStyle.Render(strings.Repeat("─", m.width-sideW-len(label))) + "\n")
	}

	// ── Status icons section — single column ──────────────────────────────────
	iconFgs := []lipgloss.Color{attentionFg, pushFg, okFg, staleFg}
	renderIconEntry := func(idx int) string {
		ic := helpIcons[idx]
		styled := lipgloss.NewStyle().Background(bg).Foreground(iconFgs[idx]).Bold(true).Render(ic.icon)
		return fill.Width(m.width).Render(fill.Render("  ") + styled + fill.Render("  ") + descStyle.Render(ic.meaning))
	}

	renderSection(" Status icons ")
	for i := 0; i < len(helpIcons); i++ {
		b.WriteString(renderIconEntry(i) + "\n")
	}

	// ── Columns section — single column ───────────────────────────────────────
	const nameW = 14 // wide enough for "LAST CHANGED"
	renderColEntry := func(c colDesc) string {
		name := nameStyle.Render(fmt.Sprintf("%-*s", nameW, c.name))
		return fill.Width(m.width).Render(fill.Render("  ") + name + fill.Render("  ") + descStyle.Render(c.vals))
	}

	renderSection(" Column Descriptions ")
	for _, c := range helpCols {
		b.WriteString(renderColEntry(c) + "\n")
	}

	// Fill remaining lines
	iconsRows := 1 + 1 + len(helpIcons) // blank + sep + content
	colsRows := 1 + 1 + len(helpCols)   // blank + sep + content
	written := 4 + maxRows + iconsRows + colsRows
	for i := written; i < m.height; i++ {
		b.WriteString(fill.Width(m.width).Render("") + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// ── Header rendering ──────────────────────────────────────────────────────────

// listHeaderLines returns up to 8 info lines for the list/scanning view.
// The logo covers lines 1-5; lines 6-8 extend below it.
func (m model) listHeaderLines() []string {
	dir := "."
	if len(m.scanDirs) == 1 {
		dir = m.scanDirs[0]
	} else if len(m.scanDirs) > 1 {
		dir = fmt.Sprintf("%d directories", len(m.scanDirs))
	}

	lines := []string{
		hdrPurpleStyle.Render("  Root: ") + hdrInfoStyle.Render(dir),
	}

	if s := m.nonMainSummary(); s != "" {
		lines = append(lines, s)
	} else {
		lines = append(lines, "")
	}

	if s := m.openPRSummary(); s != "" {
		lines = append(lines, s)
	} else {
		lines = append(lines, "")
	}

	// Status last — detailed breakdown sits under the summary counts
	lines = append(lines, hdrPurpleStyle.Render("  Status: ")+m.repoCountLine())

	return lines
}

// nonMainSummary returns "  Feature Branches: N" or "" if none.
func (m model) nonMainSummary() string {
	count := 0
	for _, r := range m.repos {
		if !git.IsMainBranch(r.Branch) && r.Branch != "?" && r.Error == "" {
			count++
		}
	}
	if count == 0 {
		return ""
	}
	return hdrPurpleStyle.Render("  Feature Branches: ") + hdrInfoStyle.Render(fmt.Sprintf("%d", count))
}

// openPRSummary always renders the "PRs:" label when PRs are enabled so it
// stays visible during bootup and refresh.
func (m model) openPRSummary() string {
	if m.noPRs {
		return ""
	}
	if m.ghUnavailable {
		return hdrPurpleStyle.Render("  PRs: ") + hdrDimStyle.Render("install gh CLI and authenticate")
	}
	if m.prsLoading {
		return hdrPurpleStyle.Render("  PRs: ") + hdrDimStyle.Render(m.spinner.View()+" loading…")
	}
	if !m.prsEverLoaded {
		return hdrPurpleStyle.Render("  PRs: ") + hdrDimStyle.Render("…")
	}
	count := 0
	for _, r := range m.repos {
		if r.PRNumber > 0 {
			count++
		}
	}
	if count == 0 {
		return hdrPurpleStyle.Render("  PRs: ") + hdrDimStyle.Render("—")
	}
	return hdrPurpleStyle.Render("  PRs: ") + hdrInfoStyle.Render(fmt.Sprintf("%d", count))
}

// detailInfoLines returns info lines for the detail view — same structure as listHeaderLines.
func (m model) detailInfoLines() []string {
	if m.cursor >= len(m.repos) {
		return m.listHeaderLines()
	}
	r := m.repos[m.cursor]

	var syncStr string
	if r.NoUpstream {
		syncStr = dimStyle.Render("no upstream")
	} else if r.Ahead == 0 && r.Behind == 0 {
		syncStr = okStyle.Background(headerBg).Render("✓ in sync")
	} else {
		var parts []string
		if r.Ahead > 0 {
			parts = append(parts, attentionStyle.Background(headerBg).Render(fmt.Sprintf("↑%d ahead", r.Ahead)))
		}
		if r.Behind > 0 {
			parts = append(parts, attentionStyle.Background(headerBg).Render(fmt.Sprintf("↓%d behind", r.Behind)))
		}
		syncStr = strings.Join(parts, "  ")
	}

	var changesStr string
	if r.Staged == 0 && r.Modified == 0 && r.Untracked == 0 {
		changesStr = okStyle.Background(headerBg).Render("clean")
	} else {
		var parts []string
		if r.Staged > 0 {
			parts = append(parts, fmt.Sprintf("S:%d", r.Staged))
		}
		if r.Modified > 0 {
			parts = append(parts, fmt.Sprintf("M:%d", r.Modified))
		}
		if r.Untracked > 0 {
			parts = append(parts, fmt.Sprintf("?:%d", r.Untracked))
		}
		changesStr = attentionStyle.Background(headerBg).Render(strings.Join(parts, " "))
	}

	var lastLine string
	if r.PRNumber > 0 {
		lastLine = hdrPurpleStyle.Render("  PR: ") +
			cyanStyle.Background(headerBg).Render(fmt.Sprintf("#%d open", r.PRNumber))
	}
	lastLine += hdrPurpleStyle.Render("  Last: ") + hdrInfoStyle.Render(r.LastRel)

	leftW := m.width / 3
	branchLabel := hdrPurpleStyle.Render("  Branch: ")
	branchMax := max(0, leftW-lipgloss.Width(branchLabel))

	return []string{
		hdrPurpleStyle.Render("  Root: ") + boldStyle.Background(headerBg).Render(r.Name),
		branchLabel + hdrInfoStyle.Render(trunc(r.Branch, branchMax)),
		hdrPurpleStyle.Render("  Upstream: ") + syncStr + hdrDimStyle.Render("   Changes: ") + changesStr,
		lastLine,
	}
}

// repoCountLine builds the stats line: "29 repos  ! 2  ↑ 4  ✓ 23".
// When a search filter is active it shows "5 / 29 repos".
func (m model) repoCountLine() string {
	now := time.Now().Unix()
	total := len(m.repos)
	display := m.displayRepos()
	var att, push, ok, stale int
	for _, r := range display {
		switch git.StatusGroup(r, now) {
		case git.GroupAttention:
			att++
		case git.GroupPush:
			push++
		case git.GroupStale:
			stale++
		default:
			ok++
		}
	}

	var s string
	if m.searchQuery != "" {
		s = hdrInfoStyle.Render(fmt.Sprintf("%d / %d repos", len(display), total))
	} else {
		s = hdrInfoStyle.Render(fmt.Sprintf("%d repos", total))
	}
	if att > 0 {
		s += attentionStyle.Background(headerBg).Render(fmt.Sprintf("  ! %d", att))
	}
	if push > 0 {
		s += pushStyle.Background(headerBg).Render(fmt.Sprintf("  ↑ %d", push))
	}
	if ok > 0 {
		s += okStyle.Background(headerBg).Render(fmt.Sprintf("  ✓ %d", ok))
	}
	if stale > 0 {
		s += staleStyle.Background(headerBg).Render(fmt.Sprintf("  · %d", stale))
	}
	return s
}

// actionLines returns one line per action for the middle header zone,
// adjusted to show only the actions available in the current view.
func (m model) actionLines() []string {
	return m.actionLinesFor(m.state)
}

// actionLinesFor returns the action shortcuts for a specific view state.
// Split out so the header can compute a consistent centering width across
// views (otherwise the shortcut block jumps horizontally between list and
// detail because their action sets have different widest lines).
func (m model) actionLinesFor(state viewState) []string {
	type act struct{ key, desc string }

	var col1, col2 []act
	if state == stateDetail {
		col1 = []act{
			{"esc", "Back to list"},
			{"o", "Open PR"},
			{"g", "Open in GitHub"},
			{"p", "Pull repo"},
		}
		col2 = []act{
			{"s", "Open shell"},
			{"r", "Refresh"},
			{"q", "Quit"},
			{"?", "Help"},
		}
	} else {
		col1 = []act{
			{"enter", "Detail view"},
			{"/", "Search"},
			{"o", "Open PR"},
			{"p", "Pull repo"},
			{"ctrl+p", "Pull all"},
		}
		col2 = []act{
			{"s", "Open shell"},
			{"ctrl+d", "Delete repo"},
			{"r", "Refresh"},
			{"z", "Settings"},
			{"?", "Help"},
		}
	}

	maxKeyWFor := func(acts []act) int {
		w := 0
		for _, a := range acts {
			if kw := lipgloss.Width(hdrKeyStyle.Render("<" + a.key + ">")); kw > w {
				w = kw
			}
		}
		return w
	}
	maxKeyW1 := maxKeyWFor(col1)
	maxKeyW2 := maxKeyWFor(col2)

	renderEntry := func(a act, maxKeyW int) string {
		k := hdrKeyStyle.Render("<" + a.key + ">")
		pad := strings.Repeat(" ", maxKeyW-lipgloss.Width(k))
		return k + hdrKeyDescStyle.Render(pad+" "+a.desc)
	}

	col1W := 0
	for _, a := range col1 {
		if w := lipgloss.Width(renderEntry(a, maxKeyW1)); w > col1W {
			col1W = w
		}
	}

	var lines []string
	for i := 0; i < max(len(col1), len(col2)); i++ {
		row := hdrKeyDescStyle.Render(strings.Repeat(" ", col1W+4))
		if i < len(col1) {
			e := renderEntry(col1[i], maxKeyW1)
			row = e + hdrKeyDescStyle.Render(strings.Repeat(" ", max(0, col1W-lipgloss.Width(e)+4)))
		}
		if i < len(col2) {
			row += renderEntry(col2[i], maxKeyW2)
		}
		lines = append(lines, row)
	}
	return lines
}

// renderHeader3Zone renders: [left info ~1/3] [middle actions ~1/3] [right logo].
// All three zones start at row 0 so the first info line (Root: …), first
// shortcut, and first logo line share the top row.
func (m model) renderHeader3Zone(leftLines []string) string {
	midLines := m.actionLines()

	fill := lipgloss.NewStyle().Background(headerBg)
	logoS := lipgloss.NewStyle().Foreground(colorPurple).Background(headerBg).Bold(true)

	thirdW := m.width / 3
	showLogo := m.width-thirdW*2 >= logoColW+8
	rightW := 0
	if showLogo {
		rightW = logoColW
	}
	leftW := thirdW
	midW := m.width - leftW - rightW

	// Center based on the widest possible shortcut block (the list view's
	// action set is always wider than the detail view's). Using the current
	// view's width would shift the shortcuts horizontally between views.
	refLines := m.actionLinesFor(stateList)
	maxMidW := 0
	for _, ml := range refLines {
		if w := lipgloss.Width(ml); w > maxMidW {
			maxMidW = w
		}
	}
	midLeftPad := max(0, m.width/2-maxMidW/2-leftW)

	totalRows := max(len(midLines), len(logoLines))
	if totalRows <= len(leftLines) {
		totalRows = len(leftLines) + 1
	}
	// Legend anchored to the last row of the header, right above the separator.
	legendIdx := totalRows - 1

	renderLogoCell := func(i int) string {
		if !showLogo {
			return ""
		}
		if i >= 0 && i < len(logoLines) {
			return fill.Render("  ") + logoS.Render(logoLines[i]) + fill.Render("  ")
		}
		return fill.Width(rightW).Render("")
	}

	var b strings.Builder
	for i := 0; i < totalRows; i++ {
		if i == legendIdx {
			legend := m.legendContent()
			row := fill.Width(leftW + midW).Render(legend)
			row += renderLogoCell(i)
			b.WriteString(row + "\n")
			continue
		}

		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		mid := ""
		if i < len(midLines) {
			mid = strings.Repeat(" ", midLeftPad) + midLines[i]
		}

		row := fill.Width(leftW).Render(left) + fill.Width(midW).Render(mid) + renderLogoCell(i)
		b.WriteString(row + "\n")
	}

	return b.String()
}

// renderStatusBar renders the bottom status line showing async operation state
// or the active search input.
func (m model) renderStatusBar() string {
	style := lipgloss.NewStyle().
		Background(colorStatusBarBg).
		Foreground(staleFg)

	if m.searching {
		queryStyle := lipgloss.NewStyle().Background(colorStatusBarBg).Foreground(colorCyan)
		content := queryStyle.Render("  /" + m.searchQuery + "█")
		return style.Width(m.width).Render(content)
	}

	if m.searchQuery != "" {
		queryStyle := lipgloss.NewStyle().Background(colorStatusBarBg).Foreground(colorCyan)
		hintStyle := lipgloss.NewStyle().Background(colorStatusBarBg).Foreground(colorFaintGray)
		content := queryStyle.Render("  /"+m.searchQuery) + hintStyle.Render("  (/ to edit · esc to clear)")
		return style.Width(m.width).Render(content)
	}

	var content string
	switch {
	case m.refreshing:
		if m.scanTotal > 0 {
			content = fmt.Sprintf("  %s  Refreshing… (%d / %d)", m.spinner.View(), m.scanDone, m.scanTotal)
		} else {
			content = fmt.Sprintf("  %s  Refreshing…", m.spinner.View())
		}
	case m.prsLoading:
		content = fmt.Sprintf("  %s  Loading PRs…", m.spinner.View())
	case m.fetchingPR:
		content = fmt.Sprintf("  %s  %s", m.spinner.View(), m.statusMsg)
	case m.statusMsg != "":
		content = "  " + m.statusMsg
	}

	return style.Width(m.width).Render(content)
}

// legendContent returns the colour legend string (no width padding).
func (m model) legendContent() string {
	bg := lipgloss.NewStyle().Background(headerBg)
	entry := func(icon, desc string, st lipgloss.Style) string {
		return bg.Render("  ") + st.Background(headerBg).Render(icon) +
			hdrLegendTextStyle.Render(" "+desc)
	}
	return entry("!", "needs attention", attentionStyle) +
		entry("  ↑", "push / branch", pushStyle) +
		entry("  ✓", "clean", okStyle) +
		entry("  ·", "stale (6mo+)", staleStyle)
}

// ── Column header ─────────────────────────────────────────────────────────────

func (m model) renderColHeader() string {
	bg := headerBg
	col := func(sortID git.SortColumn, name string, width int) string {
		label := name
		var s lipgloss.Style
		if m.sortCol == sortID {
			if m.sortDesc {
				label += "▼"
			} else {
				label += "▲"
			}
			s = colHdrSortedStyle
		} else {
			s = colHdrStyle
		}
		return s.Render(fmt.Sprintf("%-*s ", width, trunc(label, width)))
	}
	noSort := func(name string, width int) string {
		return colHdrStyle.Render(fmt.Sprintf("%-*s ", width, trunc(name, width)))
	}

	wBranch, wSync, wMsg := m.colWidths()
	line := lipgloss.NewStyle().Background(bg).Render("   ") +
		col(git.SortName, "REPO", wRepo+wST) +
		col(git.SortBranch, "BRANCH", wBranch) +
		col(git.SortSync, "SYNC", wSync) +
		col(git.SortChanges, "CHANGES", wChg) +
		noSort("STASH", wStash) +
		col(git.SortLastChanged, "LAST CHANGED", wWhen) +
		col(git.SortPR, "PR", wPR) +
		colHdrStyle.Render(fmt.Sprintf("%-*s", wMsg, "LAST MSG"))

	lw := lipgloss.Width(line)
	if lw < m.width {
		line += lipgloss.NewStyle().Background(bg).Width(m.width - lw).Render("")
	}
	return line + "\n"
}

// ── Row rendering ─────────────────────────────────────────────────────────────

func (m model) renderRow(info git.RepoInfo, selected bool) string {
	now := time.Now().Unix()
	icon, rowStyle := groupIconStyle(git.StatusGroup(info, now))

	stashStr := "-"
	if info.StashCount > 0 {
		stashStr = strconv.Itoa(info.StashCount)
	}

	// Build PR column: hyperlink wraps the text so CMD+click opens the PR.
	// We pad manually to avoid fmt counting ANSI/OSC bytes as visual width.
	prStr := "-" + strings.Repeat(" ", wPR-1)
	if info.PRNumber > 0 {
		prText := fmt.Sprintf("#%d", info.PRNumber)
		pad := strings.Repeat(" ", max(0, wPR-len(prText)))
		if info.PRUrl != "" {
			prStr = makeHyperlink(info.PRUrl, prText) + pad
		} else {
			prStr = prText + pad
		}
	}

	wBranch, wSync, wMsg := m.colWidths()

	// Build content in two halves so prStr (which may contain OSC sequences)
	// is never passed through a %-*s width specifier.
	left := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %-*s ",
		wRepo, trunc(info.Name, wRepo),
		wBranch, trunc(info.Branch, wBranch),
		wSync, trunc(git.SyncStr(info), wSync),
		wChg, trunc(git.ChangesStr(info), wChg),
		wStash, stashStr,
		wWhen, trunc(info.LastRel, wWhen),
	)
	content := left + prStr + " " + trunc(info.LastMsg, wMsg)

	if selected {
		var selStyle lipgloss.Style
		switch git.StatusGroup(info, now) {
		case git.GroupAttention:
			selStyle = selectedAttentionStyle
		case git.GroupPush:
			selStyle = selectedPushStyle
		case git.GroupStale:
			selStyle = selectedStaleStyle
		default:
			selStyle = selectedOkStyle
		}
		return selStyle.Width(m.width).Render(fmt.Sprintf("▶ %-2s %s", icon, content))
	}
	return rowStyle.Width(m.width).Render(fmt.Sprintf("  %-2s %s", icon, content))
}

// makeHyperlink wraps text in an OSC 8 terminal hyperlink (supported by
// iTerm2, Kitty, WezTerm, etc.). CMD+click opens url in the browser.
func makeHyperlink(url, text string) string {
	return "\033]8;;" + url + "\033\\" + text + "\033]8;;\033\\"
}

// ── Detail content ────────────────────────────────────────────────────────────

func (m model) renderDetailContent() string {
	if len(m.repos) == 0 || m.cursor >= len(m.repos) {
		return ""
	}
	r := m.repos[m.cursor]

	const labelColW = 14
	field := func(k, v string) string {
		kStr := boldStyle.Render(k)
		// use visual width (not byte length) so ANSI codes don't break alignment
		pad := strings.Repeat(" ", max(0, labelColW-lipgloss.Width(kStr)))
		return "  " + kStr + pad + "  " + v + "\n"
	}

	urlStyle := lipgloss.NewStyle().Foreground(colorLightGray)

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(field("Path", urlStyle.Render(r.Path)))
	b.WriteString("\n")
	b.WriteString(field("Branch", r.Branch))

	// Show all remotes so forks display both origin and upstream.
	for i, rem := range r.Remotes {
		label := ""
		if i == 0 {
			label = "Remotes"
		}
		b.WriteString(field(label, cyanStyle.Render(rem.Name)+"  "+urlStyle.Render(rem.URL)))
	}

	if r.NoUpstream {
		b.WriteString(field("Upstream", dimStyle.Render("none")))
	} else {
		var parts []string
		if r.Ahead > 0 {
			parts = append(parts, attentionStyle.Render(fmt.Sprintf("↑%d ahead", r.Ahead)))
		}
		if r.Behind > 0 {
			parts = append(parts, attentionStyle.Render(fmt.Sprintf("↓%d behind", r.Behind)))
		}
		if len(parts) == 0 {
			parts = append(parts, okStyle.Render("✓ in sync"))
		}
		b.WriteString(field("Upstream", strings.Join(parts, "  ")))
	}

	b.WriteString(field("Changes", detailChanges(r)))
	if r.StashCount > 0 {
		b.WriteString(field("Stash", fmt.Sprintf("%d changeset(s)", r.StashCount)))
	}
	b.WriteString(field("Last commit", r.LastRel))

	if r.PRNumber > 0 {
		b.WriteString("\n")
		b.WriteString(field("PR", cyanStyle.Render(fmt.Sprintf("#%d  open", r.PRNumber))))
		b.WriteString(field("", lipgloss.NewStyle().Foreground(colorLightGray).Render(r.PRUrl)))
	}

	// ── Commits behind ────────────────────────────────────────────────────────
	if r.Behind > 0 {
		b.WriteString("\n")
		b.WriteString(boldStyle.Render("  Commits behind") + "\n")
		b.WriteString("  " + strings.Repeat("─", max(0, m.width-4)) + "\n")
		if !m.behindLoaded {
			b.WriteString("  " + m.spinner.View() + "  loading…\n")
		} else if len(m.behindCommits) == 0 {
			b.WriteString(dimStyle.Render("  (none)") + "\n")
		} else {
			for _, c := range m.behindCommits {
				b.WriteString(attentionStyle.Render("  "+c) + "\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(boldStyle.Render("  Recent commits") + "\n")
	b.WriteString("  " + strings.Repeat("─", max(0, m.width-4)) + "\n")

	if !m.commitsLoaded {
		b.WriteString("  " + m.spinner.View() + "  loading…\n")
	} else if len(m.detailCommits) == 0 {
		b.WriteString(dimStyle.Render("  (no commits)") + "\n")
	} else {
		for _, c := range m.detailCommits {
			b.WriteString(textStyle.Render("  "+c) + "\n")
		}
	}
	return b.String()
}

// ── Delete confirm view ───────────────────────────────────────────────────────

func (m model) viewConfirmDelete() string {
	if len(m.repos) == 0 || m.cursor >= len(m.repos) {
		return m.viewList()
	}
	r := m.repos[m.cursor]

	bg := headerBg
	greyS := lipgloss.NewStyle().Background(bg).Foreground(colorLightGray)
	warnS := lipgloss.NewStyle().Background(bg).Foreground(attentionFg).Bold(true)
	cyanS := lipgloss.NewStyle().Background(bg).Foreground(colorCyan)

	var b strings.Builder
	b.WriteString(m.renderHeader3Zone(m.listHeaderLines()))
	b.WriteString(m.sep())

	lines := []string{
		"",
		warnS.Render("  Delete repository from disk?"),
		"",
		greyS.Render("  Path: " + r.Path),
		"",
		greyS.Render("  This permanently deletes the directory and all its contents."),
		greyS.Render("  This action cannot be undone."),
		"",
		"  " + cyanS.Render("<y>") + greyS.Render("  Confirm") + "     " + cyanS.Render("<Esc>") + greyS.Render("  Cancel"),
		"",
	}
	for _, line := range lines {
		b.WriteString(lipgloss.NewStyle().Background(bg).Width(m.width).Render(line) + "\n")
	}

	written := m.headerHeight() + 1 + len(lines)
	for i := written; i < m.height-1; i++ {
		b.WriteString(lipgloss.NewStyle().Background(bg).Width(m.width).Render("") + "\n")
	}
	b.WriteString(m.renderStatusBar())
	return b.String()
}

// ── Settings view ─────────────────────────────────────────────────────────────

func (m model) viewSettings() string {
	bg := headerBg
	fill := lipgloss.NewStyle().Background(bg)
	kStyle := lipgloss.NewStyle().Background(bg).Foreground(colorCyan)
	dStyle := lipgloss.NewStyle().Background(bg).Foreground(colorText)
	dimS := lipgloss.NewStyle().Background(bg).Foreground(staleFg)
	labelStyle := lipgloss.NewStyle().Background(bg).Foreground(colorPurple).Bold(true)
	valStyle := lipgloss.NewStyle().Background(bg).Foreground(colorCyan)
	boldBg := boldStyle.Background(bg)

	var b strings.Builder
	blank := fill.Width(m.width).Render("") + "\n"

	// Top bar
	right := kStyle.Render("<esc>") + dStyle.Render(" Save & Back ")
	gapW := max(1, m.width-lipgloss.Width(right))
	b.WriteString(fill.Width(gapW).Render("") + right + "\n")

	// Title separator
	const settingsLabel = " Settings "
	sw := (m.width - len(settingsLabel)) / 2
	b.WriteString(sepStyle.Render(strings.Repeat("─", sw)) +
		boldBg.Render(settingsLabel) +
		sepStyle.Render(strings.Repeat("─", m.width-sw-len(settingsLabel))) + "\n")

	b.WriteString(blank)
	b.WriteString(fill.Width(m.width).Render(fill.Render("  ")+dimS.Render("Config: ")+dStyle.Render(m.configPath)) + "\n")
	b.WriteString(blank)

	type settingRow struct {
		label string
		desc  string
		kind  string // "int" or "bool"
		val   string
	}
	rows := []settingRow{
		{"Auto-refresh (mins)", "Re-scan all repos on a timer. 0 = disabled.", "int", strconv.Itoa(m.autoRefreshMins)},
		{"Fetch on startup", "Run git fetch in each repo at startup.", "bool", boolStr(m.bootFetch)},
	}

	const labelW = 22
	const valW = 14

	for i, row := range rows {
		cursor := "  "
		if i == m.settingsCursor {
			cursor = "▶ "
		}

		val := row.val
		if m.settingsEditing && i == m.settingsCursor && row.kind == "int" {
			val = "[" + m.settingsEditBuf + "_]"
		}

		lPad := labelW - len(row.label)
		if lPad < 0 {
			lPad = 0
		}
		var lStr string
		if i == m.settingsCursor {
			lStr = labelStyle.Render(row.label) + fill.Render(strings.Repeat(" ", lPad))
		} else {
			lStr = dStyle.Render(row.label) + fill.Render(strings.Repeat(" ", lPad))
		}

		vStr := valStyle.Render(fmt.Sprintf("%-*s", valW, val))
		desc := dimS.Render(row.desc)
		b.WriteString(fill.Width(m.width).Render(fill.Render(cursor)+lStr+fill.Render("  ")+vStr+fill.Render("  ")+desc) + "\n")
	}

	// Info section
	b.WriteString(blank)
	const infoLabel = " Info "
	iw := (m.width - len(infoLabel)) / 2
	b.WriteString(sepStyle.Render(strings.Repeat("─", iw)) +
		boldBg.Render(infoLabel) +
		sepStyle.Render(strings.Repeat("─", m.width-iw-len(infoLabel))) + "\n")
	b.WriteString(blank)

	b.WriteString(fill.Width(m.width).Render(fill.Render("  ")+boldBg.Render("Scan directories:")) + "\n")
	for _, dir := range m.scanDirs {
		b.WriteString(fill.Width(m.width).Render(fill.Render("    ")+dStyle.Render(dir)) + "\n")
	}
	if len(m.scanDirs) == 0 {
		b.WriteString(fill.Width(m.width).Render(fill.Render("    ")+dimS.Render("(none — using current directory)")) + "\n")
	}
	b.WriteString(blank)

	b.WriteString(fill.Width(m.width).Render(fill.Render("  ")+boldBg.Render("Hidden repos:")) + "\n")
	hiddenNames := make([]string, 0, len(m.hidden))
	for name := range m.hidden {
		hiddenNames = append(hiddenNames, name)
	}
	sort.Strings(hiddenNames)
	if len(hiddenNames) == 0 {
		b.WriteString(fill.Width(m.width).Render(fill.Render("    ")+dimS.Render("(none)")) + "\n")
	} else {
		for _, name := range hiddenNames {
			b.WriteString(fill.Width(m.width).Render(fill.Render("    ")+dStyle.Render(name)) + "\n")
		}
	}
	b.WriteString(blank)
	b.WriteString(fill.Width(m.width).Render(fill.Render("  ")+dimS.Render("Manage with: git repos config add/remove/hide/unhide")) + "\n")

	b.WriteString(blank)
	const keysLabel = " Keys "
	kw := (m.width - len(keysLabel)) / 2
	b.WriteString(sepStyle.Render(strings.Repeat("─", kw)) +
		boldBg.Render(keysLabel) +
		sepStyle.Render(strings.Repeat("─", m.width-kw-len(keysLabel))) + "\n")
	b.WriteString(blank)
	settingsKeys := []helpEntry{
		{"enter", "Toggle / Edit"},
		{"esc", "Save & back"},
	}
	for _, sk := range settingsKeys {
		keyStr := kStyle.Render("<" + sk.key + ">")
		pad := max(0, 18-lipgloss.Width(keyStr))
		b.WriteString(fill.Width(m.width).Render(fill.Render("  ")+keyStr+fill.Render(strings.Repeat(" ", pad))+dStyle.Render(sk.desc)) + "\n")
	}

	// Fill remaining lines
	written := strings.Count(b.String(), "\n")
	for i := written; i < m.height; i++ {
		b.WriteString(fill.Width(m.width).Render("") + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func boolStr(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// ── Layout helpers ────────────────────────────────────────────────────────────

func (m model) sep() string {
	return sepStyle.Render(strings.Repeat("─", m.width)) + "\n"
}

// fillBg renders s padded to width with the row background colour.
func fillBg(s string, width int) string {
	return lipgloss.NewStyle().Background(rowBg).Foreground(colorText).Width(width).Render(s)
}

// ── String helpers ────────────────────────────────────────────────────────────

func groupIconStyle(group git.Group) (icon string, style lipgloss.Style) {
	switch group {
	case git.GroupAttention:
		return " !", attentionStyle
	case git.GroupPush:
		return " ↑", pushStyle
	case git.GroupStale:
		return " ·", staleStyle
	default:
		return " ✓", okStyle
	}
}

func detailChanges(r git.RepoInfo) string {
	if r.Staged == 0 && r.Modified == 0 && r.Untracked == 0 {
		return okStyle.Render("clean")
	}
	var parts []string
	if r.Staged > 0 {
		parts = append(parts, attentionStyle.Render(fmt.Sprintf("S:%d staged", r.Staged)))
	}
	if r.Modified > 0 {
		parts = append(parts, attentionStyle.Render(fmt.Sprintf("M:%d modified", r.Modified)))
	}
	if r.Untracked > 0 {
		parts = append(parts, pushStyle.Render(fmt.Sprintf("?:%d untracked", r.Untracked)))
	}
	return strings.Join(parts, "  ")
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
