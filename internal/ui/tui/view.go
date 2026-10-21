package tui

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/keys"
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

// ── Help screen static data ───────────────────────────────────────────────────

// helpEntry is one row of the help as drawn: its keys, what it does, and
// the binding's name; a note has no keys.
type helpEntry struct{ key, desc, name string }

// helpRow is one row of the help or of a header's hints: a binding, by
// name, and what it does there; its label when does is "". A row without
// a name is a note.
type helpRow struct{ name, does string }

var helpNav = []helpRow{
	{keys.MoveUp, "Up"},
	{keys.MoveDown, "Down"},
	{keys.PageDown, "Page Down"},
	{keys.PageUp, "Page Up"},
	{keys.First, "Top"},
	{keys.Last, "Bottom"},
	{keys.Details, "Detail view"},
	{keys.Back, "Back / Close"},
	{keys.SwitchPage, "Repos ⇄ PR page"},
	{keys.FilterPRs, "Filter the PR page"},
}

var helpActions = []helpRow{
	{keys.Search, "Search / filter list"},
	{keys.ViewPR, "View PR in browser"},
	{keys.OpenGitHub, "Open repo in GitHub"},
	{keys.Pull, "Pull repo"},
	{keys.PullAll, "Pull all repos"},
	{keys.Shell, "Open shell in repo"},
	{keys.OpenEditor, "Open repo in editor"},
	{keys.PickEditor, "Open in… (pick one)"},
	{keys.Refresh, "Refresh all"},
	{keys.Delete, "Delete repo from disk"},
	{keys.Settings, "Settings"},
	{keys.Quit, "Quit"},
	{keys.Help, "Help"},
}

var helpSorts = []helpRow{
	{keys.SortStatus, "Status (default)"},
	{keys.SortName, "Name"},
	{keys.SortBranch, "Branch"},
	{keys.SortSync, "Sync"},
	{keys.SortChanges, "Changes"},
	{keys.SortTime, "Time"},
	{keys.SortPR, "PR"},
	{"", "↵ again to reverse ▲▼"},
	{"", ""},
	{"", "In settings:"},
	{keys.Toggle, "Toggle / Edit"},
	{keys.Edit, "Edit a number"},
}

// helpEntries is rows as the help shows them: each binding's keys, and
// what it does.
func (m model) helpEntries(rows []helpRow) []helpEntry {
	out := make([]helpEntry, len(rows))
	for i, r := range rows {
		switch {
		case r.name == "":
			out[i] = helpEntry{"", r.does, ""}
		case r.does == "":
			out[i] = helpEntry{m.keys.capsJoined(r.name, ", "), m.keys.label(r.name), r.name}
		default:
			out[i] = helpEntry{m.keys.capsJoined(r.name, ", "), r.does, r.name}
		}
	}
	return out
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

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = m.windowTitle()
	return v
}

// windowTitle names the app in the terminal's title bar or tab, plus the repo
// being inspected in the detail view. Bubble Tea clears it again on exit.
func (m model) windowTitle() string {
	if m.state == stateDetail && m.cursor < len(m.repos) {
		return "git-repos: " + m.repos[m.cursor].Name
	}
	if m.state == statePRs {
		return "git-repos: PRs"
	}
	return "git-repos"
}

// render draws the whole app as a string.
func (m model) render() string {
	if m.width == 0 {
		return ""
	}
	return m.frame(m.viewContent())
}

func (m model) viewContent() string {
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
	case statePRs:
		return m.viewPRs()
	}
	return ""
}

// frame draws a thin border around the whole app. Full-width separator rows
// meet the border with tees so they read as part of the frame.
func (m model) frame(content string) string {
	sepOpen, sepClose, _ := strings.Cut(sepStyle.Render("─"), "─")
	horiz := strings.Repeat("─", m.width)

	// A view taller than the screen loses its last lines rather than
	// pushing the frame off it.
	lines := strings.Split(content, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}

	var b strings.Builder
	b.WriteString(sepStyle.Render("╭"+horiz+"╮") + "\n")
	for _, line := range lines {
		l, r := "│", "│"
		if strings.HasPrefix(line, sepOpen+"─") && strings.HasSuffix(line, "─"+sepClose) {
			l, r = "├", "┤"
		}
		b.WriteString(sepStyle.Render(l) + line + sepStyle.Render(r) + "\n")
	}
	b.WriteString(sepStyle.Render("╰" + horiz + "╯"))
	return b.String()
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

	msg := "Scanning repos…"
	if m.scanTotal > 0 {
		msg = fmt.Sprintf("Scanning repos… (%d / %d)", m.scanDone, m.scanTotal)
	}

	style := lipgloss.NewStyle().
		Background(colorStatusBarBg).
		Foreground(staleFg)
	b.WriteString(style.Width(m.width).Render(style.Render("  ") + m.spinnerOn(colorStatusBarBg) + style.Render("  "+msg)))
	return b.String()
}

