package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/samsar/git-repos/internal/git"
)

// searchFields returns the repo fields matched against the search query.
// To add or remove searchable columns, edit this slice.
func searchFields(r git.RepoInfo) []string {
	return []string{r.Name, r.Branch, r.LastMsg}
}

func repoMatchesSearch(r git.RepoInfo, lowerQuery string) bool {
	for _, f := range searchFields(r) {
		if strings.Contains(strings.ToLower(f), lowerQuery) {
			return true
		}
	}
	return false
}

// displayRepos returns m.repos filtered by the active search query.
// When there is no query it returns m.repos directly (no allocation).
func (m model) displayRepos() []git.RepoInfo {
	if m.searchQuery == "" {
		return m.repos
	}
	q := strings.ToLower(m.searchQuery)
	var out []git.RepoInfo
	for _, r := range m.repos {
		if repoMatchesSearch(r, q) {
			out = append(out, r)
		}
	}
	return out
}

// filteredToFullIdx translates a cursor position in displayRepos() to an index
// in m.repos. Used when transitioning from list → detail view.
func (m model) filteredToFullIdx(filteredIdx int) int {
	if m.searchQuery == "" {
		return filteredIdx
	}
	q := strings.ToLower(m.searchQuery)
	count := 0
	for i, r := range m.repos {
		if repoMatchesSearch(r, q) {
			if count == filteredIdx {
				return i
			}
			count++
		}
	}
	return min(filteredIdx, max(0, len(m.repos)-1))
}

// fullToFilteredIdx translates an m.repos index to a cursor position in
// displayRepos(). Used when transitioning from detail → list view.
func (m model) fullToFilteredIdx(fullIdx int) int {
	if m.searchQuery == "" {
		return fullIdx
	}
	if fullIdx < 0 || fullIdx >= len(m.repos) {
		return 0
	}
	targetPath := m.repos[fullIdx].Path
	q := strings.ToLower(m.searchQuery)
	count := 0
	for _, r := range m.repos {
		if repoMatchesSearch(r, q) {
			if r.Path == targetPath {
				return count
			}
			count++
		}
	}
	return 0
}

func (m model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	q := m.query()
	switch key {
	case "esc":
		m.searching = false
		m = m.setQuery("")
	case "enter":
		m.searching = false
	case "backspace", "ctrl+h":
		if len(q) > 0 {
			r := []rune(q)
			m = m.setQuery(string(r[:len(r)-1]))
		}
	default:
		if msg.Text != "" {
			m = m.setQuery(q + msg.Text)
		}
	}
	return m, nil
}

// query is the search of the page shown: the PR page's, or the repos'.
func (m model) query() string {
	if m.state == statePRs {
		return m.prQuery
	}
	return m.searchQuery
}

// setQuery sets the search of the page shown to q, its cursor back at
// its top.
func (m model) setQuery(q string) model {
	if m.state == statePRs {
		m.prQuery = q
	} else {
		m.searchQuery = q
	}
	return m.searchChanged()
}

// searchChanged puts the cursor of the page searched back at its top, as
// the search changes what it shows.
func (m model) searchChanged() model {
	if m.state == statePRs {
		m.prCursor, m.prOffset = 0, 0
		return m
	}
	m.cursor, m.offset = 0, 0
	return m
}
