package tui

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A screen that keeps the alternate screen up drops Bubble Tea's leaving
// of it and its coming back, once each, and passes everything else.
func TestScreenKeepsAltScreenUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	s := newScreen(f)

	writes := []string{
		ansi.ResetModeAltScreenSaveCursor + "a", // as is: not kept yet
		"",                                      // keepAltScreen
		"b" + ansi.ResetModeAltScreenSaveCursor, // Bubble Tea leaves: dropped
		ansi.SetModeAltScreenSaveCursor + "c",   // and comes back: dropped
		ansi.ResetModeAltScreenSaveCursor + "d", // quitting: as is
	}
	for _, w := range writes {
		if w == "" {
			s.keepAltScreen()
			continue
		}
		if n, err := s.Write([]byte(w)); err != nil || n != len(w) {
			t.Fatalf("Write(%q) = %d, %v", w, n, err)
		}
	}
	_ = f.Close()

	got, _ := os.ReadFile(path)
	want := ansi.ResetModeAltScreenSaveCursor + "a" + "b" + "c" + ansi.ResetModeAltScreenSaveCursor + "d"
	if string(got) != want {
		t.Errorf("terminal got %q, want %q", got, want)
	}
}

// A shell or editor run on git-repos's kept screen clears it, and goes
// back to it on exit in case it left, without saving or restoring the
// cursor of the screen git-repos started from.
func TestAltScreenCmdOnKeptScreen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	c := &altScreenCmd{Cmd: exec.Command("echo", "hi")}
	c.SetStdout(newScreen(f))

	if c.Stdout != f {
		t.Fatalf("command's stdout = %v, want the terminal itself", c.Stdout)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	got, _ := os.ReadFile(path)
	want := ansi.CursorHomePosition + ansi.EraseEntireScreen + "hi\n" + ansi.SetModeAltScreen
	if string(got) != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// Without git-repos's screen kept up, a shell or editor gets an alternate
// screen of its own, and leaves it on exit.
func TestAltScreenCmdOnItsOwn(t *testing.T) {
	var out bytes.Buffer
	c := &altScreenCmd{Cmd: exec.Command("echo", "hi")}
	c.SetStdout(&out)

	if err := c.Run(); err != nil {
		t.Fatal(err)
	}

	want := ansi.SetModeAltScreenSaveCursor + ansi.CursorHomePosition + ansi.EraseEntireScreen + "hi\n" + ansi.ResetModeAltScreenSaveCursor
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}
