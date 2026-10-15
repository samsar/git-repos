package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/samsar/git-repos/internal/config"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/keys"
	"github.com/samsar/git-repos/internal/launch"
	"github.com/samsar/git-repos/internal/version"
)

// ── View states ───────────────────────────────────────────────────────────────

type viewState int

const (
	stateScanning viewState = iota
	stateList
	stateDetail
	stateSettings
	statePRs
)

// ── Messages ──────────────────────────────────────────────────────────────────

type scanStartedMsg struct {
	ch    <-chan git.ScanResult
	total int
}
type repoScannedMsg git.ScanResult
type scanDoneMsg struct{}

// prsLoadedMsg is the PRs, yours and those asking for your review, which
// the handler matches to the repos as they are by then; or why they
// couldn't be looked up.
type prsLoadedMsg struct {
	prs git.PRs
	err error
}
type commitsLoadedMsg []string
type behindCommitsLoadedMsg []string
type fetchDoneMsg git.RepoInfo
type pullAllDoneMsg []git.RepoInfo
type autoRefreshMsg struct{}
type shellReturnMsg struct{}
type deleteRepoDoneMsg struct{ name string }
type errMsg struct{ err error }
type ghCheckMsg struct{ unavailable bool }
type versionCheckMsg struct{ latest string }

// ── Colour palette ────────────────────────────────────────────────────────────
//
// All colours are explicit 256-colour values so the TUI looks correct
// regardless of the terminal's own background colour.

var (
	colorCyan        = lipgloss.Color("51")  // #00ffff - cyan, accent / interactive elements
	colorPurple      = lipgloss.Color("135") // #af5fff - medium orchid, header labels and logo
	colorNearBlack   = lipgloss.Color("232") // #080808 - near black, text on coloured row backgrounds
	colorText        = lipgloss.Color("252") // #d0d0d0 - light silver, primary foreground
	colorBrightWhite = lipgloss.Color("15")  // #ffffff - white
	colorStatusBarBg = lipgloss.Color("233") // #121212 - very dark grey, status bar background
	colorDarkGray    = lipgloss.Color("237") // #3a3a3a - dark grey, separators
	colorDimGray     = lipgloss.Color("244") // #808080 - dim grey, selected-stale row background
	colorFaintGray   = lipgloss.Color("245") // #8a8a8a - faint grey, legend text
	colorSubduedGray = lipgloss.Color("246") // #949494 - subdued grey, column headers and help descriptions
	colorLightGray   = lipgloss.Color("248") // #a8a8a8 - light grey, URLs and paths
)

var (
	// All views share a uniform near-black background. The status bar overrides
	// this with its own darker grey at the bottom of the screen.
	headerBg = lipgloss.Color("232")
	rowBg    = headerBg

	// Status colours — applied as foreground to the entire row
	attentionFg = lipgloss.Color("203") // soft coral
	pushFg      = lipgloss.Color("75")  // cornflower blue
	okFg        = lipgloss.Color("82")  // bright green
	staleFg     = lipgloss.Color("241") // medium gray

	attentionStyle = lipgloss.NewStyle().Foreground(attentionFg).Background(rowBg)
	pushStyle      = lipgloss.NewStyle().Foreground(pushFg).Background(rowBg)
	okStyle        = lipgloss.NewStyle().Foreground(okFg).Background(rowBg)
	staleStyle     = lipgloss.NewStyle().Foreground(staleFg).Background(rowBg)

	// selectedStyles uses the status colour as the row background, with near-black
	// text — matching the k9s cursor style.
	selectedAttentionStyle = lipgloss.NewStyle().Background(attentionFg).Foreground(colorNearBlack).Bold(true)
	selectedPushStyle      = lipgloss.NewStyle().Background(pushFg).Foreground(colorNearBlack).Bold(true)
	selectedOkStyle        = lipgloss.NewStyle().Background(okFg).Foreground(colorNearBlack).Bold(true)
	selectedStaleStyle     = lipgloss.NewStyle().Background(colorDimGray).Foreground(colorNearBlack).Bold(true)

	// Header
	hdrInfoStyle = lipgloss.NewStyle().
			Background(headerBg).
			Foreground(colorText)
	hdrDimStyle = lipgloss.NewStyle().
			Background(headerBg).
			Foreground(staleFg)
	hdrKeyStyle = lipgloss.NewStyle().
			Background(headerBg).
			Foreground(colorText).
			Bold(true).
			Padding(0, 1)
	hdrKeyDescStyle = lipgloss.NewStyle().
			Background(headerBg).
			Foreground(colorSubduedGray)
	hdrLegendTextStyle = lipgloss.NewStyle().
				Background(headerBg).
				Foreground(colorFaintGray)
	hdrPurpleStyle = lipgloss.NewStyle().
			Background(headerBg).
			Foreground(colorPurple)

	// Column header
	colHdrStyle = lipgloss.NewStyle().
			Background(headerBg).
			Foreground(colorSubduedGray).
			Bold(true)
	colHdrSortedStyle = lipgloss.NewStyle().
				Background(headerBg).
				Foreground(colorCyan).
				Bold(true)

	// Misc
	sepStyle  = lipgloss.NewStyle().Foreground(colorDarkGray).Background(headerBg)
	boldStyle = lipgloss.NewStyle().Bold(true).Foreground(colorText).Background(rowBg)
	textStyle = lipgloss.NewStyle().Foreground(colorText).Background(rowBg)
	dimStyle  = lipgloss.NewStyle().Foreground(staleFg).Background(rowBg)
	cyanStyle = lipgloss.NewStyle().Foreground(colorCyan).Background(rowBg)
)

