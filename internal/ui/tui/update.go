package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/samsar/git-repos/internal/git"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.state == stateDetail {
			m.detailVP.Width = m.width
			m.detailVP.Height = m.detailVPHeight()
			m.detailVP.SetContent(m.renderDetailContent())
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case scanStartedMsg:
		m.resultCh = msg.ch
		m.scanTotal = msg.total
		if msg.total == 0 {
			m.state = stateList
			m.statusMsg = "no repos found"
			return m, nil
		}
		return m, waitForScanCmd(m.resultCh)

	case repoScannedMsg:
		if !m.hidden[msg.Repo.Name] {
			m.repos = append(m.repos, msg.Repo)
		}
		m.scanDone = msg.Done
		m.scanTotal = msg.Total
		return m, waitForScanCmd(m.resultCh)

	case scanDoneMsg:
		git.SortReposByCol(m.repos, m.sortCol, m.sortDesc)
		m.state = stateList
		if !m.noPRs {
			m.prsLoading = true
			return m, tea.Batch(m.spinner.Tick, loadPRsCmd(m.repos))
		}
		return m, nil

	case prsLoadedMsg:
		m.repos = []git.RepoInfo(msg)
		m.prsLoading = false
		// re-apply sort after PR data is added
		git.SortReposByCol(m.repos, m.sortCol, m.sortDesc)
		return m, nil

	case commitsLoadedMsg:
		m.detailCommits = []string(msg)
		m.commitsLoaded = true
		if m.state == stateDetail {
			m.detailVP.SetContent(m.renderDetailContent())
		}
		return m, nil

	case fetchDoneMsg:
		updated := git.RepoInfo(msg)
		for i, r := range m.repos {
			if r.Path == updated.Path {
				m.repos[i] = updated
				break
			}
		}
		m.fetchingPR = false
		m.statusMsg = "pulled " + updated.Name
		git.SortReposByCol(m.repos, m.sortCol, m.sortDesc)
		if m.cursor >= len(m.repos) {
			m.cursor = len(m.repos) - 1
		}
		return m, nil

	case ghCheckMsg:
		m.ghUnavailable = msg.unavailable
		return m, nil

	case errMsg:
		m.statusMsg = msg.err.Error()
		return m, nil
	}

	if m.state == stateDetail {
		var cmd tea.Cmd
		m.detailVP, cmd = m.detailVP.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global: quit
	if key == "q" || key == "ctrl+c" {
		return m, tea.Quit
	}

	// Help overlay intercepts all keys
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// Global: toggle help
	if key == "?" {
		m.showHelp = true
		return m, nil
	}

	switch m.state {
	case stateList:
		return m.handleListKey(key)
	case stateDetail:
		return m.handleDetailKey(key)
	}
	return m, nil
}

func (m model) handleListKey(key string) (tea.Model, tea.Cmd) {
	n := len(m.repos)
	visRows := m.visibleRows()

	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
		}

	case "down", "j":
		if m.cursor < n-1 {
			m.cursor++
			if m.cursor >= m.offset+visRows {
				m.offset = m.cursor - visRows + 1
			}
		}

	case "g", "home":
		m.cursor = 0
		m.offset = 0

	case "G", "end":
		m.cursor = n - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.offset = clampOffset(n, visRows)

	case "ctrl+f":
		m.cursor += visRows
		if m.cursor >= n {
			m.cursor = n - 1
		}
		if m.cursor >= m.offset+visRows {
			m.offset = m.cursor - visRows + 1
		}

	case "ctrl+b":
		m.cursor -= visRows
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor < m.offset {
			m.offset = m.cursor
		}

	case "enter":
		if n == 0 {
			return m, nil
		}
		m.state = stateDetail
		m.commitsLoaded = false
		m.detailCommits = nil
		m.detailVP = viewport.New(m.width, m.detailVPHeight())
		m.detailVP.SetContent(m.renderDetailContent())
		return m, loadCommitsCmd(m.repos[m.cursor].Path)

	case "o":
		if n > 0 && m.repos[m.cursor].PRUrl != "" {
			return m, openURLCmd(m.repos[m.cursor].PRUrl)
		}

	case "p":
		if n > 0 {
			m.fetchingPR = true
			m.statusMsg = "pulling " + m.repos[m.cursor].Name + "…"
			return m, tea.Batch(m.spinner.Tick, fetchRepoCmd(m.repos[m.cursor].Path))
		}

	case "r":
		m.repos = nil
		m.cursor, m.offset = 0, 0
		m.scanDone, m.scanTotal = 0, 0
		m.prsLoading, m.fetchingPR = false, false
		m.statusMsg = ""
		m.state = stateScanning
		// refresh always fetches so upstream changes are reflected
		return m, tea.Batch(m.spinner.Tick, startScanCmd(m.scanDirs, true))

	// ── Sorting ───────────────────────────────────────────────────────────
	case "0":
		return m.applySort(git.SortStatus), nil
	case "N":
		return m.applySort(git.SortName), nil
	case "B":
		return m.applySort(git.SortBranch), nil
	case "S":
		return m.applySort(git.SortSync), nil
	case "C":
		return m.applySort(git.SortChanges), nil
	case "T":
		return m.applySort(git.SortLastChanged), nil
	case "P":
		return m.applySort(git.SortPR), nil
	}

	return m, nil
}

func (m model) applySort(col git.SortColumn) model {
	if m.sortCol == col {
		m.sortDesc = !m.sortDesc
	} else {
		m.sortCol = col
		m.sortDesc = false
	}
	git.SortReposByCol(m.repos, m.sortCol, m.sortDesc)
	return m
}

func (m model) handleDetailKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.state = stateList
		return m, nil

	case "up", "k":
		m.detailVP.LineUp(1)

	case "down", "j":
		m.detailVP.LineDown(1)

	case "o":
		if len(m.repos) > 0 && m.repos[m.cursor].PRUrl != "" {
			return m, openURLCmd(m.repos[m.cursor].PRUrl)
		}

	case "p":
		if len(m.repos) > 0 {
			m.fetchingPR = true
			m.statusMsg = "pulling " + m.repos[m.cursor].Name + "…"
			return m, tea.Batch(m.spinner.Tick, fetchRepoCmd(m.repos[m.cursor].Path))
		}
	}
	return m, nil
}

// headerHeight returns the number of lines the header occupies.
// = max(len(listHeaderLines), len(logoLines)) + 1 legend line.
// listHeaderLines produces 6 items; logoLines has 8 → max = 8 + 1 = 9.
func (m model) headerHeight() int {
	infoH := 6 // listHeaderLines always produces 6 items
	logoH := len(logoLines)
	h := infoH
	if logoH > infoH {
		h = logoH
	}
	return h + 1 // +1 for the legend line
}

func (m model) visibleRows() int {
	// overhead: header + sep + col header + sep + bottom sep + status bar
	v := m.height - m.headerHeight() - 5
	if v < 1 {
		return 1
	}
	return v
}

// detailVPHeight returns the viewport height for the detail view.
// overhead: header + sep + trailing \n + bottom sep + status bar
func (m model) detailVPHeight() int {
	v := m.height - m.headerHeight() - 4
	if v < 1 {
		return 1
	}
	return v
}

func clampOffset(n, visRows int) int {
	if o := n - visRows; o > 0 {
		return o
	}
	return 0
}
