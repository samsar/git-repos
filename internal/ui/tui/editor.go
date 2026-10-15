package tui

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/samsar/git-repos/internal/git"
	"github.com/samsar/git-repos/internal/keys"
	"github.com/samsar/git-repos/internal/launch"
)

// editorOpenedMsg is how opening a repo in an editor went: what the
// status bar says of it.
type editorOpenedMsg struct {
	text string
	err  error
}

// selectedRepo is the repo the keys act on: the one under the cursor in
// the list, or the one the detail view shows.
func (m model) selectedRepo() (git.RepoInfo, bool) {
	if m.state == stateDetail {
		if m.cursor < len(m.repos) {
			return m.repos[m.cursor], true
		}
		return git.RepoInfo{}, false
	}
	filtered := m.displayRepos()
	if m.cursor < len(filtered) {
		return filtered[m.cursor], true
	}
	return git.RepoInfo{}, false
}

// theDefaultEditor is the editor e opens a repo in, and whether there is
// one.
func (m model) theDefaultEditor() (launch.Editor, bool) {
	return launch.Pick(m.editors, m.defaultEditor, os.Getenv)
}

// openInDefault opens the selected repo in the default editor.
func (m model) openInDefault() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRepo()
	if !ok {
		return m, nil
	}
	e, ok := m.theDefaultEditor()
	if !ok {
		if m.defaultEditor != "" {
			m.statusMsg = fmt.Sprintf("no editor %q: git repos editors lists them", m.defaultEditor)
		} else {
			m.statusMsg = "no editor found: add one to config.json's editors"
		}
		return m, nil
	}
	return m.openIn(e, r)
}

// openIn opens r in e: an app, at once; in git-repos's own terminal,
// which waits for it; or in a new window, here instead when this
// terminal can't open one.
func (m model) openIn(e launch.Editor, r git.RepoInfo) (tea.Model, tea.Cmd) {
	switch {
	case e.How() == launch.Inline:
		return m, runInline(e, r)
	case e.How() == launch.Window && !m.openerOK && len(e.Cmd) == 0:
		return m, openShellCmd(r.Path)
	case e.How() == launch.Window && !m.openerOK:
		return m, runInline(e, r)
	case e.How() == launch.Window:
		opener := m.opener
		m.statusMsg = fmt.Sprintf("opening %s in %s…", r.Name, e.Name)
		return m, func() tea.Msg {
			err := opener.Open(context.Background(), e, r.Path)
			return editorOpenedMsg{text: fmt.Sprintf("opened %s in %s, in a new %s window", r.Name, e.Name, opener.Name), err: err}
		}
	}
	return m, func() tea.Msg {
		err := launch.StartGUI(e, r.Path)
		return editorOpenedMsg{text: fmt.Sprintf("opened %s in %s", r.Name, e.Name), err: err}
	}
}

// runInline runs e on r in git-repos's terminal, as the shell key does.
func runInline(e launch.Editor, r git.RepoInfo) tea.Cmd {
	return tea.ExecProcess(launch.InlineCmd(e, r.Path), func(err error) tea.Msg {
		return editorOpenedMsg{err: err}
	})
}

// onEditorOpened says in the status bar how opening went.
func (m model) onEditorOpened(msg editorOpenedMsg) model {
	switch {
	case msg.err != nil:
		m.statusMsg = "couldn't open: " + msg.err.Error()
	default:
		m.statusMsg = msg.text
	}
	return m
}

// openPicker shows the editors to pick from, the default selected.
func (m model) openPicker() model {
	if _, ok := m.selectedRepo(); !ok {
		return m
	}
	m.showPicker, m.pickerCursor = true, 0
	if e, ok := m.theDefaultEditor(); ok {
		m.pickerCursor = max(0, slices.IndexFunc(m.editors, func(o launch.Editor) bool { return o.Name == e.Name }))
	}
	return m
}

