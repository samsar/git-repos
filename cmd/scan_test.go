package cmd

import (
	"strings"
	"testing"

	"github.com/samsar/git-repos/internal/git"
)

// --json redacts credentials in remote URLs, without changing the values
// used elsewhere in the process.
func TestRedactRepoURLs(t *testing.T) {
	repos := []git.RepoInfo{{
		RemoteURL: "https://user:ghp_secret@github.com/acme/api.git",
		PRUrl:     "https://github.com/acme/api/pull/42",
		Remotes: []git.Remote{
			{Name: "origin", URL: "https://ghp_secret@github.com/acme/api.git"},
			{Name: "upstream", URL: "git@github.com:acme/api.git"},
		},
	}}

	got := redactRepoURLs(repos)
	if strings.Contains(got[0].RemoteURL, "ghp_secret") {
		t.Errorf("remote_url kept its credential: %s", got[0].RemoteURL)
	}
	if got[0].Remotes[0].URL != "https://***@github.com/acme/api.git" {
		t.Errorf("origin URL = %s", got[0].Remotes[0].URL)
	}
	if got[0].Remotes[1].URL != "git@github.com:acme/api.git" {
		t.Errorf("SSH remote URL changed: %s", got[0].Remotes[1].URL)
	}
	if got[0].PRUrl != "https://github.com/acme/api/pull/42" {
		t.Errorf("PR URL changed: %s", got[0].PRUrl)
	}
	if !strings.Contains(repos[0].RemoteURL, "ghp_secret") {
		t.Error("the original repo was modified")
	}
}
