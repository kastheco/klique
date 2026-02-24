package keys

import (
	"github.com/charmbracelet/bubbles/key"
)

type KeyName int

const (
	KeyUp KeyName = iota
	KeyDown
	KeyEnter
	KeyNew
	KeyKill
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

	KeySpace // Key for opening context menu on selected item

	// Instance filter keybindings
	KeyFilterAll    // Key for showing all instances
	KeyFilterActive // Key for showing only active instances
	KeyCycleSort    // Key for cycling sort mode
	KeyRepoSwitch   // Key for switching repos

	KeyGitTab // Key for jumping directly to git tab

	// Tab switching keybindings (Shift+1/2/3 = !/@ /#)
	KeyTabAgent
	KeyTabDiff
	KeyTabGit

	KeyFocusSidebar  // Key for focusing the left sidebar / plan list
	KeyFocusList     // Key for focusing the right sidebar / instance list
	KeyViewPlan      // Key for viewing the selected plan's markdown
	KeyToggleSidebar // Key for toggling sidebar visibility
	KeyExitFocus     // Key for exiting focus/interactive mode (ctrl+space)
	KeySpaceExpand   // Space key with expand/collapse label (sidebar context)
)

// GlobalKeyStringsMap is a global, immutable map string to keybinding.
var GlobalKeyStringsMap = map[string]KeyName{
	"up":     KeyUp,
	"k":      KeyUp,
	"down":   KeyDown,
	"j":      KeyDown,
	"N":      KeyPrompt,
	"enter":  KeyEnter,
	"o":      KeyEnter,
	"n":      KeyNewPlan,
	"K":      KeyKill,
	"q":      KeyQuit,
	"tab":    KeyTab,
	"c":      KeyCheckout,
	"r":      KeyResume,
	"?":      KeyHelp,
	"S":      KeyNewSkipPermissions,
	"/":      KeySearch,
	"left":   KeyArrowLeft,
	"h":      KeyArrowLeft,
	"right":  KeyArrowRight,
	"l":      KeyArrowRight,
	"P":      KeyCreatePR,
	"i":      KeySendPrompt,
	" ":      KeySpace,
	"1":      KeyFilterAll,
	"2":      KeyFilterActive,
	"3":      KeyCycleSort,
	"R":      KeyRepoSwitch,
	"s":      KeyFocusSidebar,
	"t":      KeyFocusList,
	"v":      KeyViewPlan,
	"ctrl+s": KeyToggleSidebar,
	"g":      KeyGitTab,
	"!":      KeyTabAgent,
	"@":      KeyTabDiff,
	"#":      KeyTabGit,
}

// GlobalkeyBindings is a global, immutable map of KeyName tot keybinding.
var GlobalkeyBindings = map[KeyName]key.Binding{
	KeyUp: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	KeyDown: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	KeyEnter: key.NewBinding(
		key.WithKeys("enter", "o"),
		key.WithHelp("↵/o", "open"),
	),
	KeyNew: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	KeyKill: key.NewBinding(
		key.WithKeys("K"),
		key.WithHelp("K", "kill"),
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
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left"),
	),
	KeyArrowRight: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	KeySendPrompt: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "interactive"),
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
	KeyRepoSwitch: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "switch repo"),
	),
	KeyFocusSidebar: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "left sidebar"),
	),
	KeyFocusList: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "right sidebar"),
	),
	KeyViewPlan: key.NewBinding(
		key.WithKeys("v"),
		key.WithHelp("v", "view plan"),
	),
	KeyToggleSidebar: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "toggle sidebar"),
	),
	KeyGitTab: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "git tab"),
	),
	KeyTabAgent: key.NewBinding(
		key.WithKeys("!"),
		key.WithHelp("!/@ /#", "switch tab"),
	),
	KeyTabDiff: key.NewBinding(
		key.WithKeys("@"),
		key.WithHelp("@", "diff tab"),
	),
	KeyTabGit: key.NewBinding(
		key.WithKeys("#"),
		key.WithHelp("#", "git tab"),
	),
	KeyExitFocus: key.NewBinding(
		key.WithKeys("ctrl+@"),
		key.WithHelp("ctrl+space", "exit focus"),
	),

	KeySpaceExpand: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "expand/collapse"),
	),

	// -- Special keybindings --

	KeySubmitName: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "submit name"),
	),
}
