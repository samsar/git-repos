package keys

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Entry is one binding config.json gives keys: its name, and its keys.
type Entry struct {
	Name string
	Keys []string
}

// Resolve is defaults with the keys entries give in place of theirs, or
// every problem with entries, in their order: an unknown name, a key
// git-repos doesn't take, a reserved key, a key given twice to one
// binding, and a key two bindings take in one view. With any, the
// bindings returned aren't to be used.
func Resolve(defaults []Binding, entries []Entry) ([]Binding, []string) {
	out := make([]Binding, len(defaults))
	for i, b := range defaults {
		out[i] = defaulted(b)
	}
	var problems []string
	for _, e := range entries {
		i := slices.IndexFunc(out, func(b Binding) bool { return b.Name == e.Name })
		if i < 0 {
			problems = append(problems, unknownName(e.Name, defaults))
			continue
		}
		keys, ps := entryKeys(e)
		problems = append(problems, ps...)
		out[i].Keys, out[i].Yours = keys, !slices.Equal(keys, out[i].Defaults)
	}
	// Conflicts among what the entries give, the keys of the others among
	// them: they are problems too, whatever else is.
	for _, c := range Conflicts(out) {
		problems = append(problems, c.String())
	}
	return out, problems
}

// entryKeys is the keys e gives its binding, as git-repos keeps them, and
// the problems with them. A reserved key is dropped from the binding it
// always does, which may list it.
func entryKeys(e Entry) ([]string, []string) {
	keys := []string{}
	var problems []string
	for _, k := range e.Keys {
		key, err := Parse(k)
		switch {
		case err != nil:
			problems = append(problems, e.Name+": "+err.Error())
		case Reserved(key) && alwaysHas[key] == e.Name:
		case Reserved(key):
			problems = append(problems, e.Name+": "+reservedWhy(key))
		case slices.Contains(keys, key):
			problems = append(problems, fmt.Sprintf("%s: %s is given twice", e.Name, key))
		default:
			keys = append(keys, key)
		}
	}
	return keys, problems
}

// reservedWhy says why no binding takes key, a reserved one.
func reservedWhy(key string) string {
	if key == CtrlC {
		return "ctrl+c always quits, so nothing else takes it"
	}
	return "esc always goes back, so nothing else takes it"
}

// unknownName says that config.json names a binding git-repos doesn't
// have, and which one it may mean.
func unknownName(name string, bindings []Binding) string {
	msg := fmt.Sprintf("there is no %q to give keys to", name)
	if near := nearest(name, bindings); near != "" {
		msg += "; did you mean " + near + "?"
	}
	return msg + " (git repos keys lists them all)"
}

// nearest is the binding name closest to name, written another way or a
// letter or two from it (one, for a short name); "" for none that close.
func nearest(name string, bindings []Binding) string {
	norm := strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(name))
	best, bestDist := "", min(max(len(norm)/3, 1), 2)+1
	for _, b := range bindings {
		if b.Name == norm {
			return b.Name
		}
		if d := distance(norm, b.Name); d < bestDist {
			best, bestDist = b.Name, d
		}
	}
	return best
}

// distance is the Levenshtein distance between a and b.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// Conflict is a key bound twice in one view: the bindings that share it,
// and the views where they do.
type Conflict struct {
	Key      string
	Bindings []Binding
	Views    []View
}

func (c Conflict) String() string { return c.Key + " is " + c.shared() }

// Bound says the conflict as what a change would leave: x bound to both
// Pull repo and Refresh in the list.
func (c Conflict) Bound() string { return c.Key + " bound to " + c.shared() }

// shared names the bindings that share the key, and where: both Pull repo
// and Refresh in the list.
func (c Conflict) shared() string {
	labels := make([]string, len(c.Bindings))
	for i, b := range c.Bindings {
		labels[i] = b.Label
	}
	both := ""
	if len(labels) == 2 {
		both = "both "
	}
	return both + joinWords(labels) + " " + ViewsWords(c.Views)
}

