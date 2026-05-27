package tui

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/samsar/git-repos/internal/config"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/version"
)

// ── View states ───────────────────────────────────────────────────────────────

type viewState int

const (
	stateScanning viewState = iota
	stateList
	stateDetail
	stateSettings
)

// ── Messages ──────────────────────────────────────────────────────────────────

type scanStartedMsg struct {
	ch    <-chan git.ScanResult
	total int
}
type repoScannedMsg git.ScanResult
type scanDoneMsg struct{}
type prsLoadedMsg []git.RepoInfo
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

const (
	colorCyan        lipgloss.Color = "51"  // #00ffff - cyan, accent / interactive elements
	colorPurple      lipgloss.Color = "135" // #af5fff - medium orchid, header labels and logo
	colorNearBlack   lipgloss.Color = "232" // #080808 - near black, text on coloured row backgrounds
	colorText        lipgloss.Color = "252" // #d0d0d0 - light silver, primary foreground
	colorBrightWhite lipgloss.Color = "15"  // #ffffff - white
	colorStatusBarBg lipgloss.Color = "233" // #121212 - very dark grey, status bar background
	colorDarkGray    lipgloss.Color = "237" // #3a3a3a - dark grey, separators
	colorDimGray     lipgloss.Color = "244" // #808080 - dim grey, selected-stale row background
	colorFaintGray   lipgloss.Color = "245" // #8a8a8a - faint grey, legend text
	colorSubduedGray lipgloss.Color = "246" // #949494 - subdued grey, column headers and help descriptions
	colorLightGray   lipgloss.Color = "248" // #a8a8a8 - light grey, URLs and paths
)

var (
	// All views share a uniform near-black background. The status bar overrides
	// this with its own darker grey at the bottom of the screen.
	headerBg lipgloss.Color = "232"
	rowBg                   = headerBg

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
	boldStyle = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	textStyle = lipgloss.NewStyle().Foreground(colorText)
	dimStyle  = lipgloss.NewStyle().Foreground(staleFg)
	cyanStyle = lipgloss.NewStyle().Foreground(colorCyan)
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
	width  int
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
	fetchingPR    bool
	refreshing    bool
	refreshRepos  []git.RepoInfo

	// search
	searching   bool
	searchQuery string

	// ui
	spinner           spinner.Model
	showHelp          bool
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

func New(dirs []string, doFetch, noPRs bool, hidden map[string]bool, autoRefreshMins int, bootFetch bool, configPath, version string) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)
	return model{
		scanDirs:        dirs,
		doFetch:         doFetch,
		noPRs:           noPRs,
		state:           stateScanning,
		spinner:         s,
		sortCol:         git.SortStatus,
		hidden:          hidden,
		autoRefreshMins: autoRefreshMins,
		bootFetch:       bootFetch,
		configPath:      configPath,
		version:         normalizeVersion(version),
		gitVersion:      detectCLIVersion("git", "--version"),
		ghVersion:       detectCLIVersion("gh", "--version"),
	}
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

// saveConfig persists current settings to disk.
func (m model) saveConfig() {
	hidden := make([]string, 0, len(m.hidden))
	for name := range m.hidden {
		hidden = append(hidden, name)
	}
	sort.Strings(hidden)
	cfg := &config.Config{
		Dirs:            m.scanDirs,
		Hidden:          hidden,
		AutoRefreshMins: m.autoRefreshMins,
		BootFetch:       m.bootFetch,
	}
	_ = config.Save(cfg)
}

// colWidths returns dynamic widths for the three resizable columns: BRANCH, SYNC,
// and LAST MSG. Priority order when space is tight: protect BRANCH and SYNC first
// (down to their minimums); LAST MSG shrinks first and grows last.
//
// baseFixed accounts for every non-resizable byte in a rendered row:
//
//	prefix(5) + REPO+sp(26) + CHG+sp(15) + STASH+sp(6) + WHEN+sp(13)
//	+ PR+sp(7) + branch-trailing-sp(1) + sync-trailing-sp(1) = 74
func (m model) colWidths() (branch, sync, msg int) {
	const (
		baseFixed  = 74
		branchMin  = 22
		branchPref = 28
		syncMin    = 9  // "no-remote" is 9 chars
		syncPref   = 12
		msgMin     = 12
	)

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

	// Everything remaining goes to LAST MSG.
	msg += leftover
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

func loadPRsCmd(repos []git.RepoInfo) tea.Cmd {
	return func() tea.Msg {
		cp := make([]git.RepoInfo, len(repos))
		copy(cp, repos)
		git.FetchAndMatchPRs(cp)
		return prsLoadedMsg(cp)
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
func Run(dirs []string, doFetch, fetchPRs bool, hidden map[string]bool, autoRefreshMins int, bootFetch bool, configPath, version string) error {
	m := New(dirs, doFetch, !fetchPRs, hidden, autoRefreshMins, bootFetch, configPath, version)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