// ── Column widths ─────────────────────────────────────────────────────────────
//
// Widths shared with the plain renderer come from the git package.
// wBranch and wSync are wider here because the TUI is full-width adaptive.

const (
	wST    = git.ColWidthStatus
	wRepo  = git.ColWidthRepo
	wChg   = git.ColWidthChanges
	wStash = git.ColWidthStash
	wWhen  = git.ColWidthWhen
	wPR    = git.ColWidthPR
)

// ── Model ─────────────────────────────────────────────────────────────────────

type model struct {
	// config
	scanDirs        []string
	doFetch         bool
	noPRs           bool
	autoRefreshMins int
	bootFetch       bool
	configPath      string

	// versions
	version    string
	gitVersion string
	ghVersion  string

	// state
	state     viewState
	repos     []git.RepoInfo
	scanTotal int
	scanDone  int

	// list navigation
	cursor int
	offset int
	width  int // content area, inside the border (terminal size minus 2)
	height int

	// sorting
	sortCol  git.SortColumn
	sortDesc bool

	// detail
	detailVP      viewport.Model
	detailCommits []string
	commitsLoaded bool
	behindCommits []string
	behindLoaded  bool

	// async
	resultCh      <-chan git.ScanResult
	prsLoading    bool
	prsEverLoaded bool
	prs           git.PRs // yours and those asking for your review, as last looked up
	prsErr        error   // why the last lookup failed, if it did
	fetchingPR    bool
	refreshing    bool
	refreshRepos  []git.RepoInfo

	// PR page
	prCursor int // in displayPRs()
	prOffset int // in prLines()
	prFilter prFilter
	prQuery  string // the PR page's search, apart from the repos'
	// settingsFrom is the page the settings go back to: the list, or the
	// detail view.
	settingsFrom viewState
	// detailFrom is the page the detail view goes back to: the list, or
	// the PR page, when listCursor is the list's cursor to put back.
	detailFrom viewState
	listCursor int
	// detailPath is the repo the detail view shows, by path, which the
	// settings opened from it go back to.
	detailPath string

	// search
	searching   bool
	searchQuery string

	// editors
	editors       []launch.Editor
	defaultEditor string // by name; "" for launch.Pick's choice
	opener        launch.Opener
	openerOK      bool
	showPicker    bool
	pickerCursor  int

	// ui
	keys              keymap
	keyProblems       []string // config.json's keys' problems, the defaults in use
	spinner           spinner.Model
	showHelp          bool
	help              keysHelp
	showDeleteConfirm bool

	// settings
	settingsCursor  int
	settingsEditing bool
	settingsEditBuf string

	statusMsg string

	hidden          map[string]bool
	ghUnavailable   bool
	latestVersion   string
	updateAvailable bool
}

// Options is how the TUI runs: what it scans, and what config.json says.
type Options struct {
	Dirs            []string
	Fetch           bool // git fetch in every repo as it scans
	PRs             bool // look up open GitHub PRs
	Hidden          map[string]bool
	AutoRefreshMins int
	BootFetch       bool
	ConfigPath      string
	Version         string
	// Bindings are the keys, and KeyProblems config.json's keys' problems,
	// the default keys in use when it has any.
	Bindings    []keys.Binding
	KeyProblems []string
	// Editors are the editors a repo opens in (launch.Available), and
	// DefaultEditor the one e opens, by name; "" for launch.Pick's choice.
	Editors       []launch.Editor
	DefaultEditor string
	// EditorProblems are config.json's editors' problems, which leave
	// them out.
	EditorProblems []string
	// Opener opens terminal windows, when OpenerOK.
	Opener   launch.Opener
	OpenerOK bool
}

