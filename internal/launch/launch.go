// Package launch opens a repo in an editor, an agent or a terminal: as a
// GUI app beside git-repos, here in its terminal while git-repos waits, or
// in a new terminal tab or window.
package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Mode is how an editor opens a repo.
type Mode string

const (
	// GUI starts the editor as a process of its own, detached: git-repos
	// keeps running, and quitting it leaves the editor open.
	GUI Mode = "gui"
	// Inline runs the editor in git-repos's own terminal, git-repos
	// waiting until it exits: for vim and the like.
	Inline Mode = "inline"
	// Window runs the editor in a new terminal window, in the repo,
	// leaving a shell there once it exits: for a shell.
	Window Mode = "window"
	// Tab is Window in a new tab of the window git-repos runs in, where
	// the terminal can open one: for claude, codex and other agents.
	Tab Mode = "tab"
)

// Editor is one way to open a repo: config.json's editors, or a preset.
type Editor struct {
	Name string `json:"name"`
	// Cmd is the command, {path} standing for the repo's directory, which
	// is also where it runs. Empty for a window: a shell there.
	Cmd  []string `json:"cmd,omitempty"`
	Mode Mode     `json:"mode,omitempty"`
}

// How is e's mode: GUI unless it says otherwise.
func (e Editor) How() Mode {
	if e.Mode == "" {
		return GUI
	}
	return e.Mode
}

// InTerminal reports whether e opens in a new terminal tab or window.
func (e Editor) InTerminal() bool { return e.How() == Window || e.How() == Tab }

// Args is e's command for the repo at path.
func (e Editor) Args(path string) []string {
	out := make([]string, len(e.Cmd))
	for i, a := range e.Cmd {
		out[i] = strings.ReplaceAll(a, "{path}", path)
	}
	return out
}

// presets are the editors git-repos knows, in the order it prefers them
// when you haven't picked one: each is offered when its command is on
// PATH; Terminal, a shell in a new window, always is.
var presets = []Editor{
	{Name: "cursor", Cmd: []string{"cursor", "{path}"}},
	{Name: "code", Cmd: []string{"code", "{path}"}},
	{Name: "zed", Cmd: []string{"zed", "{path}"}},
	{Name: "subl", Cmd: []string{"subl", "{path}"}},
	{Name: "claude", Cmd: []string{"claude"}, Mode: Tab},
	{Name: "codex", Cmd: []string{"codex"}, Mode: Tab},
	{Name: "nvim", Cmd: []string{"nvim", "."}, Mode: Inline},
	{Name: "vim", Cmd: []string{"vim", "."}, Mode: Inline},
	{Name: Terminal, Mode: Window},
}

// Terminal is the preset that opens a shell in a new window.
const Terminal = "terminal"

// Check is each problem with yours, config.json's editors: a name
// missing or given twice, a mode git-repos doesn't have, or no command
// for one that isn't a window.
func Check(yours []Editor) []string {
	var problems []string
	seen := map[string]bool{}
	for i, e := range yours {
		at := fmt.Sprintf("editors[%d]", i)
		if e.Name != "" {
			at = fmt.Sprintf("editor %q", e.Name)
		}
		switch {
		case e.Name == "":
			problems = append(problems, at+": has no name")
		case seen[e.Name]:
			problems = append(problems, at+": is given twice")
		case !slices.Contains([]Mode{"", GUI, Inline, Window, Tab}, e.Mode):
			problems = append(problems, fmt.Sprintf("%s: mode %q isn't gui, inline, window or tab", at, e.Mode))
		case len(e.Cmd) == 0 && !e.InTerminal():
			problems = append(problems, at+": has no cmd, which only a window or tab may leave out (a shell there)")
		}
		seen[e.Name] = true
	}
	return problems
}

// Available is every editor a repo can open in: yours first, each in
// place of a preset of its name, then the presets whose command lookPath
// finds. Editors with problems (Check) are left out.
func Available(yours []Editor, lookPath func(string) (string, error)) []Editor {
	var out []Editor
	if len(Check(yours)) == 0 {
		out = slices.Clone(yours)
	}
	for _, p := range presets {
		if slices.ContainsFunc(out, func(e Editor) bool { return e.Name == p.Name }) {
			continue
		}
		if len(p.Cmd) > 0 {
			if _, err := lookPath(p.Cmd[0]); err != nil {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// Pick is the editor e opens: the one named name; else, without a name,
// the one $VISUAL or $EDITOR runs; else the first. False when there is
// none, or none named name.
func Pick(editors []Editor, name string, getenv func(string) string) (Editor, bool) {
	find := func(n string) (Editor, bool) {
		i := slices.IndexFunc(editors, func(e Editor) bool { return e.Name == n })
		if i < 0 {
			return Editor{}, false
		}
		return editors[i], true
	}
	if name != "" {
		return find(name)
	}
	for _, v := range []string{"VISUAL", "EDITOR"} {
		if fields := strings.Fields(getenv(v)); len(fields) > 0 {
			if e, ok := find(filepath.Base(fields[0])); ok {
				return e, true
			}
		}
	}
	if len(editors) == 0 {
		return Editor{}, false
	}
	return editors[0], true
}

// Describe says how e opens a repo, as the picker and git repos editors
// say it.
func (e Editor) Describe() string {
	switch {
	case e.How() == Inline:
		return "here, in this terminal"
	case e.InTerminal() && len(e.Cmd) == 0:
		return "a shell in a new terminal " + string(e.How())
	case e.InTerminal():
		return "in a new terminal " + string(e.How())
	}
	return "as an app"
}

// StartGUI starts e on the repo at path, detached: no terminal, a process
// group of its own, so it outlives git-repos.
func StartGUI(e Editor, path string) error {
	args := e.Args(path)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = path
	return startDetached(cmd)
}

// earlyExit is how long a detached start watches the program for a
// failure before taking it as started.
const earlyExit = time.Second

// startDetached starts cmd detached and watches it for earlyExit: one
// that exits with an error by then failed, and the error says why, from
// the first line of its stderr; one still running, or done without an
// error (a CLI handing the repo to its app), started.
//
// Its stderr is a file, never a pipe git-repos reads: a pipe would lose
// its reader as git-repos quits, and a program writing to it then would
// die of SIGPIPE. The file is removed as the watch ends; a program still
// running writes on to it unseen, and its space comes back as it exits.
func startDetached(cmd *exec.Cmd) error {
	detach(cmd)
	errFile, err := os.CreateTemp("", "git-repos-start-*.log")
	if err != nil {
		return err
	}
	defer os.Remove(errFile.Name())
	defer errFile.Close()
	cmd.Stderr = errFile
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err == nil {
			return nil
		}
		head := make([]byte, 4096)
		n, _ := errFile.ReadAt(head, 0)
		return fmt.Errorf("%s: %s", filepath.Base(cmd.Args[0]), firstLine(string(head[:n]), err))
	case <-time.After(earlyExit):
		return nil
	}
}

// InlineCmd is the command running e in the repo at path, for git-repos
// to run in its own terminal.
func InlineCmd(e Editor, path string) *exec.Cmd {
	args := e.Args(path)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = path
	return cmd
}
