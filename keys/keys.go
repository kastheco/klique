package keys

import (
	"charm.land/bubbles/v2/key"
)

type KeyName int

const (
	KeyUp KeyName = iota
	KeyDown
	KeyEnter
	KeyKill  // k — soft kill: terminates tmux session, keeps instance in list
	KeyAbort // K — full abort: kills tmux, removes worktree, removes from list
	KeyQuit
	KeyReview
	KeyPush
	KeySubmit

	KeyTab        // Tab is a special keybinding for cycling the focus ring.
	KeySubmitName // SubmitName is a special keybinding for submitting the name of a new instance.

	KeyCheckout
	KeyResume
	KeyPrompt             // New key for entering a prompt
	KeyHelp               // Key for showing help screen
	KeyNewSkipPermissions // Key for creating instance with --dangerously-skip-permissions

	KeyNewPlan    // Key for creating a new plan
	KeySearch     // Key for activating search
	KeyArrowLeft  // Key for in-pane horizontal navigation left (tree collapse, etc.)
	KeyArrowRight // Key for in-pane horizontal navigation right (tree expand, etc.)

	KeyCreatePR // Key for creating a pull request

	KeySendPrompt // Key for sending a prompt to a running instance
	KeySendYes    // Key for sending yes to a waiting instance

	KeySpace // Key for opening context menu on selected item

	// Instance filter keybindings
	KeyFilterAll    // Key for showing all instances
	KeyFilterActive // Key for showing only active instances
	KeyCycleSort    // Key for cycling sort mode

	KeySpawnAgent  // s - spawn ad-hoc agent session
	KeyViewPlan    // Key for viewing the selected plan's markdown
	KeySpaceExpand // Space key with expand/collapse label (sidebar context)

	KeyTmuxBrowser // t - browse orphaned tmux sessions

	KeyAuditToggle // L - toggle audit log pane visibility
	KeyAuditCursor // A - enter audit log cursor mode (navigate log lines)
)

// GlobalKeyStringsMap is a global, immutable map string to keybinding.
var GlobalKeyStringsMap = map[string]KeyName{
	"up":    KeyUp,
	"down":  KeyDown,
	"N":     KeyPrompt,
	"enter": KeyEnter,
	"o":     KeyEnter,
	"n":     KeyNewPlan,
	"k":     KeyKill,
	"K":     KeyAbort,
	"q":     KeyQuit,
	"tab":   KeyTab,
	"c":     KeyCheckout,
	"r":     KeyResume,
	"?":     KeyHelp,
	"S":     KeyNewSkipPermissions,
	"/":     KeySearch,
	"left":  KeyArrowLeft,
	"right": KeyArrowRight,
	"P":     KeyCreatePR,
	"i":     KeySendPrompt,
	"y":     KeySendYes,
	" ":     KeySpace,
	"1":     KeyFilterAll,
	"2":     KeyFilterActive,
	"3":     KeyCycleSort,
	"t":     KeyTmuxBrowser,
	"s":     KeySpawnAgent,
	"L":     KeyAuditToggle,
	"A":     KeyAuditCursor,
	"p":     KeyViewPlan,
	// ctrl+space is handled directly as a raw key event in app/app_input.go (pane focus toggle).
}

// GlobalkeyBindings is a global, immutable map of KeyName tot keybinding.
var GlobalkeyBindings = map[KeyName]key.Binding{
	KeyUp: key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("↑", "up"),
	),
	KeyDown: key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "down"),
	),
	KeyEnter: key.NewBinding(
		key.WithKeys("enter", "o"),
		key.WithHelp("↵/o", "select"),
	),
	KeyKill: key.NewBinding(
		key.WithKeys("k"),
		key.WithHelp("k", "kill"),
	),
	KeyAbort: key.NewBinding(
		key.WithKeys("K"),
		key.WithHelp("K", "abort"),
	),
	KeyHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	KeyQuit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	KeyNewPlan: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new plan"),
	),
	KeyPrompt: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "new with prompt"),
	),
	KeyCheckout: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "checkout"),
	),
	KeyTab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "cycle panes"),
	),
	KeyResume: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "resume"),
	),
	KeyNewSkipPermissions: key.NewBinding(
		key.WithKeys("S"),
		key.WithHelp("S", "new (skip permissions)"),
	),
	KeySearch: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	KeyCreatePR: key.NewBinding(
		key.WithKeys("P"),
		key.WithHelp("P", "create PR"),
	),
	KeyArrowLeft: key.NewBinding(
		key.WithKeys("left"),
		key.WithHelp("←", "left"),
	),
	KeyArrowRight: key.NewBinding(
		key.WithKeys("right"),
		key.WithHelp("→", "right"),
	),
	KeySendPrompt: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "send prompt"),
	),
	KeySendYes: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "yes"),
	),
	KeySpace: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "menu"),
	),
	KeyFilterAll: key.NewBinding(
		key.WithKeys("1"),
		key.WithHelp("1", "all"),
	),
	KeyFilterActive: key.NewBinding(
		key.WithKeys("2"),
		key.WithHelp("2", "active"),
	),
	KeyCycleSort: key.NewBinding(
		key.WithKeys("3"),
		key.WithHelp("3", "sort"),
	),
	KeySpawnAgent: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "spawn agent"),
	),
	KeyViewPlan: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "preview"),
	),
	KeySpaceExpand: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle"),
	),

	KeyTmuxBrowser: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "tmux sessions"),
	),

	KeyAuditToggle: key.NewBinding(
		key.WithKeys("L"),
		key.WithHelp("L", "log"),
	),

	KeyAuditCursor: key.NewBinding(
		key.WithKeys("A"),
		key.WithHelp("A", "log actions"),
	),

	// -- Special keybindings --

	KeySubmitName: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "submit name"),
	),
}
