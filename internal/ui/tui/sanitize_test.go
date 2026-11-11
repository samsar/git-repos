package tui

import (
	"strings"
	"testing"

	"github.com/samsar/git-repos/internal/git"
)

func TestMakeHyperlinkRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{
		"javascript:alert(1)",
		"file:///etc/passwd",
		"https://github.com/a/b\x1b]8;;https://evil.example",
		"",
	} {
		if got := makeHyperlink(raw, "#1"); got != "#1" {
			t.Errorf("makeHyperlink(%q) = %q, want the plain text", raw, got)
		}
	}
	got := makeHyperlink("https://github.com/acme/api/pull/42", "#42")
	if !strings.Contains(got, "\x1b]8;;https://github.com/acme/api/pull/42\x1b\\#42\x1b]8;;\x1b\\") {
		t.Errorf("hyperlink %q", got)
	}
}

// A URL that isn't http(s) is never handed to the browser.
func TestOpenURLCmdRejectsUnsafeURL(t *testing.T) {
	msg := openURLCmd("file:///etc/passwd")()
	if _, ok := msg.(errMsg); !ok {
		t.Errorf("openURLCmd returned %T, want errMsg", msg)
	}
}

func TestRenderRowStripsRepoData(t *testing.T) {
	r := repo("evil\x1b[2J", "main\x1b]0;spoof\x07", "subject\x1b]8;;https://evil.example\x1b\\click\x1b]8;;\x1b\\")
	m := model{width: 140, repos: []git.RepoInfo{r}}

	got := m.renderRow(r, false)
	if strings.Contains(got, "\x1b[2J") || strings.Contains(got, "\x1b]0;") || strings.Contains(got, "\x1b]8;;https://evil.example") {
		t.Errorf("repo data reached the terminal: %q", got)
	}
}

func TestRenderDetailStripsAndRedacts(t *testing.T) {
	r := repo("evil", "main\x1b[2J", "subject")
	r.Path = "/repos/\x1b]8;;https://evil.example\x1b\\path"
	r.LastRel = "now\x1b]0;spoof\x07"
	r.Remotes = []git.Remote{{
		Name: "origin\x1b[2J",
		URL:  "https://user:ghp_secret@github.com/acme/api.git\x1b]8;;https://evil.example",
	}}
	r.StagedFiles = []string{"\x1b]8;;https://evil.example\x1b\\file"}
	r.PRNumber, r.PRUrl = 42, "https://github.com/acme/api/pull/42"

	m := model{
		width:         120,
		height:        40,
		repos:         []git.RepoInfo{r},
		commitsLoaded: true,
		behindLoaded:  true,
		detailCommits: []string{"abc  now  \x1b]8;;https://evil.example\x1b\\subject"},
	}
	got := m.renderDetailContent()

	if strings.Contains(got, "\x1b]8;;https://evil.example") || strings.Contains(got, "\x1b[2J") || strings.Contains(got, "\x1b]0;") {
		t.Errorf("repo data reached the terminal: %q", got)
	}
	if strings.Contains(got, "ghp_secret") {
		t.Error("a credential in a remote URL was displayed")
	}
	if !strings.Contains(got, "***@github.com/acme/api.git") {
		t.Errorf("the remote URL was not shown redacted: %q", got)
	}
}

func TestWindowTitleStripsRepoName(t *testing.T) {
	m := model{
		state: stateDetail,
		repos: []git.RepoInfo{{Name: "evil\x1b]0;spoof\x07"}},
	}
	if got := m.windowTitle(); strings.ContainsRune(got, '\x1b') {
		t.Errorf("window title kept an escape sequence: %q", got)
	}
}
