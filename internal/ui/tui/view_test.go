package tui

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/samsar/git-repos/internal/git"
)

// Every view must fill the whole terminal and give every cell an explicit
// background, otherwise the terminal's own background shows through.
func TestViewsPaintEveryCell(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	dirty := repo("catio-harness", "fix/drain-handoff", "fix(core): make the drain park non-reentrant")
	dirty.Behind = 1
	dirty.Staged, dirty.StagedFiles = 1, []string{"cmd/root.go"}
	dirty.Modified, dirty.ModifiedFiles = 1, []string{"internal/ui/tui/view.go"}
	dirty.Untracked, dirty.UntrackedFiles = 1, []string{"notes.txt"}
	dirty.StashCount = 2
	dirty.LastRel = "6 minutes ago"
	dirty.PRNumber, dirty.PRUrl = 263, "https://github.com/catio-tech/catio-harness/pull/263"
	dirty.Remotes = []git.Remote{{Name: "origin", URL: "git@github.com:catio-tech/catio-harness.git"}}

	noUpstream := repo("scratch", "main", "wip")
	noUpstream.NoUpstream = true

	base := func() model {
		s := spinner.New()
		s.Spinner = spinner.Dot
		return model{
			width:   120,
			height:  40,
			state:   stateList,
			spinner: s,
			version: "v1.1.8",
			repos:   []git.RepoInfo{dirty, noUpstream},
		}
	}
	detail := func(cursor int, loaded bool) model {
		m := base()
		m.cursor = cursor
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(model)
		if loaded {
			m.commitsLoaded, m.detailCommits = true, []string{
				"1071823  6 minutes ago  fix(core): drain",
				"6fc937c  20 hours ago  " + strings.Repeat("a commit subject longer than the terminal is wide ", 4),
			}
			m.behindLoaded, m.behindCommits = true, []string{"1b7164e  9 minutes ago  feat!: catalogue v2"}
			m.detailVP.SetContent(m.renderDetailContent())
		}
		return m
	}

	cases := map[string]func() model{
		"list": base,
		"list refreshing": func() model {
			m := base()
			m.refreshing, m.scanDone, m.scanTotal = true, 3, 10
			return m
		},
		"list loading PRs": func() model {
			m := base()
			m.prsLoading = true
			return m
		},
		"list status message": func() model {
			m := base()
			m.statusMsg = "pulled catio-harness"
			return m
		},
		"list searching": func() model {
			m := base()
			m.searching, m.searchQuery = true, "cat"
			return m
		},
		"list delete confirm": func() model {
			m := base()
			m.showDeleteConfirm = true
			return m
		},
		"scanning": func() model {
			m := base()
			m.state, m.scanDone, m.scanTotal = stateScanning, 3, 10
			return m
		},
		"detail loading":          func() model { return detail(0, false) },
		"detail loaded":           func() model { return detail(0, true) },
		"detail no upstream":      func() model { return detail(1, true) },
		"detail short terminal":   func() model { m := detail(0, true); m.height = 20; return resize(m) },
		"help":                    func() model { m := base(); m.showHelp = true; return m },
		"settings":                func() model { m := base(); m.state = stateSettings; return m },
		"detail after resize":     func() model { m := detail(0, true); m.width, m.height = 150, 50; return resize(m) },
		"list after resize small": func() model { m := base(); m.width, m.height = 130, 25; return resize(m) },
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			m := build()
			out := m.View()
			rows := strings.Split(out, "\n")
			if len(rows) != m.height {
				t.Errorf("rendered %d rows, want %d (the terminal height)", len(rows), m.height)
			}
			for i, row := range rows {
				if w := lipgloss.Width(row); w < m.width {
					t.Errorf("row %d is %d cells wide, want %d: %q", i, w, m.width, plain(row))
				}
				if n := unpaintedCells(row); n > 0 {
					t.Errorf("row %d has %d cells without a background: %q", i, n, plain(row))
				}
			}
		})
	}
}

// resize feeds the model's current size back through Update, as a terminal
// resize would.
func resize(m model) model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return next.(model)
}

// unpaintedCells counts the printable characters in row that are drawn while
// no background colour is active.
func unpaintedCells(row string) int {
	bg, n := false, 0
	for i := 0; i < len(row); {
		if row[i] == '\x1b' && i+1 < len(row) {
			switch row[i+1] {
			case '[': // CSI … final byte
				j := i + 2
				for j < len(row) && (row[j] < 0x40 || row[j] > 0x7e) {
					j++
				}
				if j < len(row) && row[j] == 'm' {
					bg = applySGR(bg, row[i+2:j])
				}
				i = j + 1
				continue
			case ']': // OSC … BEL or ST
				j := i + 2
				for j < len(row) && row[j] != '\a' && !(row[j] == '\x1b' && j+1 < len(row) && row[j+1] == '\\') {
					j++
				}
				if j < len(row) && row[j] == '\a' {
					i = j + 1
				} else {
					i = j + 2
				}
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(row[i:])
		if !bg {
			n++
		}
		i += size
	}
	return n
}

// applySGR returns whether a background colour is active after the SGR
// parameters in params are applied.
func applySGR(bg bool, params string) bool {
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		switch p := ps[i]; p {
		case "", "0", "49":
			bg = false
		case "38", "48":
			if i+1 < len(ps) && ps[i+1] == "5" {
				i += 2
			} else if i+1 < len(ps) && ps[i+1] == "2" {
				i += 4
			}
			if p == "48" {
				bg = true
			}
		default:
			if c, err := strconv.Atoi(p); err == nil && (c >= 40 && c <= 47 || c >= 100 && c <= 107) {
				bg = true
			}
		}
	}
	return bg
}

// plain strips escape sequences so failure messages are readable.
func plain(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && (s[i+1] == '[' || s[i+1] == ']') {
			j := i + 2
			for j < len(s) && !(s[i+1] == '[' && s[j] >= 0x40 && s[j] <= 0x7e) && !(s[i+1] == ']' && (s[j] == '\a' || s[j] == '\\')) {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
