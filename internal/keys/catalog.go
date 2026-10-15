package keys

import "slices"

// View is where a binding acts: one of the TUI's views. A key means one
// thing at most in each.
type View string

const (
	ViewList     View = "list"
	ViewDetail   View = "detail"
	ViewPRs      View = "prs"
	ViewSettings View = "settings"
	// ViewHelp is the help (?), where you also rebind the keys.
	ViewHelp View = "help"
)

// Views is every view a key acts in, in the order git-repos lists them.
var Views = []View{ViewList, ViewDetail, ViewPRs, ViewSettings, ViewHelp}

// Binding is one thing a key does, with the keys it takes now and where.
type Binding struct {
	// Name is what config.json's keys names it by: move_down, open_pr…
	Name string `json:"name"`
	// Label names it in words, as a conflict or a hint says it.
	Label string `json:"label"`
	// Keys are the keys it takes now: yours from config.json, or its
	// defaults. Empty when it has none.
	Keys     []string `json:"keys"`
	Defaults []string `json:"defaults"`
	Views    []View   `json:"views"`
	// Yours: config.json gives it other keys than its defaults.
	Yours bool `json:"yours"`
}

// ReservedKey is a key no binding can take: ctrl+c always quits, and esc
// always goes back.
type ReservedKey struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// The names of the bindings, as config.json's keys names them.
const (
	MoveDown    = "move_down"
	MoveUp      = "move_up"
	PageDown    = "page_down"
	PageUp      = "page_up"
	First       = "first"
	Last        = "last"
	Details     = "details"
	Search      = "search"
	OpenPR      = "open_pr"
	OpenGitHub  = "open_github"
	Pull        = "pull"
	PullAll     = "pull_all"
	Shell       = "shell"
	OpenEditor  = "open_editor"
	PickEditor  = "pick_editor"
	Delete      = "delete"
	Refresh     = "refresh"
	Settings    = "settings"
	SortStatus  = "sort_status"
	SortName    = "sort_name"
	SortBranch  = "sort_branch"
	SortSync    = "sort_sync"
	SortChanges = "sort_changes"
	SortTime    = "sort_time"
	SortPR      = "sort_pr"
	// SwitchPage goes between the repos and the PR page; FilterPRs, the
	// PR page's own, shows all PRs, yours, or those asking your review.
	SwitchPage = "switch_page"
	FilterPRs  = "filter_prs"
	// Toggle and Edit are the settings' own: toggle or edit the setting
	// selected, and edit a number.
	Toggle = "toggle"
	Edit   = "edit"
	// Back is what esc does: close the detail view or the settings, or
	// clear a search. It has no key but esc by default.
	Back = "back"
	Help = "help"
	Quit = "quit"
	// Rebind and Reset are the help's own: they rebind the key selected
	// there, and put its defaults back.
	Rebind = "rebind"
	Reset  = "reset"
)

var (
	inList   = []View{ViewList}
	inDetail = []View{ViewDetail}
	inPRs    = []View{ViewPRs}
	onRepos  = []View{ViewList, ViewDetail}
	inLists  = []View{ViewList, ViewPRs}
	onAny    = []View{ViewList, ViewDetail, ViewPRs}
	inAll    = []View{ViewList, ViewDetail, ViewPRs, ViewSettings, ViewHelp}
	inHelp   = []View{ViewHelp}
)