func New(o Options) model {
	statusMsg := ""
	if n := len(o.KeyProblems); n > 0 {
		statusMsg = fmt.Sprintf("config.json's keys have %d problem(s), so the default keys are in use: run git repos keys check", n)
	} else if len(o.EditorProblems) > 0 {
		statusMsg = "config.json's editors are left out: " + o.EditorProblems[0] + " (git repos editors lists them)"
	}
	return model{
		keys:            keymap{bindings: o.Bindings},
		keyProblems:     o.KeyProblems,
		editors:         o.Editors,
		defaultEditor:   o.DefaultEditor,
		opener:          o.Opener,
		openerOK:        o.OpenerOK,
		statusMsg:       statusMsg,
		scanDirs:        o.Dirs,
		doFetch:         o.Fetch,
		noPRs:           !o.PRs,
		state:           stateScanning,
		spinner:         newSpinner(),
		sortCol:         git.SortStatus,
		hidden:          o.Hidden,
		autoRefreshMins: o.AutoRefreshMins,
		bootFetch:       o.BootFetch,
		configPath:      o.ConfigPath,
		version:         normalizeVersion(o.Version),
		gitVersion:      detectCLIVersion("git", "--version"),
		ghVersion:       detectCLIVersion("gh", "--version"),
	}
}

func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)
	return s
}

func normalizeVersion(v string) string {
	if v != "" && !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

func detectCLIVersion(name string, args ...string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return ""
	}
	// Parse first line: "git version 2.39.0" or "gh version 2.40.1 (2023-12-13)"
	line := strings.SplitN(string(out), "\n", 2)[0]
	fields := strings.Fields(line)
	for _, f := range fields {
		if len(f) > 0 && f[0] >= '0' && f[0] <= '9' {
			return f
		}
	}
	return strings.TrimSpace(line)
}

// saveConfig persists the settings the TUI changes to disk, keeping
// everything else config.json says (your keys, and dirs given as
// arguments rather than from the file).
func (m model) saveConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.AutoRefreshMins = m.autoRefreshMins
	cfg.BootFetch = m.bootFetch
	cfg.DefaultEditor = m.defaultEditor
	return config.Save(cfg)
}

// colWidths returns dynamic widths for the three resizable columns: BRANCH, SYNC,
// and LAST MSG. Priority order when space is tight: protect BRANCH and SYNC first
// (down to their minimums); LAST MSG shrinks first and grows last. BRANCH's
// preferred width follows the longest branch name on screen (up to branchCap),
// so long names like "feat/otel-instrument-and-repoint" show in full when
// there's room instead of being clipped at a fixed width.
//
// baseFixed accounts for every non-resizable byte in a rendered row:
//
//	prefix(5) + REPO+sp(26) + CHG+sp(15) + STASH+sp(6) + WHEN+sp(13)
//	+ PR+sp(7) + branch-trailing-sp(1) + sync-trailing-sp(1) = 74
func (m model) colWidths() (branch, sync, msg int) {
	const (
		baseFixed  = 74
		branchMin  = 22
		branchCap  = 60 // upper bound so one long branch can't starve LAST MSG
		syncMin    = 9  // "no-remote" is 9 chars
		syncPref   = 12
		msgMin     = 12
		msgFloor   = 6 // LAST MSG yields below msgMin (down to here) so a long BRANCH fits
	)

	// branchPref tracks the longest branch name present (clamped to a sane
	// range) so full names show when there's room, instead of a fixed cap.
	branchPref := branchMin
	for i := range m.repos {
		if w := lipgloss.Width(m.repos[i].Branch); w > branchPref {
			branchPref = w
		}
	}
	branchPref = min(branchPref, branchCap)

	avail := m.width - baseFixed

	branch = branchMin
	sync = syncMin
	msg = msgMin

	leftover := avail - branchMin - syncMin - msgMin
	if leftover <= 0 {
		return
	}

	// Grow branch to preferred first.
	grow := min(leftover, branchPref-branchMin)
	branch += grow
	leftover -= grow

	// Grow sync to preferred next.
	grow = min(leftover, syncPref-syncMin)
	sync += grow
	leftover -= grow

	if leftover > 0 {
		// Spare room: give it to LAST MSG.
		msg += leftover
		return
	}

	// No spare room but BRANCH is still short of its preferred width (a long
	// branch on a slightly-too-narrow terminal). Borrow from LAST MSG down to
	// msgFloor so the full branch name shows — LAST MSG shrinks first.
	if branch < branchPref {
		borrow := min(branchPref-branch, msg-msgFloor)
		branch += borrow
		msg -= borrow
	}
	return
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, startScanCmd(m.scanDirs, m.doFetch), checkVersionCmd}
	if !m.noPRs {
		cmds = append(cmds, checkGHCmd)
	}
	if m.autoRefreshMins > 0 {
		cmds = append(cmds, autoRefreshTickCmd(m.autoRefreshMins))
	}
	return tea.Batch(cmds...)
}

