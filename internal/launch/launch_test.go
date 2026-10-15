package launch

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// onPath is a lookPath that finds names alone.
func onPath(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		if slices.Contains(names, name) {
			return "/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}

func names(editors []Editor) string {
	out := make([]string, len(editors))
	for i, e := range editors {
		out[i] = e.Name
	}
	return strings.Join(out, " ")
}

// Yours come first, each in place of a preset of its name; then the
// presets on PATH, and a shell in a window always.
func TestAvailable(t *testing.T) {
	yours := []Editor{{Name: "nvim", Cmd: []string{"nvim", "-p", "."}, Mode: Inline}, {Name: "mine", Cmd: []string{"mine"}}}
	got := Available(yours, onPath("cursor", "nvim", "claude"))
	if want := "nvim mine cursor claude terminal"; names(got) != want {
		t.Errorf("available %q, want %q", names(got), want)
	}
	if !slices.Equal(got[0].Cmd, yours[0].Cmd) {
		t.Errorf("nvim is the preset's, not yours: %v", got[0].Cmd)
	}
	if got := Available([]Editor{{Name: "broken"}}, onPath("subl")); names(got) != "subl terminal" {
		t.Errorf("with problems, yours are taken: %q", names(got))
	}
}

// Pick takes the editor named; else the one $VISUAL or $EDITOR runs;
// else the first.
func TestPick(t *testing.T) {
	editors := Available(nil, onPath("cursor", "nvim", "vim"))
	env := func(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }
	tests := []struct {
		name string
		vars map[string]string
		want string
		ok   bool
	}{
		{name: "vim", want: "vim", ok: true},
		{name: "nope", ok: false},
		{vars: map[string]string{"EDITOR": "/opt/homebrew/bin/nvim -p"}, want: "nvim", ok: true},
		{vars: map[string]string{"VISUAL": "vim", "EDITOR": "nvim"}, want: "vim", ok: true},
		{vars: map[string]string{"EDITOR": "emacs"}, want: "cursor", ok: true},
	}
	for _, tt := range tests {
		got, ok := Pick(editors, tt.name, env(tt.vars))
		if ok != tt.ok || got.Name != tt.want {
			t.Errorf("Pick(%q, %v) = %q, %v; want %q, %v", tt.name, tt.vars, got.Name, ok, tt.want, tt.ok)
		}
	}
	if _, ok := Pick(nil, "", env(nil)); ok {
		t.Error("picked from none")
	}
}

func TestCheck(t *testing.T) {
	got := Check([]Editor{
		{Name: "a", Cmd: []string{"a"}},
		{Cmd: []string{"b"}},
		{Name: "a", Cmd: []string{"a"}},
		{Name: "c", Cmd: []string{"c"}, Mode: "split"},
		{Name: "d"},
		{Name: "shell", Mode: Window},
	})
	want := []string{
		"editors[1]: has no name",
		`editor "a": is given twice`,
		`editor "c": mode "split" isn't gui, inline, window or tab`,
		`editor "d": has no cmd, which only a window or tab may leave out (a shell there)`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("problems\n %q\nwant\n %q", got, want)
	}
}

func TestArgs(t *testing.T) {
	e := Editor{Cmd: []string{"code", "--goto", "{path}/README.md"}}
	if got := e.Args("/r/my api"); !slices.Equal(got, []string{"code", "--goto", "/r/my api/README.md"}) {
		t.Errorf("args %q", got)
	}
}

// Windows open by config.json's terminal command when it has one, in tmux
// inside tmux, then in the macOS terminal apps git-repos can script.
func TestDetectOpener(t *testing.T) {
	env := func(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }
	tests := []struct {
		template []string
		vars     map[string]string
		goos     string
		want     string
	}{
		{template: []string{"/usr/bin/wezterm", "start"}, vars: map[string]string{"TMUX": "x"}, goos: "darwin", want: "wezterm"},
		{vars: map[string]string{"TMUX": "x", "TERM_PROGRAM": "ghostty"}, goos: "darwin", want: "tmux"},
		{vars: map[string]string{"TERM_PROGRAM": "ghostty"}, goos: "darwin", want: "Ghostty"},
		{vars: map[string]string{"TERM_PROGRAM": "iTerm.app"}, goos: "darwin", want: "iTerm2"},
		{vars: map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, goos: "darwin", want: "Terminal"},
		{vars: map[string]string{"TERM_PROGRAM": "vscode"}, goos: "darwin"},
		{vars: map[string]string{"TERM_PROGRAM": "ghostty"}, goos: "linux"},
	}
	for _, tt := range tests {
		o, ok := DetectOpener(tt.template, env(tt.vars), tt.goos)
		if ok != (tt.want != "") || o.Name != tt.want {
			t.Errorf("DetectOpener(%v, %v, %s) = %q, %v; want %q", tt.template, tt.vars, tt.goos, o.Name, ok, tt.want)
		}
	}
}

// A terminal command takes the repo and the editor's command word for
// word; a shell for none.
func TestTemplateArgv(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	tmpl := []string{"wezterm", "start", "--cwd", "{path}", "--", "{cmd}"}
	got := templateArgv(tmpl, Editor{Cmd: []string{"claude", "--resume"}}, "/r/my api")
	if want := []string{"wezterm", "start", "--cwd", "/r/my api", "--", "claude", "--resume"}; !slices.Equal(got, want) {
		t.Errorf("argv %q", got)
	}
	if got := templateArgv(tmpl, Editor{Name: Terminal}, "/r"); got[len(got)-1] != "/bin/zsh" {
		t.Errorf("a shell: %q", got)
	}
}

// tmux gets the command as words, which no shell splits, then a shell.
func TestTmuxArgv(t *testing.T) {
	got := tmuxArgv([]string{"/bin/claude", "a b"}, "/r/my api", "/bin")
	want := []string{"new-window", "-c", "/r/my api", "-e", "PATH=/bin", "sh", "-c", `"$@"; exec "${SHELL:-/bin/sh}"`, "sh", "/bin/claude", "a b"}
	if !slices.Equal(got, want) {
		t.Errorf("argv\n %q\nwant\n %q", got, want)
	}
	if got := tmuxArgv(nil, "/r", "/bin"); len(got) != 5 {
		t.Errorf("a shell alone: %q", got)
	}
}

// A handoff is read once, by a token git-repos made, within a minute.
func TestHandoff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", home)
	h := Handoff{Dir: "/r/my api", Args: []string{"/bin/claude"}, PATH: "/bin"}
	token, err := handOff(h)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(handoffPath(token)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("handoff file: %v, %v", info, err)
	}
	got, err := TakeHandoff(token)
	if err != nil || got.Dir != h.Dir || !slices.Equal(got.Args, h.Args) || got.PATH != h.PATH {
		t.Fatalf("took %+v, %v", got, err)
	}
	if _, err := TakeHandoff(token); err == nil {
		t.Error("taken twice")
	}
	if _, err := TakeHandoff("../../etc/passwd"); err == nil || !strings.Contains(err.Error(), "isn't a token") {
		t.Errorf("a path as a token: %v", err)
	}
	old, _ := handOff(h)
	past := time.Now().Add(-2 * handoffTTL)
	_ = os.Chtimes(handoffPath(old), past, past)
	if _, err := TakeHandoff(old); err == nil || !strings.Contains(err.Error(), "over a minute old") {
		t.Errorf("an old handoff: %v", err)
	}
	if _, err := os.Stat(handoffPath(old)); !os.IsNotExist(err) {
		t.Error("an old handoff is left behind")
	}
}

// A handoff no window took in time is swept away by the next.
func TestSweepHandoffs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", home)
	stale, _ := handOff(Handoff{Dir: "/r"})
	past := time.Now().Add(-2 * handoffTTL)
	_ = os.Chtimes(handoffPath(stale), past, past)
	fresh, _ := handOff(Handoff{Dir: "/r"})
	if _, err := os.Stat(handoffPath(stale)); !os.IsNotExist(err) {
		t.Error("a stale handoff is left behind")
	}
	if _, err := os.Stat(handoffPath(fresh)); err != nil {
		t.Errorf("the fresh handoff: %v", err)
	}
}

