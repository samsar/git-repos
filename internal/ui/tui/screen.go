package tui

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// screen is the terminal git-repos draws on. Bubble Tea leaves its
// alternate screen to hand the terminal to a shell or editor, and comes
// back to it after, so the screen git-repos started from flickers
// through on the way in and out. The screen can keep the alternate
// screen up instead, for the shell or editor to run on.
type screen struct {
	f     *os.File
	state atomic.Int32
}

const (
	screenAsIs   int32 = iota // writes go through as written
	screenKeepUp              // drop Bubble Tea's next leaving of the alternate screen
	screenStayOn              // and then its coming back to it, as it never left
)

var (
	leaveAltScreen = []byte(ansi.ResetModeAltScreenSaveCursor)
	enterAltScreen = []byte(ansi.SetModeAltScreenSaveCursor)
)

func newScreen(f *os.File) *screen { return &screen{f: f} }

// keepAltScreen keeps the alternate screen up through Bubble Tea's next
// hand-over of the terminal. A nil screen does nothing.
func (s *screen) keepAltScreen() {
	if s != nil {
		s.state.Store(screenKeepUp)
	}
}

func (s *screen) Write(p []byte) (int, error) {
	out := p
	switch s.state.Load() {
	case screenKeepUp:
		if bytes.Contains(p, leaveAltScreen) {
			out = bytes.Replace(p, leaveAltScreen, nil, 1)
			s.state.Store(screenStayOn)
		}
	case screenStayOn:
		if bytes.Contains(p, enterAltScreen) {
			out = bytes.Replace(p, enterAltScreen, nil, 1)
			s.state.Store(screenAsIs)
		}
	}
	if _, err := s.f.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *screen) Read(p []byte) (int, error) { return s.f.Read(p) }
func (s *screen) Close() error               { return s.f.Close() }
func (s *screen) Fd() uintptr                { return s.f.Fd() }

// execOnAltScreen runs c as tea.ExecProcess does, but on git-repos's
// alternate screen, cleared, so it never draws on the screen git-repos
// started from, which quitting shows, nor lets it flicker through.
func (m model) execOnAltScreen(c *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
	run := tea.Exec(&altScreenCmd{Cmd: c}, fn)
	return func() tea.Msg {
		m.screen.keepAltScreen()
		return run()
	}
}

// altScreenCmd is an exec.Cmd that runs on the alternate screen: the one
// git-repos keeps up when its screen does, or one of its own.
type altScreenCmd struct {
	*exec.Cmd
	kept bool
}

func (c *altScreenCmd) SetStdin(r io.Reader) {
	if c.Stdin == nil {
		c.Stdin = r
	}
}

func (c *altScreenCmd) SetStdout(w io.Writer) {
	if s, ok := w.(*screen); ok {
		// The command gets the terminal itself, not a pipe to it
		w, c.kept = s.f, true
	}
	if c.Stdout == nil {
		c.Stdout = w
	}
}

func (c *altScreenCmd) SetStderr(w io.Writer) {
	if c.Stderr == nil {
		c.Stderr = w
	}
}

func (c *altScreenCmd) Run() error {
	enter, leave := ansi.SetModeAltScreenSaveCursor, ansi.ResetModeAltScreenSaveCursor
	if c.kept {
		// Already on the alternate screen: stay on it, and go back to it
		// if the command leaves it, as vim does on quitting
		enter, leave = "", ansi.SetModeAltScreen
	}
	if c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, enter+ansi.CursorHomePosition+ansi.EraseEntireScreen)
		defer func() { _, _ = io.WriteString(c.Stdout, leave) }()
	}
	return c.Cmd.Run()
}
