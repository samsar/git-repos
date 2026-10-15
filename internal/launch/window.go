package launch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ErrNoWindow: git-repos can't open a window in the terminal it runs in,
// and config.json names no terminal command to do it.
var ErrNoWindow = errors.New("can't open a terminal window here")

// Opener opens a new terminal window running an editor in a repo: the
// terminal git-repos runs in, or config.json's terminal command.
type Opener struct {
	// Name is the terminal, as a status line says it: Ghostty, tmux…
	Name string
	open func(ctx context.Context, e Editor, path string) error
}

// Open opens a window running e in the repo at path.
func (o Opener) Open(ctx context.Context, e Editor, path string) error {
	return o.open(ctx, e, path)
}

// resolved is e's command for path, its program found on git-repos's own
// PATH: a window's environment is its terminal app's, whose PATH may not
// reach it. None for a shell.
func resolved(e Editor, path string) ([]string, error) {
	args := e.Args(path)
	if len(args) == 0 {
		return nil, nil
	}
	p, err := exec.LookPath(args[0])
	if err != nil {
		return nil, err
	}
	args[0] = p
	return args, nil
}

// DetectOpener is how git-repos opens windows: with template, config.json's
// terminal command, when it has one; else in tmux, when git-repos runs in
// it; else, on macOS, in Ghostty, iTerm2 or Terminal, the one it runs in.
// False for any other terminal.
func DetectOpener(template []string, getenv func(string) string, goos string) (Opener, bool) {
	switch {
	case len(template) > 0:
		return Opener{Name: filepath.Base(template[0]), open: func(_ context.Context, e Editor, path string) error {
			return startTemplate(template, e, path)
		}}, true
	case getenv("TMUX") != "":
		return Opener{Name: "tmux", open: tmuxWindow}, true
	case goos != "darwin":
		return Opener{}, false
	}
	app, ok := scriptable[getenv("TERM_PROGRAM")]
	if !ok {
		return Opener{}, false
	}
	return Opener{Name: app.name, open: func(ctx context.Context, e Editor, path string) error {
		return scriptWindow(ctx, app, e, path)
	}}, true
}

// shellArgs is e's command for path, or the user's shell for none.
func shellArgs(e Editor, path string) []string {
	if args := e.Args(path); len(args) > 0 {
		return args
	}
	return []string{loginShell()}
}

func loginShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// startTemplate runs config.json's terminal command, {path} the repo's
// directory and {cmd} the editor's command, word for word: nothing goes
// through a shell.
func startTemplate(template []string, e Editor, path string) error {
	argv := templateArgv(template, e, path)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = path
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// templateArgv is config.json's terminal command for e in the repo at
// path: {cmd} the editor's command (the user's shell for none), {path}
// the repo's directory.
func templateArgv(template []string, e Editor, path string) []string {
	var argv []string
	for _, a := range template {
		if a == "{cmd}" {
			argv = append(argv, shellArgs(e, path)...)
			continue
		}
		argv = append(argv, strings.ReplaceAll(a, "{path}", path))
	}
	return argv
}

// tmuxWindow opens a tmux window in the repo running the editor.
func tmuxWindow(ctx context.Context, e Editor, path string) error {
	args, err := resolved(e, path)
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, "tmux", tmuxArgv(args, path, os.Getenv("PATH"))...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// tmuxArgv is tmux's arguments for a window in the repo at path running
// args with PATH, then a shell there; a shell alone for no args. tmux runs
// a command of several words as they are, never through a shell, so sh's
// "$@" carries them.
func tmuxArgv(args []string, path, PATH string) []string {
	argv := []string{"new-window", "-c", path, "-e", "PATH=" + PATH}
	if len(args) > 0 {
		argv = append(argv, "sh", "-c", `"$@"; exec "${SHELL:-/bin/sh}"`, "sh")
		argv = append(argv, args...)
	}
	return argv
}

// scriptApp is a terminal app git-repos opens windows in by AppleScript.
type scriptApp struct {
	name, script string
}

// scriptable are the terminal apps that open windows by AppleScript, by
// their TERM_PROGRAM. Each script takes the command line as item 1 of
// argv, never as part of its source.
var scriptable = map[string]scriptApp{
	"ghostty": {"Ghostty", `on run argv
	tell application id "com.mitchellh.ghostty"
		new window with configuration {command:item 1 of argv}
		activate
	end tell
end run`},
	"iTerm.app": {"iTerm2", `on run argv
	tell application id "com.googlecode.iterm2"
		create window with default profile command (item 1 of argv)
		activate
	end tell
end run`},
	"Apple_Terminal": {"Terminal", `on run argv
	tell application id "com.apple.Terminal"
		do script (item 1 of argv)
		activate
	end tell
end run`},
}

// plainPath is a path a terminal's command line takes as one word,
// unchanged: iTerm2 splits a command into words itself, and Ghostty and
// Terminal run one through a shell.
var plainPath = regexp.MustCompile(`^/[A-Za-z0-9._+@/-]+$`)

// scriptWindow opens a window of app running git-repos itself, by its
// absolute path, with a token: git repos launch <token> reads the repo
// and the editor's command from the file the token names, which only it
// reads, once. So no path or command reaches AppleScript or a shell.
func scriptWindow(ctx context.Context, app scriptApp, e Editor, path string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !plainPath.MatchString(self) {
		return fmt.Errorf("git-repos is at %q, which a terminal's command line can't take as it is", self)
	}
	args, err := resolved(e, path)
	if err != nil {
		return err
	}
	token, err := handOff(Handoff{Dir: path, Args: args, PATH: os.Getenv("PATH")})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	argv := []string{}
	for line := range strings.Lines(app.script) {
		argv = append(argv, "-e", strings.TrimSuffix(line, "\n"))
	}
	argv = append(argv, self+" launch "+token)
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", argv...).CombinedOutput()
	if err != nil {
		_ = os.Remove(handoffPath(token))
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "-1743") {
			return fmt.Errorf("macOS didn't let git-repos script %s: allow it in System Settings › Privacy & Security › Automation", app.name)
		}
		return fmt.Errorf("%s: %s", app.name, msg)
	}
	return nil
}

// Handoff is what git repos launch runs: the editor's command (none for
// a shell), in the repo's directory, with git-repos's PATH.
type Handoff struct {
	Dir  string   `json:"dir"`
	Args []string `json:"args,omitempty"`
	PATH string   `json:"path,omitempty"`
}

// handoffTTL is how long a handoff waits for its window to read it.
const handoffTTL = time.Minute

var tokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// handoffDir is where handoffs wait: the user's cache, private to them.
func handoffDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "git-repos", "launch")
}

func handoffPath(token string) string { return filepath.Join(handoffDir(), token+".json") }

// handOff writes h where git repos launch <token> reads it, readable by
// the user alone, and returns the token.
func handOff(h Handoff) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	if err := os.MkdirAll(handoffDir(), 0o700); err != nil {
		return "", err
	}
	data, err := json.Marshal(h)
	if err != nil {
		return "", err
	}
	return token, os.WriteFile(handoffPath(token), data, 0o600)
}

// TakeHandoff reads and removes the handoff token names: an error for a
// token git-repos didn't make, none waiting, or one older than a minute.
func TakeHandoff(token string) (Handoff, error) {
	if !tokenPattern.MatchString(token) {
		return Handoff{}, fmt.Errorf("%q isn't a token git-repos made", token)
	}
	path := handoffPath(token)
	info, err := os.Stat(path)
	if err != nil {
		return Handoff{}, fmt.Errorf("nothing to launch for %s: it was launched already, or never handed off", token)
	}
	data, err := os.ReadFile(path)
	_ = os.Remove(path)
	if err != nil {
		return Handoff{}, err
	}
	if time.Since(info.ModTime()) > handoffTTL {
		return Handoff{}, errors.New("the handoff is over a minute old; open it from git-repos again")
	}
	var h Handoff
	if err := json.Unmarshal(data, &h); err != nil {
		return Handoff{}, err
	}
	return h, nil
}

// Run runs h in a window opened for it: its command in its directory,
// then a shell there, which it becomes, so the window stays a terminal in
// the repo. A command that fails says why above the shell.
func Run(h Handoff) error {
	if err := os.Chdir(h.Dir); err != nil {
		return err
	}
	if h.PATH != "" {
		_ = os.Setenv("PATH", h.PATH)
	}
	if len(h.Args) > 0 {
		if err := runForeground(h.Args); err != nil {
			fmt.Fprintf(os.Stderr, "git-repos: %s: %v\n", h.Args[0], err)
		}
	}
	return execShell(loginShell(), slices.Clone(os.Environ()))
}
