package tui

import (
	"testing"

	"github.com/samsar/git-repos/internal/keys"
)

// The settings toggle fetch-on-auto-refresh and check-for-updates.
func TestSettingsToggles(t *testing.T) {
	m := model{autoRefreshFetch: true, checkUpdates: true}

	m.settingsCursor = 2
	next, _ := m.handleSettingsKey(keys.Toggle)
	m = next.(model)
	if m.autoRefreshFetch {
		t.Error("fetch on auto-refresh was not turned off")
	}

	m.settingsCursor = 3
	next, _ = m.handleSettingsKey(keys.Toggle)
	m = next.(model)
	if m.checkUpdates {
		t.Error("check for updates was not turned off")
	}
}
