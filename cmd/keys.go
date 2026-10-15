package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/samsar/git-repos/internal/config"
	"github.com/samsar/git-repos/internal/keys"
	"github.com/spf13/cobra"
)

var (
	flagKeysJSON  bool
	flagKeysPlain bool
	flagKeysFree  bool
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "List the TUI's keys, view by view, yours from config.json marked",
	Long: `List every key the TUI takes, view by view: the list, the detail view and
settings. Each binding is named as config.json's keys names it, and keys of
yours are marked, with their defaults. When config.json's keys have
problems, they are listed, and the keys shown are the defaults, which the
TUI uses then. --free lists, instead, the keys still free in each view.
--plain prints one tab-separated line per key: view, key, name, and yours,
default or always (a reserved key); with --free, view and key. --json prints
them as JSON.

Change them in config.json:

  "keys": {
    "search": "f",
    "pull_all": "ctrl+u",
    "move_down": ["j", "down"]
  }

A key is a character (a, A, ?), a named key (enter, tab, up, f1), or ctrl
with a letter (ctrl+d); never alt, and never a sequence. ctrl+c always quits
and esc always goes back. git repos keys check checks a file.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.Path()
		cfg, err := config.LoadFrom(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		bindings, problems := keys.FromConfig(cfg.KeyMap())
		out := cmd.OutOrStdout()
		switch {
		case flagKeysFree && flagKeysJSON:
			return printJSON(out, freeByView(bindings))
		case flagKeysFree && flagKeysPlain:
			printFreePlain(out, bindings)
			return nil
		case flagKeysFree:
			return printFree(out, bindings)
		case flagKeysJSON:
			return printJSON(out, keysListing{bindings, keys.ReservedKeys(), path, cfg != nil, problems})
		case flagKeysPlain:
			printKeysPlain(out, bindings)
			return nil
		}
		return printKeys(out, bindings, path, cfg != nil, problems)
	},
}

// errProblems: a file checked has problems, each printed already.
var errProblems = errors.New("problems found")

var keysCheckCmd = &cobra.Command{
	Use:   "check [file]",
	Short: "Check a config.json's keys: every problem",
	Long: `Check a config.json's keys: file, or the config file git-repos reads. Every
problem is printed as file: problem: JSON git-repos can't read, a name it
doesn't have, a key it doesn't take, a reserved key (ctrl+c always quits,
esc always goes back), and a key bound twice in one view. It exits 1 on any.`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, given := config.Path(), len(args) == 1
		if given {
			path = args[0]
		}
		out := cmd.OutOrStdout()
		cfg, err := config.LoadFrom(path)
		switch {
		case err != nil:
			fmt.Fprintf(out, "%s: %v\n", path, err)
			return errProblems
		case cfg == nil && given:
			fmt.Fprintf(out, "there is no %s\n", path)
			return errProblems
		case cfg == nil:
			fmt.Fprintf(out, "%s: none yet, so git-repos takes its default keys\n", path)
			return nil
		}
		bindings, problems := keys.FromConfig(cfg.KeyMap())
		if len(problems) > 0 {
			for _, p := range problems {
				fmt.Fprintf(out, "%s: %s\n", path, p)
			}
			return errProblems
		}
		fmt.Fprintf(out, "%s: ok, %s\n", path, yoursWords(bindings))
		return nil
	},
}

func init() {
	keysCmd.Flags().BoolVar(&flagKeysJSON, "json", false, "print the keys as JSON")
	keysCmd.Flags().BoolVar(&flagKeysPlain, "plain", false, "print one tab-separated line per key")
	keysCmd.Flags().BoolVar(&flagKeysFree, "free", false, "list the keys still free in each view")
	keysCmd.MarkFlagsMutuallyExclusive("json", "plain")
	keysCmd.AddCommand(keysCheckCmd)
}

// keysListing is git repos keys --json: every binding, the reserved keys,
// and where yours come from.
type keysListing struct {
	Bindings []keys.Binding     `json:"bindings"`
	Reserved []keys.ReservedKey `json:"reserved"`
	Config   string             `json:"config"`
	Exists   bool               `json:"exists"`
	Problems []string           `json:"problems,omitempty"`
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// yoursWords says how many bindings are given keys of yours.
func yoursWords(bindings []keys.Binding) string {
	n := 0
	for _, b := range bindings {
		if b.Yours {
			n++
		}
	}
	switch n {
	case 0:
		return "with no keys of yours"
	case 1:
		return "1 binding with keys of yours"
	}
	return fmt.Sprintf("%d bindings with keys of yours", n)
}

// printKeys is each view under its heading: each binding there, its keys,
// its name and what it does, yours marked with their defaults, a key it
// shares with another there marked; then the reserved keys, and where
// the keys come from.
func printKeys(w io.Writer, bindings []keys.Binding, path string, exists bool, problems []string) error {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	conflicts := keys.Conflicts(bindings)
	for i, v := range keys.Views {
		if i > 0 {
			fmt.Fprintln(tw)
		}
		fmt.Fprintln(tw, strings.ToUpper(keys.ViewWords(v)))
		for _, b := range keys.In(bindings, v) {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", keysText(b.Keys), b.Name, b.Label, bindingNote(b, v, conflicts))
		}
		for _, r := range keys.ReservedKeys() {
			fmt.Fprintf(tw, "  %s\t\t%s\talways\n", r.Key, r.Label)
		}
	}
	fmt.Fprintln(tw)
	switch {
	case len(problems) > 0:
		fmt.Fprintf(tw, "⚠ %s's keys have %d problem(s), so the keys above are the defaults:\n", path, len(problems))
		for _, p := range problems {
			fmt.Fprintf(tw, "  %s\n", p)
		}
	case !exists:
		fmt.Fprintf(tw, "%s: none yet, so these are git-repos's default keys\n", path)
	default:
		fmt.Fprintf(tw, "%s: %s\n", path, yoursWords(bindings))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return writeTrimmed(w, b.String())
}

// writeTrimmed writes text without the blanks a tabwriter pads a line's
// last, empty cells with.
func writeTrimmed(w io.Writer, text string) error {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	_, err := io.WriteString(w, strings.Join(lines, "\n"))
	return err
}

// keysText is keys as config.json names them; – for none.
func keysText(ks []string) string {
	if len(ks) == 0 {
		return "–"
	}
	return strings.Join(ks, " ")
}

// bindingNote is what the listing says of b in view v: that its keys are
// yours, and its defaults; and each key it shares there with another.
func bindingNote(b keys.Binding, v keys.View, conflicts []keys.Conflict) string {
	var notes []string
	if b.Yours {
		notes = append(notes, "yours; "+keysText(b.Defaults)+" by default")
	}
	for _, c := range conflicts {
		if !slices.Contains(c.Views, v) || !slices.ContainsFunc(c.Bindings, func(o keys.Binding) bool { return o.Name == b.Name }) {
			continue
		}
		var others []string
		for _, o := range c.Bindings {
			if o.Name != b.Name {
				others = append(others, o.Label)
			}
		}
		notes = append(notes, fmt.Sprintf("⚠ %s is %s's too", c.Key, strings.Join(others, " and ")))
	}
	return strings.Join(notes, " · ")
}

// printKeysPlain is one tab-separated line per key: view, key, name, and
// yours, default or always.
func printKeysPlain(w io.Writer, bindings []keys.Binding) {
	for _, v := range keys.Views {
		for _, b := range keys.In(bindings, v) {
			source := "default"
			if b.Yours {
				source = "yours"
			}
			for _, k := range b.Keys {
				fmt.Fprintln(w, strings.Join([]string{string(v), k, b.Name, source}, "\t"))
			}
		}
		for _, r := range keys.ReservedKeys() {
			fmt.Fprintln(w, strings.Join([]string{string(v), r.Key, strings.ToLower(r.Label), "always"}, "\t"))
		}
	}
}

// freeByView is the keys free in each view, by view.
func freeByView(bindings []keys.Binding) map[keys.View][]string {
	out := map[keys.View][]string{}
	for _, v := range keys.Views {
		out[v] = keys.Free(bindings, v)
	}
	return out
}

// printFree is each view's free keys under its heading, by kind.
func printFree(w io.Writer, bindings []keys.Binding) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for i, v := range keys.Views {
		if i > 0 {
			fmt.Fprintln(tw)
		}
		fmt.Fprintln(tw, strings.ToUpper(keys.ViewWords(v)))
		byKind := map[string][]string{}
		for _, k := range keys.Free(bindings, v) {
			byKind[kindOf(k)] = append(byKind[kindOf(k)], k)
		}
		for _, kind := range []string{"letters", "capitals", "digits", "symbols", "named", "ctrl"} {
			if ks := byKind[kind]; len(ks) > 0 {
				fmt.Fprintf(tw, "  %s\t%s\n", kind, strings.Join(ks, " "))
			}
		}
	}
	return tw.Flush()
}

// kindOf is which kind of key k is, as --free groups them.
func kindOf(k string) string {
	switch c := k[0]; {
	case strings.HasPrefix(k, "ctrl+"):
		return "ctrl"
	case len(k) > 1:
		return "named"
	case c >= 'a' && c <= 'z':
		return "letters"
	case c >= 'A' && c <= 'Z':
		return "capitals"
	case c >= '0' && c <= '9':
		return "digits"
	}
	return "symbols"
}

// printFreePlain is one tab-separated line per free key: view, key.
func printFreePlain(w io.Writer, bindings []keys.Binding) {
	for _, v := range keys.Views {
		for _, k := range keys.Free(bindings, v) {
			fmt.Fprintln(w, string(v)+"\t"+k)
		}
	}
}