// handlePickerKey takes a key while the picker is open: 1 to 9 open that
// editor, enter the one selected, your move keys select, esc closes.
func (m model) handlePickerKey(action, key string) (tea.Model, tea.Cmd) {
	pick := -1
	switch {
	case action == keys.MoveDown:
		m.pickerCursor = min(m.pickerCursor+1, len(m.editors)-1)
	case action == keys.MoveUp:
		m.pickerCursor = max(m.pickerCursor-1, 0)
	case key == "enter":
		pick = m.pickerCursor
	case action == keys.Back:
		m.showPicker = false
	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		pick = int(key[0] - '1')
	}
	if pick < 0 || pick >= len(m.editors) {
		return m, nil
	}
	m.showPicker = false
	r, ok := m.selectedRepo()
	if !ok {
		return m, nil
	}
	return m.openIn(m.editors[pick], r)
}

// describe says how e opens a repo here: in which terminal's window, or
// here when this one can't open them.
func (m model) describe(e launch.Editor) string {
	switch {
	case e.How() == launch.Window && !m.openerOK:
		return "here (this terminal can't open windows)"
	case e.How() == launch.Window && len(e.Cmd) == 0:
		return "a shell in a new " + m.opener.Name + " window"
	case e.How() == launch.Window:
		return "in a new " + m.opener.Name + " window"
	}
	return e.Describe()
}

// viewPicker is the editors to open the selected repo in, numbered.
func (m model) viewPicker(headerLines []string) string {
	r, _ := m.selectedRepo()
	bg := headerBg
	fill := lipgloss.NewStyle().Background(bg)
	greyS := lipgloss.NewStyle().Background(bg).Foreground(colorLightGray)
	dimS := lipgloss.NewStyle().Background(bg).Foreground(staleFg)
	cyanS := lipgloss.NewStyle().Background(bg).Foreground(colorCyan)
	titleS := lipgloss.NewStyle().Background(bg).Foreground(colorBrightWhite).Bold(true)
	selS := lipgloss.NewStyle().Background(bg).Foreground(colorBrightWhite).Bold(true)

	var b strings.Builder
	b.WriteString(m.renderHeader3Zone(headerLines))
	b.WriteString(m.sep())

	def, _ := m.theDefaultEditor()
	nameW := 0
	for _, e := range m.editors {
		nameW = max(nameW, lipgloss.Width(e.Name))
	}
	lines := []string{"", titleS.Render("  Open " + r.Name + " in…"), ""}
	for i, e := range m.editors {
		num := "   "
		if i < 9 {
			num = strconv.Itoa(i+1) + "  "
		}
		mark, nameS := fill.Render("    "), greyS
		if i == m.pickerCursor {
			mark, nameS = cyanS.Render("  › "), selS
		}
		line := mark + cyanS.Render(num) + nameS.Render(fmt.Sprintf("%-*s", nameW, e.Name)) + fill.Render("  ") + dimS.Render(m.describe(e))
		if e.Name == def.Name {
			line += dimS.Render("  · default")
		}
		lines = append(lines, line)
	}
	lines = append(lines, "",
		greyS.Render("  ")+cyanS.Render("<1-9>")+greyS.Render(" Open   ")+cyanS.Render("<enter>")+greyS.Render(" Open selected   ")+cyanS.Render("<esc>")+greyS.Render(" Cancel"),
		"")
	for _, line := range lines {
		b.WriteString(fill.Width(m.width).Render(line) + "\n")
	}

	written := m.headerHeight() + 1 + len(lines)
	for i := written; i < m.height-1; i++ {
		b.WriteString(fill.Width(m.width).Render("") + "\n")
	}
	b.WriteString(m.renderStatusBar())
	return b.String()
}

// nextDefaultEditor is the editor after the default, as the settings
// cycle through them.
func (m model) nextDefaultEditor() string {
	if len(m.editors) == 0 {
		return m.defaultEditor
	}
	e, _ := m.theDefaultEditor()
	i := slices.IndexFunc(m.editors, func(o launch.Editor) bool { return o.Name == e.Name })
	return m.editors[(i+1)%len(m.editors)].Name
}
