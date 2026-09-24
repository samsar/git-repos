package tui

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/samsar/git-repos/internal/git"
)

// Every view must fill the whole terminal inside a border, and give every cell
// an explicit background, otherwise the terminal's own background shows through.
func TestViewsFillTerminal(t *testing.T) {
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
		return resize(model{
			state:   stateList,
			spinner: newSpinner(),
			version: "v1.1.8",
			repos:   []git.RepoInfo{dirty, noUpstream},
		}, 120, 40)
	}
	detail := func(cursor int, loaded bool) model {
		m := base()
		m.cursor = cursor
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
		"detail short terminal":   func() model { return resize(detail(0, true), 120, 20) },
		"help":                    func() model { m := base(); m.showHelp = true; return m },
		"settings":                func() model { m := base(); m.state = stateSettings; return m },
		"detail after resize":     func() model { return resize(detail(0, true), 150, 50) },
		"list after resize small": func() model { return resize(base(), 130, 25) },
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			m := build()
			// The border takes one row / column on each side of the content area.
			termW, termH := m.width+2, m.height+2
			rows := strings.Split(m.View().Content, "\n")
			if len(rows) != termH {
				t.Errorf("rendered %d rows, want %d (the terminal height)", len(rows), termH)
			}
			for i, row := range rows {
				if w := lipgloss.Width(row); w != termW {
					t.Errorf("row %d is %d cells wide, want %d: %q", i, w, termW, plain(row))
				}
				if n := unpaintedCells(row); n > 0 {
					t.Errorf("row %d has %d cells without a background: %q", i, n, plain(row))
				}
			}
			checkFrame(t, rows)
		})
	}
}

// checkFrame asserts rows are wrapped in the rounded border, with tees where a
// full-width separator meets it.
func checkFrame(t *testing.T, rows []string) {
	t.Helper()
	for i, row := range rows {
		r := []rune(plain(row))
		if len(r) < 2 {
			t.Errorf("row %d is too short to carry a border: %q", i, string(r))
			continue
		}
		inner := string(r[1 : len(r)-1])
		want := "││"
		switch {
		case i == 0:
			want = "╭╮"
		case i == len(rows)-1:
			want = "╰╯"
		case strings.HasPrefix(inner, "─") && strings.HasSuffix(inner, "─"):
			want = "├┤"
		}
		if got := string(r[0]) + string(r[len(r)-1]); got != want {
			t.Errorf("row %d edges = %q, want %q: %q", i, got, want, string(r))
		}
	}
}

// The version beside the logo keeps a margin from the border rather than
// running straight into it.
func TestVersionPaddedFromBorder(t *testing.T) {
	for _, version := range []string{"v1.1.8", "v1.10.12"} {
		for _, update := range []bool{false, true} {
			m := resize(model{state: stateList, spinner: newSpinner(), version: version, updateAvailable: update}, 120, 40)
			label := m.versionLabel()
			found := false
			for _, row := range strings.Split(plain(m.View().Content), "\n") {
				if !strings.Contains(row, label) {
					continue
				}
				found = true
				if !strings.HasSuffix(row, label+"  │") {
					t.Errorf("version %q not padded from the border: %q", label, row)
				}
			}
			if !found {
				t.Errorf("version %q not rendered", label)
			}
		}
	}
}

// resize sends the model a w×h terminal size, as a terminal resize would.
func resize(m model, w, h int) model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
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
