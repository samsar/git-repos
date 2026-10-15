package keys

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
)

func entry(name string, ks ...string) Entry { return Entry{Name: name, Keys: ks} }

// The defaults: one binding of each name, every one acting somewhere,
// each with a key but Back, which esc always does, and no key twice in a
// view.
func TestDefaults(t *testing.T) {
	d := Defaults()
	names := map[string]bool{}
	for _, b := range d {
		if names[b.Name] || len(b.Views) == 0 || b.Label == "" || (len(b.Defaults) == 0) != (b.Name == Back) || !slices.Equal(b.Keys, b.Defaults) || b.Yours {
			t.Errorf("binding %+v", b)
		}
		names[b.Name] = true
	}
	if c := Conflicts(d); len(c) > 0 {
		t.Errorf("the defaults conflict: %v", c)
	}
	d[0].Defaults[0] = "z"
	if Defaults()[0].Defaults[0] == "z" {
		t.Error("Defaults shares its slices")
	}
}

// Resolve gives each binding config.json's keys over its defaults, or
// says every problem with them, in order.
func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
		want    map[string]string // binding → its keys, when fine
		yours   []string
		errs    []string
	}{
		{name: "nothing", want: map[string]string{Search: "/", MoveDown: "j down"}},
		{name: "yours", entries: []Entry{entry(Search, "f"), entry(Delete, "ctrl+X"), entry(MoveDown, "j", "x")},
			want: map[string]string{Search: "f", Delete: "ctrl+x", MoveDown: "j x"}, yours: []string{Delete, MoveDown, Search}},
		{name: "none", entries: []Entry{entry(PullAll)}, want: map[string]string{PullAll: ""}, yours: []string{PullAll}},
		{name: "the defaults again", entries: []Entry{entry(Search, "/")}, want: map[string]string{Search: "/"}},
		{name: "a reserved key its binding always has", entries: []Entry{entry(Back, "esc", "h"), entry(Quit, "Q", "ctrl+c")},
			want: map[string]string{Back: "h", Quit: "Q"}, yours: []string{Back, Quit}},
		{name: "a swap", entries: []Entry{entry(Pull, "s"), entry(Shell, "p")},
			want: map[string]string{Pull: "s", Shell: "p"}, yours: []string{Pull, Shell}},
		{name: "a key that means one thing in each view", entries: []Entry{entry(Delete, "space")},
			want: map[string]string{Delete: "space", Toggle: "enter space"}, yours: []string{Delete}},

		{name: "an unknown name", entries: []Entry{entry("serch", "f")},
			errs: []string{`there is no "serch" to give keys to; did you mean search? (git repos keys lists them all)`}},
		{name: "a name written another way", entries: []Entry{entry("Move-Down", "x")},
			errs: []string{`there is no "Move-Down" to give keys to; did you mean move_down? (git repos keys lists them all)`}},
		{name: "a name like none", entries: []Entry{entry("launch", "x")},
			errs: []string{`there is no "launch" to give keys to (git repos keys lists them all)`}},
		{name: "keys git-repos doesn't take", entries: []Entry{entry(Details, "alt+d", "x"), entry(Search, "g g")},
			errs: []string{`details: "alt+d" isn't a key git-repos takes: macOS terminals type a character for Option+x (≈) unless set to send Esc+, so git-repos takes no alt`,
				`search: "g g" isn't a key git-repos takes: git-repos takes one key at a time, never a sequence of them`}},
		{name: "a reserved key", entries: []Entry{entry(Details, "x", "esc"), entry(Search, "ctrl+c")},
			errs: []string{"details: esc always goes back, so nothing else takes it", "search: ctrl+c always quits, so nothing else takes it"}},
		{name: "a key given twice", entries: []Entry{entry(MoveDown, "x", "x")}, errs: []string{"move_down: x is given twice"}},
		{name: "a key bound twice in a view", entries: []Entry{entry(Pull, "x"), entry(Refresh, "x")},
			errs: []string{"x is both Pull repo and Refresh in the list and in the detail view"}},
		{name: "a key a default has", entries: []Entry{entry(Search, "p")},
			errs: []string{"p is both Search and Pull repo in the list"}},
		{name: "a default's key taken elsewhere", entries: []Entry{entry(Edit, "j")},
			errs: []string{"j is both Move down and Edit in settings"}},
		{name: "every problem, in order", entries: []Entry{entry("nope", "x"), entry(Search, "alt+a")},
			errs: []string{`there is no "nope" to give keys to (git repos keys lists them all)`,
				`search: "alt+a" isn't a key git-repos takes: macOS terminals type a character for Option+x (≈) unless set to send Esc+, so git-repos takes no alt`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, problems := Resolve(Defaults(), tt.entries)
			if !slices.Equal(problems, tt.errs) {
				t.Fatalf("problems:\n %q\nwant\n %q", problems, tt.errs)
			}
			if len(tt.errs) > 0 {
				return
			}
			var yours []string
			for _, b := range got {
				if want, ok := tt.want[b.Name]; ok && strings.Join(b.Keys, " ") != want {
					t.Errorf("%s takes %q, want %q", b.Name, b.Keys, want)
				}
				if b.Yours {
					yours = append(yours, b.Name)
				}
			}
			slices.Sort(yours)
			if !slices.Equal(yours, tt.yours) {
				t.Errorf("yours %v, want %v", yours, tt.yours)
			}
		})
	}
}

