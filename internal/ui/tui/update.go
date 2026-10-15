package tui

import (
	"strconv"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/keys"
	"github.com/samsar/git-repos/internal/version"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		// Lay out inside the border that View draws around the whole app.
		m.width, m.height = max(0, msg.Width-2), max(0, msg.Height-2)
		if m.state == stateDetail {
			m.detailVP.SetWidth(m.width)
			m.detailVP.SetHeight(m.detailVPHeight())
			m.detailVP.SetContent(m.renderDetailContent())
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case scanStartedMsg:
		m.resultCh = msg.ch
		m.scanTotal = msg.total
		if msg.total == 0 {
			if m.refreshing {
				m.repos = nil
				m.refreshRepos = nil
				m.refreshing = false
			} else {
				m.state = stateList
			}
			m.statusMsg = "no repos found"
			return m, nil
		}
		return m, waitForScanCmd(m.resultCh)

	case repoScannedMsg:
		if m.refreshing {
			if !m.hidden[msg.Repo.Name] {
				m.refreshRepos = append(m.refreshRepos, msg.Repo)
			}
		} else {
			if !m.hidden[msg.Repo.Name] {
				m.repos = append(m.repos, msg.Repo)
			}
		}
		m.scanDone = msg.Done
		m.scanTotal = msg.Total
		return m, waitForScanCmd(m.resultCh)

	case scanDoneMsg:
		if m.refreshing {
			m.repos = m.refreshRepos
			m.refreshRepos = nil
			m.refreshing = false
			m.statusMsg = ""
			git.SortReposByCol(m.repos, m.sortCol, m.sortDesc)
			m.cursor = min(m.cursor, max(0, len(m.repos)-1))
			if m.state == stateDetail {
				m.detailVP.SetContent(m.renderDetailContent())
			}
			if !m.noPRs {
				m.prsLoading = true
				return m, tea.Batch(m.spinner.Tick, loadPRsCmd(m.repos))
			}
			return m, nil
		}
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
		m.prsEverLoaded = true
		git.SortReposByCol(m.repos, m.sortCol, m.sortDesc)
		if m.state == stateDetail {
			m.detailVP.SetContent(m.renderDetailContent())
		}
		return m, nil

	case singleRepoPRLoadedMsg:
		updated := git.RepoInfo(msg)
		activePath := ""
		if m.state == stateDetail && m.cursor < len(m.repos) {
			activePath = m.repos[m.cursor].Path
		}
		for i, r := range m.repos {
			if r.Path == updated.Path {
				m.repos[i] = updated
				break
			}
		}
		m.prsLoading = false
		m.prsEverLoaded = true
		git.SortReposByCol(m.repos, m.sortCol, m.sortDesc)
		if activePath != "" {
			for i, r := range m.repos {
				if r.Path == activePath {
					m.cursor = i
					break
				}
			}
		}
		if m.state == stateDetail {
			m.detailVP.SetContent(m.renderDetailContent())
		}
		return m, nil

	case commitsLoadedMsg:
		m.detailCommits = []string(msg)
		m.commitsLoaded = true
		if m.state == stateDetail {
			m.detailVP.SetContent(m.renderDetailContent())
		}
		return m, nil

	case behindCommitsLoadedMsg:
		m.behindCommits = []string(msg)
		m.behindLoaded = true
		if m.state == stateDetail {
			m.detailVP.SetContent(m.renderDetailContent())
		}
		return m, nil

	case fetchDoneMsg:
		updated := git.RepoInfo(msg)
		// Detail view follows the active repo across re-sorts so the user
		// keeps looking at the repo they just pulled.
		activePath := ""
		if m.state == stateDetail && m.cursor < len(m.repos) {
			activePath = m.repos[m.cursor].Path
		}
		for i, r := range m.repos {
			if r.Path == updated.Path {
				m.repos[i] = updated
				break
			}
		}
		m.fetchingPR = false
		m.statusMsg = "pulled " + updated.Name
		git.SortReposByCol(m.repos, m.sortCol, m.sortDesc)
		if activePath != "" {
			for i, r := range m.repos {
				if r.Path == activePath {
					m.cursor = i
					break
				}
			}
		} else {
			m.cursor = min(m.cursor, max(0, len(m.repos)-1))
		}
		if m.state == stateDetail {
			m.commitsLoaded = false
			m.detailCommits = nil
			m.behindLoaded = false
			m.behindCommits = nil
			m.detailVP.SetContent(m.renderDetailContent())
			cmds := []tea.Cmd{loadCommitsCmd(updated.Path)}
			if updated.Behind > 0 {
				cmds = append(cmds, loadBehindCommitsCmd(updated.Path))
			} else {
				m.behindLoaded = true
			}
			if !m.noPRs {
				m.prsLoading = true
				cmds = append(cmds, m.spinner.Tick, loadPRForRepoCmd(updated))
			}
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case pullAllDoneMsg:
		m.repos = []git.RepoInfo(msg)
		m.fetchingPR = false
		m.statusMsg = "pulled all repos"
		git.SortReposByCol(m.repos, m.sortCol, m.sortDesc)
		m.cursor = min(m.cursor, max(0, len(m.repos)-1))
		if !m.noPRs {
			m.prsLoading = true
			return m, tea.Batch(m.spinner.Tick, loadPRsCmd(m.repos))
		}
		return m, nil

	case autoRefreshMsg:
		if m.autoRefreshMins > 0 {
			if m.state == stateList && !m.refreshing && !m.fetchingPR {
				m.refreshing = true
				m.refreshRepos = nil
				m.scanDone, m.scanTotal = 0, 0
				return m, tea.Batch(m.spinner.Tick, startScanCmd(m.scanDirs, true), autoRefreshTickCmd(m.autoRefreshMins))
			}
			return m, autoRefreshTickCmd(m.autoRefreshMins)
		}
		return m, nil

	case shellReturnMsg:
		m.statusMsg = ""
		return m, nil

	case deleteRepoDoneMsg:
		for i, r := range m.repos {
			if r.Name == msg.name {
				m.repos = append(m.repos[:i], m.repos[i+1:]...)
				break
			}
		}
		m.cursor = min(m.cursor, max(0, len(m.repos)-1))
		m.statusMsg = "deleted " + msg.name
		return m, nil

	case ghCheckMsg:
		m.ghUnavailable = msg.unavailable
		return m, nil

	case versionCheckMsg:
		if msg.latest != "" {
			m.latestVersion = msg.latest
			m.updateAvailable = version.IsNewer(m.version, msg.latest)
		}
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

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// ctrl+c always quits, even in search mode
	if key == keys.CtrlC {
		return m, tea.Quit
	}

	// Text fields take every remaining key while active
	if m.searching {
		return m.handleSearchKey(msg)
	}
	if m.settingsEditing {
		return m.handleSettingsEditKey(key)
	}

	action := m.keys.in(m.keyView(), key)

	if action == keys.Quit {
		return m, tea.Quit
	}

	// Help overlay intercepts all keys
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	if action == keys.Help {
		m.showHelp = true
		return m, nil
	}

	// Delete confirmation intercepts all keys
	if m.showDeleteConfirm {
		switch key {
		case "y", "enter":
			filtered := m.displayRepos()
			if len(filtered) > 0 && m.cursor < len(filtered) {
				repo := filtered[m.cursor]
				m.showDeleteConfirm = false
				m.statusMsg = "deleting " + repo.Name + "…"
				return m, tea.Batch(m.spinner.Tick, deleteRepoDirCmd(repo))
			}
		}
		m.showDeleteConfirm = false
		return m, nil
	}

	switch m.state {
	case stateList:
		return m.handleListKey(action)
	case stateDetail:
		return m.handleDetailKey(action)
	case stateSettings:
		return m.handleSettingsKey(action)
	}
	return m, nil
}

func (m model) handleListKey(action string) (tea.Model, tea.Cmd) {
	filtered := m.displayRepos()
	n := len(filtered)
	visRows := m.visibleRows()

	switch action {
	case keys.MoveUp:
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
		}

	case keys.MoveDown:
		if m.cursor < n-1 {
			m.cursor++
			if m.cursor >= m.offset+visRows {
				m.offset = m.cursor - visRows + 1
			}
		}

	case keys.First:
		m.cursor = 0
		m.offset = 0

	case keys.Last:
		m.cursor = max(0, n-1)
		m.offset = clampOffset(n, visRows)

	case keys.PageDown:
		m.cursor += visRows
		if m.cursor >= n {
			m.cursor = n - 1
		}
		if m.cursor >= m.offset+visRows {
			m.offset = m.cursor - visRows + 1
		}

	case keys.PageUp:
		m.cursor -= visRows
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor < m.offset {
			m.offset = m.cursor
		}

	case keys.Search:
		m.searching = true
		// Keep existing query so user can refine it; cursor stays put.
		return m, nil

	case keys.Details:
		if n == 0 {
			return m, nil
		}
		// Translate filtered cursor → full m.repos index for detail view.
		m.cursor = m.filteredToFullIdx(m.cursor)
		m.state = stateDetail
		m.commitsLoaded = false
		m.detailCommits = nil
		m.behindLoaded = false
		m.behindCommits = nil
		m.detailVP = viewport.New(viewport.WithWidth(m.width), viewport.WithHeight(m.detailVPHeight()))
		m.detailVP.Style = lipgloss.NewStyle().Background(rowBg)
		m.detailVP.SetContent(m.renderDetailContent())
		cmds := []tea.Cmd{loadCommitsCmd(m.repos[m.cursor].Path)}
		if m.repos[m.cursor].Behind > 0 {
			cmds = append(cmds, loadBehindCommitsCmd(m.repos[m.cursor].Path))
		} else {
			m.behindLoaded = true
		}
		return m, tea.Batch(cmds...)

	case keys.OpenPR:
		if n > 0 && filtered[m.cursor].PRUrl != "" {
			return m, openURLCmd(filtered[m.cursor].PRUrl)
		}

	case keys.Pull:
		if n > 0 {
			m.fetchingPR = true
			m.statusMsg = "pulling " + filtered[m.cursor].Name + "…"
			return m, tea.Batch(m.spinner.Tick, fetchRepoCmd(filtered[m.cursor].Path))
		}

	case keys.PullAll:
		if n > 0 && !m.fetchingPR && !m.refreshing {
			m.fetchingPR = true
			m.statusMsg = "pulling all repos…"
			return m, tea.Batch(m.spinner.Tick, pullAllCmd(m.repos))
		}

	case keys.Shell:
		if n > 0 {
			return m, openShellCmd(filtered[m.cursor].Path)
		}

	case keys.Delete:
		if n > 0 {
			m.showDeleteConfirm = true
		}

	case keys.Back:
		if m.searchQuery != "" {
			m.searchQuery = ""
			m.cursor = 0
			m.offset = 0
		}

	case keys.Settings:
		m.state = stateSettings

	case keys.Refresh:
		if !m.refreshing {
			m.refreshing = true
			m.refreshRepos = nil
			m.scanDone, m.scanTotal = 0, 0
			m.statusMsg = ""
			return m, tea.Batch(m.spinner.Tick, startScanCmd(m.scanDirs, true))
		}

	// ── Sorting ───────────────────────────────────────────────────────────
	case keys.SortStatus:
		return m.applySort(git.SortStatus), nil
	case keys.SortName:
		return m.applySort(git.SortName), nil
	case keys.SortBranch:
		return m.applySort(git.SortBranch), nil
	case keys.SortSync:
		return m.applySort(git.SortSync), nil
	case keys.SortChanges:
		return m.applySort(git.SortChanges), nil
	case keys.SortTime:
		return m.applySort(git.SortLastChanged), nil
	case keys.SortPR:
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

func (m model) handleDetailKey(action string) (tea.Model, tea.Cmd) {
	switch action {
	case keys.Back:
		// Translate full m.repos cursor back to filtered-list cursor.
		m.cursor = m.fullToFilteredIdx(m.cursor)
		m.state = stateList
		return m, nil

	case keys.MoveUp:
		m.detailVP.ScrollUp(1)

	case keys.MoveDown:
		m.detailVP.ScrollDown(1)

	case keys.OpenPR:
		if len(m.repos) > 0 && m.repos[m.cursor].PRUrl != "" {
			return m, openURLCmd(m.repos[m.cursor].PRUrl)
		}

	case keys.OpenGitHub:
		if len(m.repos) > 0 {
			r := m.repos[m.cursor]
			url := git.RemoteToWebURL(r.RemoteURL)
			if url == "" {
				for _, rem := range r.Remotes {
					if u := git.RemoteToWebURL(rem.URL); u != "" {
						url = u
						break
					}
				}
			}
			if url != "" {
				return m, openURLCmd(url)
			}
		}

	case keys.Pull:
		if len(m.repos) > 0 {
			m.fetchingPR = true
			m.statusMsg = "pulling " + m.repos[m.cursor].Name + "…"
			return m, tea.Batch(m.spinner.Tick, fetchRepoCmd(m.repos[m.cursor].Path))
		}

	case keys.Shell:
		if len(m.repos) > 0 {
			return m, openShellCmd(m.repos[m.cursor].Path)
		}

	case keys.Refresh:
		if len(m.repos) > 0 && !m.fetchingPR {
			m.fetchingPR = true
			m.statusMsg = "refreshing " + m.repos[m.cursor].Name + "…"
			return m, tea.Batch(m.spinner.Tick, refreshSingleRepoCmd(m.repos[m.cursor].Path))
		}

	case keys.Settings:
		m.state = stateSettings
	}
	return m, nil
}

// handleSettingsEditKey is the number field of the setting being edited:
// it takes every key while open.
func (m model) handleSettingsEditKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		if n, err := strconv.Atoi(m.settingsEditBuf); err == nil && n >= 0 {
			m.autoRefreshMins = n
		}
		m.settingsEditing = false
		m.settingsEditBuf = ""
	case "esc":
		m.settingsEditing = false
		m.settingsEditBuf = ""
	case "backspace":
		if len(m.settingsEditBuf) > 0 {
			m.settingsEditBuf = m.settingsEditBuf[:len(m.settingsEditBuf)-1]
		}
	default:
		if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
			m.settingsEditBuf += key
		}
	}
	return m, nil
}