// catalog is every binding with its default keys: git-repos's keys as they
// were before you could change them.
var catalog = []Binding{
	{Name: MoveDown, Label: "Move down", Defaults: []string{"j", "down"}, Views: inAll},
	{Name: MoveUp, Label: "Move up", Defaults: []string{"k", "up"}, Views: inAll},
	{Name: PageDown, Label: "Page down", Defaults: []string{"ctrl+f"}, Views: inLists},
	{Name: PageUp, Label: "Page up", Defaults: []string{"ctrl+b"}, Views: inLists},
	{Name: First, Label: "Top", Defaults: []string{"g", "home"}, Views: inLists},
	{Name: Last, Label: "Bottom", Defaults: []string{"G", "end"}, Views: inLists},
	{Name: Details, Label: "Detail view", Defaults: []string{"enter"}, Views: inLists},
	{Name: Search, Label: "Search", Defaults: []string{"/"}, Views: inLists},
	{Name: OpenPR, Label: "Open PR", Defaults: []string{"o"}, Views: onAny},
	{Name: OpenGitHub, Label: "Open in GitHub", Defaults: []string{"g"}, Views: inDetail},
	{Name: Pull, Label: "Pull repo", Defaults: []string{"p"}, Views: onRepos},
	{Name: PullAll, Label: "Pull all", Defaults: []string{"ctrl+p"}, Views: inList},
	{Name: Shell, Label: "Open shell", Defaults: []string{"s"}, Views: onRepos},
	{Name: OpenEditor, Label: "Open in editor", Defaults: []string{"e"}, Views: onAny},
	{Name: PickEditor, Label: "Open in…", Defaults: []string{"E"}, Views: onAny},
	{Name: Delete, Label: "Delete repo", Defaults: []string{"ctrl+d"}, Views: inList},
	{Name: Refresh, Label: "Refresh", Defaults: []string{"r"}, Views: onAny},
	{Name: Settings, Label: "Settings", Defaults: []string{"z"}, Views: onRepos},
	{Name: SortStatus, Label: "Sort by status", Defaults: []string{"0"}, Views: inList},
	{Name: SortName, Label: "Sort by name", Defaults: []string{"N"}, Views: inList},
	{Name: SortBranch, Label: "Sort by branch", Defaults: []string{"B"}, Views: inList},
	{Name: SortSync, Label: "Sort by sync", Defaults: []string{"S"}, Views: inList},
	{Name: SortChanges, Label: "Sort by changes", Defaults: []string{"C"}, Views: inList},
	{Name: SortTime, Label: "Sort by time", Defaults: []string{"T"}, Views: inList},
	{Name: SortPR, Label: "Sort by PR", Defaults: []string{"P"}, Views: inList},
	{Name: SwitchPage, Label: "PRs ⇄ repos", Defaults: []string{"tab"}, Views: inLists},
	{Name: FilterPRs, Label: "Filter PRs", Defaults: []string{"f"}, Views: inPRs},
	{Name: Toggle, Label: "Toggle / Edit", Defaults: []string{"enter", "space"}, Views: []View{ViewSettings}},
	{Name: Edit, Label: "Edit", Defaults: []string{"e"}, Views: []View{ViewSettings}},
	{Name: Back, Label: "Back", Views: inAll},
	{Name: Help, Label: "Help", Defaults: []string{"?"}, Views: inAll},
	{Name: Quit, Label: "Quit", Defaults: []string{"q"}, Views: inAll},
	{Name: Rebind, Label: "Rebind", Defaults: []string{"r"}, Views: inHelp},
	{Name: Reset, Label: "Reset to default", Defaults: []string{"backspace"}, Views: inHelp},
}

// reserved are the keys no binding takes, and what they do: each always
// does it, wherever you are.
var reserved = []ReservedKey{{Key: CtrlC, Label: "Quit"}, {Key: Esc, Label: "Back"}}

// alwaysHas is the binding each reserved key does, which may list it,
// though it always has it.
var alwaysHas = map[string]string{CtrlC: Quit, Esc: Back}

// ReservedKeys is the keys no binding takes, and what each does.
func ReservedKeys() []ReservedKey { return slices.Clone(reserved) }

// Defaults is every binding with its default keys.
func Defaults() []Binding {
	out := make([]Binding, len(catalog))
	for i, b := range catalog {
		out[i] = defaulted(b)
	}
	return out
}

// defaulted is b taking its defaults, with slices of its own.
func defaulted(b Binding) Binding {
	b.Defaults, b.Views = slices.Clone(b.Defaults), slices.Clone(b.Views)
	b.Keys, b.Yours = slices.Clone(b.Defaults), false
	return b
}