// Conflicts is every key bound twice in one view, by key, each with the
// views where the same bindings share it.
func Conflicts(bindings []Binding) []Conflict {
	var out []Conflict
	for _, v := range Views {
		byKey := map[string][]Binding{}
		var order []string
		for _, b := range bindings {
			if !slices.Contains(b.Views, v) {
				continue
			}
			for _, k := range b.Keys {
				if byKey[k] == nil {
					order = append(order, k)
				}
				byKey[k] = append(byKey[k], b)
			}
		}
		for _, k := range order {
			if len(byKey[k]) < 2 {
				continue
			}
			i := slices.IndexFunc(out, func(c Conflict) bool { return c.Key == k && sameNames(c.Bindings, byKey[k]) })
			if i < 0 {
				out = append(out, Conflict{Key: k, Bindings: byKey[k]})
				i = len(out) - 1
			}
			out[i].Views = append(out[i].Views, v)
		}
	}
	return out
}

func sameNames(a, b []Binding) bool {
	return slices.EqualFunc(a, b, func(x, y Binding) bool { return x.Name == y.Name })
}

// ViewWords names a view as a sentence does: in the list, in settings.
func ViewWords(v View) string {
	switch v {
	case ViewList:
		return "in the list"
	case ViewDetail:
		return "in the detail view"
	case ViewSettings:
		return "in settings"
	}
	return "in " + string(v)
}

// ViewsWords names views as a sentence does: in the list and in the
// detail view.
func ViewsWords(views []View) string {
	words := make([]string, len(views))
	for i, v := range views {
		words[i] = ViewWords(v)
	}
	return joinWords(words)
}

// joinWords is a list in words: a, b and c.
func joinWords(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// Use is another binding that has a key, in the views where that
// matters.
type Use struct {
	Binding Binding
	Views   []View
}

func (u Use) String() string { return u.Binding.Label + " " + ViewsWords(u.Views) }

// Taken is every other binding that has key in a view where binding name
// acts.
func Taken(bindings []Binding, name, key string) []Use {
	b, ok := find(bindings, name)
	if !ok {
		return nil
	}
	var out []Use
	for _, o := range bindings {
		if o.Name == name || !slices.Contains(o.Keys, key) {
			continue
		}
		var views []View
		for _, v := range Views {
			if slices.Contains(b.Views, v) && slices.Contains(o.Views, v) {
				views = append(views, v)
			}
		}
		if len(views) > 0 {
			out = append(out, Use{Binding: o, Views: views})
		}
	}
	return out
}

// ErrRebind: a binding can't take the key it was given; the error says
// why.
var ErrRebind = errors.New("can't rebind")

// Rebinding is what giving binding name key, as its first, changes: its
// keys, and, when one other binding has key where name acts, that one's,
// which takes name's old first key in its place (or loses key, when name
// had none): the swap other is set for. It fails (ErrRebind) for a
// reserved key, a key name has first already, a key two other bindings
// have, and a swap that would bind another key twice.
func Rebinding(bindings []Binding, name, key string) (set map[string][]string, other *Use, err error) {
	b, ok := find(bindings, name)
	switch {
	case !ok:
		return nil, nil, fmt.Errorf("%w: there is no %q", ErrRebind, name)
	case Reserved(key):
		return nil, nil, fmt.Errorf("%w: %s", ErrRebind, reservedWhy(key))
	case len(b.Keys) > 0 && b.Keys[0] == key:
		return nil, nil, fmt.Errorf("%w: %s is already %s's key", ErrRebind, key, b.Label)
	}
	set = map[string][]string{name: withFirst(b.Keys, key)}
	switch uses := Taken(bindings, name, key); len(uses) {
	case 0:
	case 1:
		other = &uses[0]
		set[other.Binding.Name] = swapped(other.Binding.Keys, key, b.Keys)
	default:
		words := make([]string, len(uses))
		for i, u := range uses {
			words[i] = u.String()
		}
		return nil, nil, fmt.Errorf("%w: %s is already %s: pick another key", ErrRebind, key, joinWords(words))
	}
	if conflicts := Conflicts(With(bindings, set, nil)); len(conflicts) > 0 {
		return nil, nil, fmt.Errorf("%w: that would leave %s", ErrRebind, conflicts[0].Bound())
	}
	return set, other, nil
}

// ErrReset: a binding can't go back to its defaults; the error says why.
var ErrReset = errors.New("can't reset")

// Resetting is what putting binding name back to its defaults resets:
// name alone, when that binds no key twice; or name and the one other
// binding that has one of name's defaults now (key, the first it has),
// when that one going back to its own defaults as well binds no key
// twice: a swap, undone. It fails (ErrReset) otherwise, saying what
// name's defaults would leave.
func Resetting(bindings []Binding, name string) (reset []string, key string, err error) {
	if _, ok := find(bindings, name); !ok {
		return nil, "", fmt.Errorf("%w: there is no %q", ErrReset, name)
	}
	conflicts := Conflicts(With(bindings, nil, []string{name}))
	if len(conflicts) == 0 {
		return []string{name}, "", nil
	}
	if other, ok := soleOther(conflicts, name); ok && len(Conflicts(With(bindings, nil, []string{name, other}))) == 0 {
		return []string{name, other}, conflicts[0].Key, nil
	}
	return nil, "", fmt.Errorf("%w: default would leave %s: rebind the other first", ErrReset, conflicts[0].Bound())
}

// soleOther is the one binding other than name that every conflict pairs
// name with, and whether there is one: none when a conflict leaves name
// out, or binds a key to three.
func soleOther(conflicts []Conflict, name string) (string, bool) {
	other := ""
	for _, c := range conflicts {
		i := slices.IndexFunc(c.Bindings, func(b Binding) bool { return b.Name == name })
		if i < 0 || len(c.Bindings) != 2 {
			return "", false
		}
		o := c.Bindings[1-i].Name
		if other != "" && o != other {
			return "", false
		}
		other = o
	}
	return other, other != ""
}

// withFirst is keys with key first, in place of the first, and nowhere
// else.
func withFirst(keys []string, key string) []string {
	rest := []string{}
	if len(keys) > 1 {
		rest = keys[1:]
	}
	rest = slices.DeleteFunc(slices.Clone(rest), func(k string) bool { return k == key })
	return append([]string{key}, rest...)
}

// swapped is keys with key in it replaced by the first of from, the keys
// of the binding taking key; removed, when from has none, or keys has it
// already.
func swapped(keys []string, key string, from []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		switch {
		case k != key:
			out = append(out, k)
		case len(from) > 0 && !slices.Contains(keys, from[0]):
			out = append(out, from[0])
		}
	}
	return out
}