// Agents open in a tab where the terminal opens tabs, as Ghostty and
// iTerm2 do; in a window where it doesn't, as Terminal and tmux.
func TestTabs(t *testing.T) {
	claude := Editor{Name: "claude", Cmd: []string{"claude"}, Mode: Tab}
	shell := Editor{Name: Terminal, Mode: Window}
	env := func(term string) func(string) string {
		return func(k string) string {
			if k == "TERM_PROGRAM" {
				return term
			}
			return ""
		}
	}
	for term, want := range map[string]string{"ghostty": "tab", "iTerm.app": "tab", "Apple_Terminal": "window"} {
		o, _ := DetectOpener(nil, env(term), "darwin")
		if got := o.Kind(claude); got != want {
			t.Errorf("%s opens claude in a %s, want a %s", term, got, want)
		}
		if got := o.Kind(shell); got != "window" {
			t.Errorf("%s opens a shell in a %s", term, got)
		}
	}
	if o, _ := DetectOpener(nil, func(k string) string { return map[string]string{"TMUX": "x"}[k] }, "darwin"); o.Kind(claude) != "window" {
		t.Error("tmux opens a tab")
	}
	ghostty := scriptable["ghostty"]
	if s := ghostty.script(claude); !strings.Contains(s, "new tab in front window") {
		t.Errorf("Ghostty's script for a tab:\n%s", s)
	}
	if s := ghostty.script(shell); strings.Contains(s, "new tab") {
		t.Errorf("Ghostty's script for a window:\n%s", s)
	}
	if s := scriptable["Apple_Terminal"].script(claude); !strings.Contains(s, "do script") {
		t.Errorf("Terminal's script for a tab:\n%s", s)
	}
	for _, p := range presets {
		if (p.Name == "claude" || p.Name == "codex") && p.How() != Tab {
			t.Errorf("%s opens in a %s", p.Name, p.How())
		}
	}
	if d := claude.Describe(); d != "in a new terminal tab" {
		t.Errorf("Describe %q", d)
	}
}