// ── List view ─────────────────────────────────────────────────────────────────

func (m model) viewList() string {
	if m.showPicker {
		return m.viewPicker(m.listHeaderLines())
	}
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
	if m.showPicker {
		return m.viewPicker(m.detailInfoLines())
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

	yoursStyle := lipgloss.NewStyle().Background(bg).Foreground(colorPurple)
	markStyle := lipgloss.NewStyle().Background(bg).Foreground(colorCyan).Bold(true)
	selStyle := lipgloss.NewStyle().Background(bg).Foreground(colorBrightWhite).Bold(true)
	warnStyle := lipgloss.NewStyle().Background(bg).Foreground(attentionFg)
	askStyle := lipgloss.NewStyle().Background(bg).Foreground(colorCyan)

	colW := m.width / 3

	selected := ""
	if sel := helpSelectable(); m.help.row < len(sel) {
		selected = sel[m.help.row]
	}

	// renderEntry draws a row: the selected one marked, keys of yours in
	// the accent.
	renderEntry := func(e helpEntry, w int) string {
		if e.key == "" {
			return fill.Width(w).Render(fill.Render("    ") + noteStyle.Render(e.desc))
		}
		ks := kStyle
		if b, ok := m.keys.binding(e.name); ok && b.Yours {
			ks = yoursStyle
		}
		k := ks.Render(e.key)
		kw := lipgloss.Width(k)
		pad := max(0, 16-kw)
		mark, ds := fill.Render(" "), dStyle
		if e.name == selected {
			mark, ds = markStyle.Render("›"), selStyle
		}
		// Cut what it does to the column, rather than wrap the row.
		desc := ""
		if descW := w - 1 - kw - pad; descW >= 2 {
			desc = trunc(e.desc, descW)
		}
		return fill.Width(w).MaxWidth(w).Render(mark + k + fill.Render(strings.Repeat(" ", pad)) + ds.Render(desc))
	}

	// Top bar: on the left, the selected binding, or the rebinding under
	// way; on the right, the help's own keys.
	var right string
	if m.help.stage == browsing {
		for _, h := range []helpRow{{keys.Rebind, "Rebind"}, {keys.Reset, "Reset"}} {
			if k := m.keys.first(h.name); k != "" {
				right += kStyle.Render(k) + dStyle.Render(" "+h.does+"  ")
			}
		}
		right += kStyle.Render("esc") + dStyle.Render(" Back ")
	} else {
		right = kStyle.Render("esc") + dStyle.Render(" Cancel ")
	}
	left, leftStyle := "", noteStyle
	switch {
	case m.help.stage == recording && m.help.warn:
		left, leftStyle = m.help.note+" · press another key", warnStyle
	case m.help.stage == recording:
		left, leftStyle = "press the new key for "+m.keys.label(selected), askStyle
	case m.help.stage == asking:
		left, leftStyle = m.help.note, askStyle
	case m.help.note != "" && m.help.warn:
		left, leftStyle = m.help.note, warnStyle
	case m.help.note != "":
		left, leftStyle = m.help.note, dStyle
	default:
		if b, ok := m.selectedBinding(); ok {
			left = m.aboutBinding(b)
		}
	}
	leftW := max(0, m.width-lipgloss.Width(right)-1)
	if leftW < 2 {
		left = ""
	} else {
		left = " " + trunc(left, leftW-1)
	}
	topBar := fill.Width(leftW).Render(leftStyle.Render(left)) + fill.Render(" ") + right

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

	nav, actions, sorts := m.helpEntries(helpNav), m.helpEntries(helpActions), m.helpEntries(helpSorts)
	maxRows := max(max(len(nav), len(actions)), len(sorts))
	for i := 0; i < maxRows; i++ {
		var e1, e2, e3 helpEntry
		if i < len(nav) {
			e1 = nav[i]
		}
		if i < len(actions) {
			e2 = actions[i]
		}
		if i < len(sorts) {
			e3 = sorts[i]
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
	iconFgs := []color.Color{attentionFg, pushFg, okFg, staleFg}
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

	// ── Versions section ──────────────────────────────────────────────────────
	type versionEntry struct{ name, ver string }
	var versions []versionEntry
	if m.version != "" {
		grVer := m.version
		if m.updateAvailable {
			grVer += "  " + attentionStyle.Render("update available: "+m.latestVersion)
		}
		versions = append(versions, versionEntry{"gr", grVer})
	}
	if m.gitVersion != "" {
		versions = append(versions, versionEntry{"git", m.gitVersion})
	}
	if m.ghVersion != "" {
		versions = append(versions, versionEntry{"gh", m.ghVersion})
	}

	versionsRows := 0
	if len(versions) > 0 {
		renderSection(" Versions ")
		for _, v := range versions {
			name := nameStyle.Render(fmt.Sprintf("%-*s", nameW, v.name))
			b.WriteString(fill.Width(m.width).Render(fill.Render("  ") + name + fill.Render("  ") + descStyle.Render(v.ver)) + "\n")
		}
		versionsRows = 1 + 1 + len(versions) // blank + sep + content
	}

	// Fill remaining lines
	iconsRows := 1 + 1 + len(helpIcons) // blank + sep + content
	colsRows := 1 + 1 + len(helpCols)   // blank + sep + content
	written := 4 + maxRows + iconsRows + colsRows + versionsRows
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

	leftW := m.width / 3
	rootLabel := hdrPurpleStyle.Render("  Root: ")
	rootMax := max(1, leftW-lipgloss.Width(rootLabel))

	lines := []string{
		rootLabel + hdrInfoStyle.Render(trunc(dir, rootMax)),
	}

	lines = append(lines, m.nonMainSummary())
	lines = append(lines, m.openPRSummary())

	// Status last — detailed breakdown sits under the summary counts
	lines = append(lines, hdrPurpleStyle.Render("  Status: ")+m.repoCountLine())

	return lines
}

// nonMainSummary always renders the "Feature Branches:" label so it stays
// visible during bootup and refresh, showing "…" until the scan finishes.
func (m model) nonMainSummary() string {
	label := hdrPurpleStyle.Render("  Feature Branches: ")
	if m.state == stateScanning && m.scanDone < m.scanTotal {
		return label + hdrDimStyle.Render("…")
	}
	count := 0
	for _, r := range m.repos {
		if !git.IsMainBranch(r.Branch) && r.Branch != "?" && r.Error == "" {
			count++
		}
	}
	if count == 0 {
		return label + hdrDimStyle.Render("—")
	}
	return label + hdrInfoStyle.Render(fmt.Sprintf("%d", count))
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
		return hdrPurpleStyle.Render("  PRs: ") + m.spinnerOn(headerBg) + hdrDimStyle.Render(" loading…")
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

	// acts is rows as hints: each binding's first key and what it does,
	// leaving out one without a key.
	acts := func(rows ...helpRow) []act {
		var out []act
		for _, r := range rows {
			if k := m.keys.first(r.name); k != "" {
				desc := r.does
				if desc == "" {
					desc = m.keys.label(r.name)
				}
				out = append(out, act{k, desc})
			}
		}
		return out
	}

	var col1, col2 []act
	if state == statePRs {
		col1 = acts(helpRow{name: keys.Details}, helpRow{name: keys.ViewPR}, helpRow{keys.FilterPRs, "Filter"}, helpRow{name: keys.Search}, helpRow{name: keys.OpenEditor})
		col2 = acts(helpRow{keys.SwitchPage, "Repos"}, helpRow{keys.Refresh, "Refresh PRs"}, helpRow{keys.Back, "Back"}, helpRow{name: keys.Quit}, helpRow{name: keys.Help})
	} else if state == stateDetail {
		col1 = acts(helpRow{keys.Back, "Back to list"}, helpRow{name: keys.ViewPR}, helpRow{name: keys.OpenGitHub}, helpRow{name: keys.Pull}, helpRow{name: keys.OpenEditor})
		col2 = acts(helpRow{name: keys.Shell}, helpRow{name: keys.Refresh}, helpRow{name: keys.Quit}, helpRow{name: keys.Help})
	} else {
		col1 = acts(helpRow{name: keys.Details}, helpRow{keys.SwitchPage, "PR page"}, helpRow{name: keys.Search}, helpRow{name: keys.ViewPR}, helpRow{name: keys.Pull}, helpRow{name: keys.PullAll})
		col2 = acts(helpRow{name: keys.Shell}, helpRow{name: keys.OpenEditor}, helpRow{name: keys.Delete}, helpRow{name: keys.Refresh}, helpRow{name: keys.Settings}, helpRow{name: keys.Help})
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
	logoColW := m.logoColW()
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

	logoH := len(logoLines)
	totalRows := max(len(midLines), logoH)
	if totalRows <= len(leftLines) {
		totalRows = len(leftLines) + 1
	}
	// Legend anchored to the last row of the header, right above the separator.
	legendIdx := totalRows - 1

	versionS := lipgloss.NewStyle().Foreground(colorPurple).Background(headerBg).Bold(true)
	versionUpdateS := lipgloss.NewStyle().Foreground(attentionFg).Background(headerBg).Bold(true)

	renderLogoCell := func(i int) string {
		if !showLogo {
			return ""
		}
		if i == len(logoLines)-1 && m.version != "" {
			trimmed := strings.TrimRight(logoLines[i], " ")
			vLabel := m.versionLabel()
			vs := versionS
			if m.updateAvailable {
				vs = versionUpdateS
			}
			// Right-align the version, keeping the art's two-column margin.
			gap := max(1, logoColW-4-lipgloss.Width(trimmed)-lipgloss.Width(vLabel))
			return fill.Render("  ") + logoS.Render(trimmed) + fill.Render(strings.Repeat(" ", gap)) + vs.Render(vLabel) + fill.Render("  ")
		}
		if i >= 0 && i < len(logoLines) {
			pad := max(0, logoColW-2-lipgloss.Width(logoLines[i]))
			return fill.Render("  ") + logoS.Render(logoLines[i]) + fill.Render(strings.Repeat(" ", pad))
		}
		return fill.Width(rightW).Render("")
	}

	var b strings.Builder
	for i := 0; i < totalRows; i++ {
		if i == legendIdx {
			legend := m.legendContent()
			row := fill.Inline(true).Width(leftW + midW).MaxWidth(leftW + midW).Render(legend)
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

		// Inline truncates at MaxWidth rather than wrapping onto an extra row.
		cell := fill.Inline(true)
		row := cell.Width(leftW).MaxWidth(leftW).Render(left) + cell.Width(midW).MaxWidth(midW).Render(mid) + renderLogoCell(i)
		b.WriteString(row + "\n")
	}

	return b.String()
}

// versionLabel is the version shown beside the logo: "!" replaces the leading
// "v" when an update is available.
func (m model) versionLabel() string {
	if m.updateAvailable {
		return "!" + strings.TrimPrefix(m.version, "v")
	}
	return m.version
}

// logoColW returns the width of the logo column: the wider of the logo art and
// its last line with the version beside it, plus a two-column margin each side.
func (m model) logoColW() int {
	w := 0
	for _, l := range logoLines {
		w = max(w, lipgloss.Width(l))
	}
	if m.version != "" {
		last := strings.TrimRight(logoLines[len(logoLines)-1], " ")
		w = max(w, lipgloss.Width(last)+1+lipgloss.Width(m.versionLabel()))
	}
	return 2 + w + 2
}

// renderStatusBar renders the bottom status line showing async operation state
// or the active search input.
func (m model) renderStatusBar() string {
	style := lipgloss.NewStyle().
		Background(colorStatusBarBg).
		Foreground(staleFg)

	if m.searching {
		queryStyle := lipgloss.NewStyle().Background(colorStatusBarBg).Foreground(colorCyan)
		content := queryStyle.Render("  /" + m.query() + "█")
		return style.Width(m.width).Render(ansi.Truncate(content, m.width, "…"))
	}

	if m.query() != "" {
		queryStyle := lipgloss.NewStyle().Background(colorStatusBarBg).Foreground(colorCyan)
		hintStyle := lipgloss.NewStyle().Background(colorStatusBarBg).Foreground(colorFaintGray)
		hint := "  (esc to clear)"
		if k := m.keys.first(keys.Search); k != "" {
			hint = "  (" + k + " to edit · esc to clear)"
		}
		content := queryStyle.Render("  /"+m.query()) + hintStyle.Render(hint)
		return style.Width(m.width).Render(ansi.Truncate(content, m.width, "…"))
	}

	var msg string
	busy := true
	switch {
	case m.refreshing:
		msg = "Refreshing…"
		if m.scanTotal > 0 {
			msg = fmt.Sprintf("Refreshing… (%d / %d)", m.scanDone, m.scanTotal)
		}
	case m.prsLoading:
		msg = "Loading PRs…"
	case m.fetchingPR:
		msg = m.statusMsg
	default:
		msg, busy = m.statusMsg, false
	}

	content := style.Render("  ")
	if busy {
		content += m.spinnerOn(colorStatusBarBg) + style.Render("  ")
	}
	if msg != "" {
		content += style.Render(msg)
	}
	return style.Width(m.width).Render(ansi.Truncate(content, m.width, "…"))
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

	wRepo, wBranch, wSync, wMsg := m.colWidths()
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

	wRepo, wBranch, wSync, wMsg := m.colWidths()

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

	fill := lipgloss.NewStyle().Background(rowBg)
	urlStyle := lipgloss.NewStyle().Background(rowBg).Foreground(colorLightGray)

	// line pads s to the full width so the background reaches the right edge.
	line := func(s string) string {
		return s + fill.Render(strings.Repeat(" ", max(0, m.width-lipgloss.Width(s)))) + "\n"
	}
	blank := line("")

	const labelColW = 14
	field := func(k, v string) string {
		kStr := boldStyle.Render(k)
		// use visual width (not byte length) so ANSI codes don't break alignment
		pad := strings.Repeat(" ", max(0, labelColW-lipgloss.Width(kStr)))
		return line(fill.Render("  ") + kStr + fill.Render(pad+"  ") + v)
	}
	heading := func(title string) string {
		return blank +
			line(boldStyle.Render("  "+title)) +
			line(textStyle.Render("  "+strings.Repeat("─", max(0, m.width-4))))
	}
	loading := line(fill.Render("  ") + m.spinnerOn(rowBg) + textStyle.Render("  loading…"))

	var b strings.Builder
	b.WriteString(blank)
	b.WriteString(field("Path", urlStyle.Render(r.Path)))
	b.WriteString(blank)
	b.WriteString(field("Branch", textStyle.Render(r.Branch)))

	// Show all remotes so forks display both origin and upstream.
	for i, rem := range r.Remotes {
		label := ""
		if i == 0 {
			label = "Remotes"
		}
		b.WriteString(field(label, cyanStyle.Render(rem.Name)+fill.Render("  ")+urlStyle.Render(rem.URL)))
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
		b.WriteString(field("Upstream", strings.Join(parts, fill.Render("  "))))
	}

	b.WriteString(field("Changes", detailChanges(r)))
	if len(r.StagedFiles) > 0 {
		b.WriteString(heading("Staged changes"))
		for _, f := range r.StagedFiles {
			b.WriteString(line(attentionStyle.Render("  " + f)))
		}
	}
	if len(r.ModifiedFiles) > 0 {
		b.WriteString(heading("Modified files"))
		for _, f := range r.ModifiedFiles {
			b.WriteString(line(attentionStyle.Render("  " + f)))
		}
	}
	if len(r.UntrackedFiles) > 0 {
		b.WriteString(heading("Untracked files"))
		for _, f := range r.UntrackedFiles {
			b.WriteString(line(pushStyle.Render("  " + f)))
		}
	}
	if r.StashCount > 0 {
		b.WriteString(field("Stash", textStyle.Render(fmt.Sprintf("%d changeset(s)", r.StashCount))))
	}
	b.WriteString(field("Last commit", textStyle.Render(r.LastRel)))

	if r.PRNumber > 0 {
		b.WriteString(blank)
		b.WriteString(field("PR", cyanStyle.Render(fmt.Sprintf("#%d  open", r.PRNumber))))
		b.WriteString(field("", urlStyle.Render(r.PRUrl)))
	}

	// ── Commits behind ────────────────────────────────────────────────────────
	if r.Behind > 0 {
		b.WriteString(heading("Commits behind"))
		if !m.behindLoaded {
			b.WriteString(loading)
		} else if len(m.behindCommits) == 0 {
			b.WriteString(line(dimStyle.Render("  (none)")))
		} else {
			for _, c := range m.behindCommits {
				b.WriteString(line(attentionStyle.Render("  " + c)))
			}
		}
	}

	b.WriteString(heading("Recent commits"))
	if !m.commitsLoaded {
		b.WriteString(loading)
	} else if len(m.detailCommits) == 0 {
		b.WriteString(line(dimStyle.Render("  (no commits)")))
	} else {
		for _, c := range m.detailCommits {
			b.WriteString(line(textStyle.Render("  " + c)))
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
		greyS.Render("  ") + cyanS.Render("<y>") + greyS.Render("  Confirm     ") + cyanS.Render("<Esc>") + greyS.Render("  Cancel"),
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
	right := kStyle.Render("esc") + dStyle.Render(" Save & Back ")
	gapW := max(1, m.width-lipgloss.Width(right))
	b.WriteString(fill.Width(gapW).Render("") + right + "\n")

	// Title separator
	const settingsLabel = " Settings "
	sw := (m.width - len(settingsLabel)) / 2
	b.WriteString(sepStyle.Render(strings.Repeat("─", sw)) +
		boldBg.Render(settingsLabel) +
		sepStyle.Render(strings.Repeat("─", m.width-sw-len(settingsLabel))) + "\n")

	b.WriteString(blank)
	b.WriteString(fill.Width(m.width).Render(ansi.Truncate(fill.Render("  ")+dimS.Render("Config: ")+dStyle.Render(m.configPath), m.width, "…")) + "\n")
	b.WriteString(blank)

	type settingRow struct {
		label string
		desc  string
		kind  string // "int" or "bool"
		val   string
	}
	editorName := "none found"
	if e, ok := m.theDefaultEditor(); ok {
		editorName = e.Name
	}
	rows := []settingRow{
		{"Auto-refresh (mins)", "Re-scan all repos on a timer. 0 = disabled.", "int", strconv.Itoa(m.autoRefreshMins)},
		{"Fetch on startup", "Run git fetch in each repo at startup.", "bool", boolStr(m.bootFetch)},
		{"Default editor", "What " + m.keys.first(keys.OpenEditor) + " opens a repo in; " + m.keys.first(keys.PickEditor) + " picks another. git repos editors lists them.", "choice", editorName},
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
		b.WriteString(fill.Width(m.width).Render(ansi.Truncate(fill.Render(cursor)+lStr+fill.Render("  ")+vStr+fill.Render("  ")+desc, m.width, "…")) + "\n")
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
		b.WriteString(fill.Width(m.width).Render(ansi.Truncate(fill.Render("    ")+dStyle.Render(dir), m.width, "…")) + "\n")
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
	b.WriteString(fill.Width(m.width).Render(ansi.Truncate(fill.Render("  ")+dimS.Render("Manage with: git repos config add/remove/hide/unhide"), m.width, "…")) + "\n")
	b.WriteString(fill.Width(m.width).Render(ansi.Truncate(fill.Render("  ")+dimS.Render("Keys: rebind them in the help ("+m.keys.first(keys.Help)+"), or in config.json's keys"), m.width, "…")) + "\n")

	b.WriteString(blank)
	const keysLabel = " Keys "
	kw := (m.width - len(keysLabel)) / 2
	b.WriteString(sepStyle.Render(strings.Repeat("─", kw)) +
		boldBg.Render(keysLabel) +
		sepStyle.Render(strings.Repeat("─", m.width-kw-len(keysLabel))) + "\n")
	b.WriteString(blank)
	// Without the help's brackets, a comma keeps several keys apart.
	settingsKeys := []helpEntry{
		{key: m.keys.capsJoined(keys.Toggle, ", "), desc: m.keys.label(keys.Toggle)},
		{key: m.keys.capsJoined(keys.Back, ", "), desc: "Save & back"},
	}
	for _, sk := range settingsKeys {
		keyStr := kStyle.Render(sk.key)
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

// spinnerOn renders the spinner frame over bg. The spinner's own style only
// sets a foreground, so without this the frame sits on the terminal background.
func (m model) spinnerOn(bg color.Color) string {
	s := m.spinner
	s.Style = s.Style.Background(bg)
	return s.View()
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
	return strings.Join(parts, lipgloss.NewStyle().Background(rowBg).Render("  "))
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