func (m model) handleSettingsKey(action string) (tea.Model, tea.Cmd) {
	switch action {
	case keys.MoveDown:
		if m.settingsCursor < 1 {
			m.settingsCursor++
		}
	case keys.MoveUp:
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case keys.Toggle:
		switch m.settingsCursor {
		case 0: // auto_refresh_mins — start editing
			m.settingsEditing = true
			m.settingsEditBuf = strconv.Itoa(m.autoRefreshMins)
		case 1: // boot_fetch — toggle
			m.bootFetch = !m.bootFetch
		}
	case keys.Edit:
		if m.settingsCursor == 0 {
			m.settingsEditing = true
			m.settingsEditBuf = strconv.Itoa(m.autoRefreshMins)
		}
	case keys.Back:
		m.state = stateList
		m.saveConfig()
		var cmds []tea.Cmd
		if m.autoRefreshMins > 0 {
			cmds = append(cmds, autoRefreshTickCmd(m.autoRefreshMins))
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// headerHeight returns the number of lines the header occupies: one per logo
// line. The version sits beside the last logo line, and the legend shares that
// row too.
func (m model) headerHeight() int {
	return len(logoLines)
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
// overhead: header + sep + bottom sep + status bar
func (m model) detailVPHeight() int {
	v := m.height - m.headerHeight() - 3
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