// checkGHCmd verifies gh is installed and authenticated by running
// "gh auth token". Exits 0 only when both conditions are met.
func checkGHCmd() tea.Msg {
	path, err := exec.LookPath("gh")
	if err != nil {
		return ghCheckMsg{unavailable: true}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = exec.CommandContext(ctx, path, "auth", "token").Run()
	return ghCheckMsg{unavailable: err != nil}
}

func checkVersionCmd() tea.Msg {
	return versionCheckMsg{latest: version.FetchLatest()}
}

// ── tea.Cmd helpers ───────────────────────────────────────────────────────────

func startScanCmd(dirs []string, doFetch bool) tea.Cmd {
	return func() tea.Msg {
		ch, total, err := git.ScanAllAsync(dirs, doFetch)
		if err != nil {
			return errMsg{err}
		}
		return scanStartedMsg{ch: ch, total: total}
	}
}

func waitForScanCmd(ch <-chan git.ScanResult) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return scanDoneMsg{}
		}
		return repoScannedMsg(r)
	}
}

// loadPRsCmd looks up the PRs. It touches no repo: the handler matches
// the PRs to the repos as they are when it answers, which a pull or a
// refresh meanwhile may have changed.
func loadPRsCmd() tea.Cmd {
	return func() tea.Msg {
		prs, err := git.FetchPRs()
		return prsLoadedMsg{prs: prs, err: err}
	}
}

func loadCommitsCmd(path string) tea.Cmd {
	return func() tea.Msg {
		return commitsLoadedMsg(git.RecentCommits(path, 15))
	}
}

func loadBehindCommitsCmd(path string) tea.Cmd {
	return func() tea.Msg {
		return behindCommitsLoadedMsg(git.CommitsBehind(path, 15))
	}
}

func fetchRepoCmd(path string) tea.Cmd {
	return func() tea.Msg {
		git.RunCmd([]string{"git", "pull", "--ff-only", "--quiet", "--recurse-submodules"}, path, 60*time.Second)
		return fetchDoneMsg(git.CollectRepo(path, false))
	}
}

func refreshSingleRepoCmd(path string) tea.Cmd {
	return func() tea.Msg {
		return fetchDoneMsg(git.CollectRepo(path, true))
	}
}

func pullAllCmd(repos []git.RepoInfo) tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8)
		updated := make([]git.RepoInfo, len(repos))
		for i, r := range repos {
			updated[i] = r // keep as-is unless we pull
			if r.Behind == 0 {
				continue
			}
			wg.Add(1)
			go func(i int, r git.RepoInfo) {
				defer wg.Done()
				sem <- struct{}{}
				git.RunCmd([]string{"git", "pull", "--ff-only", "--quiet", "--recurse-submodules"}, r.Path, 60*time.Second)
				updated[i] = git.CollectRepo(r.Path, false)
				<-sem
			}(i, r)
		}
		wg.Wait()
		return pullAllDoneMsg(updated)
	}
}

func autoRefreshTickCmd(mins int) tea.Cmd {
	return tea.Tick(time.Duration(mins)*time.Minute, func(t time.Time) tea.Msg {
		return autoRefreshMsg{}
	})
}

func openShellCmd(path string) tea.Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	// Clear the screen before launching the shell so it doesn't look like
	// we're shelling into an existing session.
	c := exec.Command("sh", "-c", "clear; exec "+shell)
	c.Dir = path
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return shellReturnMsg{}
	})
}

func deleteRepoDirCmd(info git.RepoInfo) tea.Cmd {
	return func() tea.Msg {
		if err := os.RemoveAll(info.Path); err != nil {
			return errMsg{err}
		}
		return deleteRepoDoneMsg{name: info.Name}
	}
}

func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		openBrowser(url)
		return nil
	}
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Start()
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}

// Run starts the full-screen TUI program.
func Run(o Options) error {
	m := New(o)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