// With is bindings with set's keys in place of theirs, and reset's back
// to their defaults.
func With(bindings []Binding, set map[string][]string, reset []string) []Binding {
	out := make([]Binding, len(bindings))
	for i, b := range bindings {
		b.Keys = slices.Clone(b.Keys)
		if keys, ok := set[b.Name]; ok {
			b.Keys = slices.Clone(keys)
		}
		if slices.Contains(reset, b.Name) {
			b.Keys = slices.Clone(b.Defaults)
		}
		b.Yours = !slices.Equal(b.Keys, b.Defaults)
		out[i] = b
	}
	return out
}

// In is the bindings that act in view, in order.
func In(bindings []Binding, view View) []Binding {
	var out []Binding
	for _, b := range bindings {
		if slices.Contains(b.Views, view) {
			out = append(out, b)
		}
	}
	return out
}

// Free is every key git-repos takes that no binding has in view, in the
// order of Universe.
func Free(bindings []Binding, view View) []string {
	used := map[string]bool{}
	for _, b := range In(bindings, view) {
		for _, k := range b.Keys {
			used[k] = true
		}
	}
	return slices.DeleteFunc(Universe(), func(k string) bool { return used[k] })
}

func find(bindings []Binding, name string) (Binding, bool) {
	i := slices.IndexFunc(bindings, func(b Binding) bool { return b.Name == name })
	if i < 0 {
		return Binding{}, false
	}
	return bindings[i], true
}

// FromConfig is the bindings config.json's keys give, over the defaults,
// each binding's name sorted so its problems come in a stable order; the
// defaults, with the problems, when it has any.
func FromConfig(yours map[string][]string) ([]Binding, []string) {
	entries := make([]Entry, 0, len(yours))
	for _, name := range slices.Sorted(maps.Keys(yours)) {
		entries = append(entries, Entry{Name: name, Keys: yours[name]})
	}
	bindings, problems := Resolve(Defaults(), entries)
	if len(problems) > 0 {
		return Defaults(), problems
	}
	return bindings, nil
}
