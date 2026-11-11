package tui

import (
	"context"
	"testing"
	"time"

	"github.com/samsar/git-repos/internal/keys"
)

// Restarting the timer cancels the old chain and leaves one new generation;
// a tick already in flight from the old chain is ignored.
func TestRestartAutoRefreshCancelsOldChain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := model{
		autoRefreshMins:   10,
		autoRefreshCtx:    ctx,
		autoRefreshCancel: cancel,
		autoRefreshGen:    1,
	}

	next, cmd := m.restartAutoRefresh()
	if next.autoRefreshGen != 2 {
		t.Errorf("generation %d, want 2", next.autoRefreshGen)
	}
	if cmd == nil || !next.autoRefreshRunning {
		t.Error("restart did not schedule a live timer")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Error("the old timer's context was not canceled")
	}

	got, _ := next.Update(autoRefreshMsg{gen: 1})
	if got.(model).refreshing {
		t.Error("a stale tick started a refresh")
	}
}

// Turning auto-refresh off cancels the one timer and schedules none.
func TestRestartAutoRefreshOff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := model{
		autoRefreshMins:   0,
		autoRefreshCtx:    ctx,
		autoRefreshCancel: cancel,
		autoRefreshGen:    1,
	}

	next, cmd := m.restartAutoRefresh()
	if cmd != nil {
		t.Error("turning auto-refresh off scheduled a timer")
	}
	if next.autoRefreshCtx != nil || next.autoRefreshCancel != nil || next.autoRefreshRunning {
		t.Error("turning auto-refresh off kept the timer alive")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Error("the old timer's context was not canceled")
	}
}

// A canceled chain's tick command returns without a message.
func TestAutoRefreshTickCmdCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if msg := autoRefreshTickCmd(ctx, 10, 1)(); msg != nil {
		t.Errorf("canceled tick returned %T", msg)
	}
}

// Leaving settings without changing the interval leaves the one timer alone.
func TestSettingsLeavesUnchangedTimerRunning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	m := model{
		state:                   stateSettings,
		autoRefreshMins:         10,
		settingsAutoRefreshMins: 10,
		autoRefreshCtx:          ctx,
		autoRefreshCancel:       cancel,
		autoRefreshRunning:      true,
		autoRefreshGen:          1,
	}

	next, cmd := m.handleSettingsKey(keys.Back)
	got := next.(model)
	if cmd != nil {
		t.Error("leaving settings scheduled a second timer")
	}
	if got.autoRefreshGen != 1 {
		t.Errorf("generation %d, want 1", got.autoRefreshGen)
	}
	select {
	case <-ctx.Done():
		t.Error("leaving settings canceled the running timer")
	default:
	}
}

// Changing the interval cancels the old timer and starts one new one.
func TestSettingsIntervalChangeRestartsOneTimer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	m := model{
		state:                   stateSettings,
		autoRefreshMins:         20,
		settingsAutoRefreshMins: 10,
		autoRefreshCtx:          ctx,
		autoRefreshCancel:       cancel,
		autoRefreshRunning:      true,
		autoRefreshGen:          1,
	}

	next, cmd := m.handleSettingsKey(keys.Back)
	got := next.(model)
	if cmd == nil {
		t.Error("changing the interval did not schedule a timer")
	}
	if got.autoRefreshGen != 2 || !got.autoRefreshRunning {
		t.Errorf("generation %d, running %v, want 2 and a live timer", got.autoRefreshGen, got.autoRefreshRunning)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Error("the old timer was not canceled")
	}
}

// Editing the interval to 0 can let an arriving tick end the chain; going
// back to the original interval must notice the dead chain and restart it.
func TestSettingsZeroThenBackToSameRestartsDeadChain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	m := model{
		state:                   stateSettings,
		autoRefreshMins:         10,
		settingsAutoRefreshMins: 10,
		autoRefreshCtx:          ctx,
		autoRefreshCancel:       cancel,
		autoRefreshRunning:      true,
		autoRefreshGen:          1,
	}

	m.autoRefreshMins = 0
	next, cmd := m.Update(autoRefreshMsg{gen: 1})
	m = next.(model)
	if cmd != nil || m.autoRefreshRunning {
		t.Fatalf("a tick with auto-refresh at 0 kept the chain alive")
	}

	m.autoRefreshMins = 10
	next, cmd = m.handleSettingsKey(keys.Back)
	m = next.(model)
	if cmd == nil || !m.autoRefreshRunning {
		t.Error("leaving settings did not revive the dead timer")
	}
	if m.autoRefreshGen != 2 {
		t.Errorf("generation %d, want 2", m.autoRefreshGen)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Error("the dead chain's context was not canceled on restart")
	}
}