// FromConfig takes config.json's keys over the defaults, its problems in
// the order of their names, and the defaults alone when it has any.
func TestFromConfig(t *testing.T) {
	got, problems := FromConfig(map[string][]string{Search: {"f"}})
	if b, _ := find(got, Search); len(problems) > 0 || !slices.Equal(b.Keys, []string{"f"}) || !b.Yours {
		t.Errorf("search %+v, problems %q", b, problems)
	}
	got, problems = FromConfig(map[string][]string{Search: {"f"}, "zz": {"x"}, "aa": {"y"}})
	if len(problems) != 2 || !strings.Contains(problems[0], `"aa"`) || !strings.Contains(problems[1], `"zz"`) {
		t.Errorf("problems %q", problems)
	}
	if b, _ := find(got, Search); !slices.Equal(b.Keys, []string{"/"}) || b.Yours {
		t.Errorf("with problems, search %+v, want its default", b)
	}
}

// Rebinding gives a binding a key as its first: free, at once; another
// binding's there, as a swap of the two; refused, saying why, for a
// reserved key, its own, a key two others have, or a swap that would
// bind another key twice.
func TestRebinding(t *testing.T) {
	d := Defaults()
	tests := []struct {
		name, binding, key string
		bindings           []Binding
		set                map[string]string
		other              string
		err                string
	}{
		{name: "free", binding: Search, key: "f", set: map[string]string{Search: "f"}},
		{name: "keeps the rest", binding: MoveDown, key: "x", set: map[string]string{MoveDown: "x down"}},
		{name: "a key it has after its first", binding: MoveDown, key: "down", set: map[string]string{MoveDown: "down"}},
		{name: "a swap", binding: Pull, key: "s", set: map[string]string{Pull: "s", Shell: "p"}, other: Shell},
		{name: "a swap keeps the other's rest", binding: Search, key: "j", set: map[string]string{Search: "j", MoveDown: "/ down"}, other: MoveDown},
		{name: "from a binding with no key", binding: PullAll, key: "s",
			bindings: With(d, map[string][]string{PullAll: {}}, nil), set: map[string]string{PullAll: "s", Shell: ""}, other: Shell},
		{name: "a key free in its views", binding: OpenGitHub, key: "N", set: map[string]string{OpenGitHub: "N"}},
		{name: "its own", binding: Search, key: "/", err: "/ is already Search's key"},
		{name: "reserved", binding: Search, key: "esc", err: "esc always goes back, so nothing else takes it"},
		{name: "two others'", binding: Pull, key: "g", err: "g is already Top in the list and Open in GitHub in the detail view: pick another key"},
		{name: "a swap that binds another twice", binding: Search, key: "x",
			bindings: With(d, map[string][]string{Details: {"x"}, Search: {"p"}}, nil),
			err:      "that would leave p bound to both Detail view and Pull repo in the list"},
		{name: "unknown", binding: "nope", key: "x", err: `there is no "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bindings := tt.bindings
			if bindings == nil {
				bindings = d
			}
			set, other, err := Rebinding(bindings, tt.binding, tt.key)
			if tt.err != "" {
				if !errors.Is(err, ErrRebind) || err.Error() != ErrRebind.Error()+": "+tt.err {
					t.Fatalf("err %v, want %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for name, ks := range set {
				got[name] = strings.Join(ks, " ")
			}
			if !maps.Equal(got, tt.set) {
				t.Errorf("set %v, want %v", got, tt.set)
			}
			if (other == nil) != (tt.other == "") || other != nil && other.Binding.Name != tt.other {
				t.Errorf("other %+v, want %q", other, tt.other)
			}
			if c := Conflicts(With(bindings, set, nil)); len(c) > 0 {
				t.Errorf("the rebinding conflicts: %v", c)
			}
		})
	}
}

// Resetting puts a binding back to its defaults alone when that binds no
// key twice; with the one binding that has its default now, when that one
// going back too binds none (a swap, undone); and refuses otherwise,
// saying what the default would leave.
func TestResetting(t *testing.T) {
	d := Defaults()
	tests := []struct {
		name, binding string
		set           map[string][]string
		reset         []string
		key, err      string
	}{
		{name: "alone", binding: Search, set: map[string][]string{Search: {"f"}}, reset: []string{Search}},
		{name: "at its defaults", binding: Search, reset: []string{Search}},
		{name: "a swap, undone", binding: Pull, set: map[string][]string{Pull: {"s"}, Shell: {"p"}},
			reset: []string{Pull, Shell}, key: "p"},
		{name: "the other way", binding: Shell, set: map[string][]string{Pull: {"s"}, Shell: {"p"}},
			reset: []string{Shell, Pull}, key: "s"},
		{name: "a default of several", binding: First, set: map[string][]string{First: {"x"}, Last: {"g", "home"}},
			reset: []string{First, Last}, key: "g"},
		{name: "the other's default taken in turn", binding: Pull,
			set: map[string][]string{Pull: {"s"}, Shell: {"z"}, Settings: {"p"}},
			err: "default would leave p bound to both Pull repo and Settings in the list and in the detail view: rebind the other first"},
		{name: "the other's free default", binding: Details, set: map[string][]string{Details: {"x"}, Search: {"enter"}},
			reset: []string{Details, Search}, key: "enter"},
		{name: "two others", binding: Pull, set: map[string][]string{Pull: {"x"}, Shell: {"p"}, Settings: {"p"}},
			err: "default would leave p bound to Pull repo, Open shell and Settings in the list and in the detail view: rebind the other first"},
		{name: "unknown", binding: "nope", err: `there is no "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bindings := With(d, tt.set, nil)
			reset, key, err := Resetting(bindings, tt.binding)
			if tt.err != "" {
				if !errors.Is(err, ErrReset) || err.Error() != ErrReset.Error()+": "+tt.err || reset != nil {
					t.Fatalf("reset %v, err %v, want %q", reset, err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(reset, tt.reset) || key != tt.key {
				t.Errorf("reset %v, key %q; want %v, %q", reset, key, tt.reset, tt.key)
			}
			if c := Conflicts(With(bindings, nil, reset)); len(c) > 0 {
				t.Errorf("the reset conflicts: %v", c)
			}
		})
	}
}

// Taken names who else has a key, where the binding asked about acts.
func TestTaken(t *testing.T) {
	d := Defaults()
	uses := Taken(d, Pull, "s")
	if len(uses) != 1 || uses[0].String() != "Open shell in the list and in the detail view" {
		t.Errorf("s for pull: %v", uses)
	}
	if uses := Taken(d, OpenGitHub, "N"); len(uses) != 0 {
		t.Errorf("N in the detail view: %v", uses)
	}
}

// Free is every key no binding has in a view, reserved ones left out.
func TestFree(t *testing.T) {
	d := Defaults()
	list := Free(d, ViewList)
	for _, k := range []string{"j", "p", "s", "N", "/", "enter", "esc", "ctrl+c", "ctrl+d"} {
		if slices.Contains(list, k) {
			t.Errorf("%q free in the list", k)
		}
	}
	for _, k := range []string{"f", "x", "tab", "ctrl+e"} {
		if !slices.Contains(list, k) {
			t.Errorf("%q isn't free in the list", k)
		}
	}
	if detail := Free(d, ViewDetail); !slices.Contains(detail, "N") || slices.Contains(detail, "g") {
		t.Error("N and g in the detail view")
	}
}

// A conflict of the same bindings in several views is one, naming them
// all; other bindings sharing the key in another view are another.
func TestConflictsAcrossViews(t *testing.T) {
	c := Conflicts(With(Defaults(), map[string][]string{Details: {"x"}, Search: {"x"}, Pull: {"x"}, OpenGitHub: {"x"}}, nil))
	said := make([]string, 0, len(c))
	for _, x := range c {
		said = append(said, x.String())
	}
	want := []string{"x is Detail view, Search and Pull repo in the list", "x is both Open in GitHub and Pull repo in the detail view"}
	if !slices.Equal(said, want) {
		t.Errorf("conflicts %q, want %q", said, want)
	}
}
