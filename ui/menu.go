package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/kastheco/kasmos/keys"
	"github.com/kastheco/kasmos/session"
)

// Style definitions for the bottom keybind bar.
var keyStyle = lipgloss.NewStyle().Foreground(ColorSubtle)
var descStyle = lipgloss.NewStyle().Foreground(ColorMuted)
var sepStyle = lipgloss.NewStyle().Foreground(ColorOverlay)
var actionGroupStyle = lipgloss.NewStyle().Foreground(ColorRose)
var menuStyle = lipgloss.NewStyle().Foreground(ColorFoam)

// Separator tokens inserted between keybind items.
var separator = " • "
var verticalSeparator = " │ "

// MenuState identifies which mode the bottom bar is operating in.
type MenuState int

const (
	StateDefault MenuState = iota
	StateEmpty
	StateNewInstance
	StatePrompt
)

// Focus slot identifiers mirrored from the app package to avoid import cycles.
const (
	MenuSlotSidebar = 0
	MenuSlotInfo    = 1
	MenuSlotAgent   = 2
	MenuSlotDiff    = 3
	MenuSlotList    = 4
)

// Menu renders the bottom keybind bar. It is context-sensitive: the displayed
// shortcuts adapt based on the current state and which panel is focused.
type Menu struct {
	options            []keys.KeyName
	height             int
	width              int
	state              MenuState
	instance           *session.Instance
	isInDiffTab        bool
	isFocusMode        bool
	focusSlot          int
	sidebarSpaceAction string
	keyDown            keys.KeyName
	systemGroupSize    int
	tmuxSessionCount   int
}

// Pre-built option slices for each menu state.
var defaultMenuOptions = []keys.KeyName{
	keys.KeyNewPlan, keys.KeySearch, keys.KeySpace, keys.KeyHelp, keys.KeyQuit,
}
var emptyMenuOptions = []keys.KeyName{
	keys.KeySearch, keys.KeyHelp, keys.KeyQuit,
}
var newInstanceMenuOptions = []keys.KeyName{keys.KeySubmitName}
var promptMenuOptions = []keys.KeyName{keys.KeySubmitName}

// System group sizes for default and empty states.
const (
	defaultSystemGroupSize = 4
	emptySystemGroupSize   = 3
)

// Focus mode animation frames (braille spinner characters).
var focusModeFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Styles specific to focus / interactive mode overlay.
var focusDotStyle = lipgloss.NewStyle().Foreground(ColorLove).Bold(true)
var focusLabelStyle = lipgloss.NewStyle().Foreground(ColorLove).Bold(true)
var focusHintKeyStyle = lipgloss.NewStyle().Foreground(ColorSubtle)
var focusHintDescStyle = lipgloss.NewStyle().Foreground(ColorMuted)

// NewMenu constructs a Menu in the StateEmpty state with sensible defaults.
func NewMenu() *Menu {
	m := &Menu{
		state:              StateEmpty,
		keyDown:            -1,
		sidebarSpaceAction: "toggle",
	}
	m.updateOptions()
	return m
}

// SetTmuxSessionCount sets the number of active tmux sessions shown right-aligned.
func (m *Menu) SetTmuxSessionCount(count int) {
	m.tmuxSessionCount = count
}

// Keydown records a key press so that the corresponding item is underlined.
func (m *Menu) Keydown(name keys.KeyName) {
	m.keyDown = name
}

// ClearKeydown removes the current key-press highlight.
func (m *Menu) ClearKeydown() {
	m.keyDown = -1
}

// SetState transitions to a new state and refreshes the displayed options.
func (m *Menu) SetState(state MenuState) {
	m.state = state
	m.updateOptions()
}

// SetInstance updates the selected instance. When not in a special state the
// overall menu state is automatically inferred from whether inst is nil.
func (m *Menu) SetInstance(inst *session.Instance) {
	m.instance = inst
	if m.state != StateNewInstance && m.state != StatePrompt {
		if inst != nil {
			m.state = StateDefault
		} else {
			m.state = StateEmpty
		}
	}
	m.updateOptions()
}

// SetInDiffTab notes whether the diff tab is currently displayed.
func (m *Menu) SetInDiffTab(active bool) {
	m.isInDiffTab = active
	m.updateOptions()
}

// SetFocusMode toggles the interactive/focus overlay mode.
func (m *Menu) SetFocusMode(focused bool) {
	m.isFocusMode = focused
	m.updateOptions()
}

// SetFocusSlot records which panel currently holds keyboard focus.
func (m *Menu) SetFocusSlot(slot int) {
	m.focusSlot = slot
	m.updateOptions()
}

// SetSidebarSpaceAction controls the label shown for the space key while the
// sidebar is focused. Accepted values: "expand", "collapse"; others → "toggle".
func (m *Menu) SetSidebarSpaceAction(action string) {
	switch action {
	case "expand", "collapse":
		m.sidebarSpaceAction = action
	default:
		m.sidebarSpaceAction = "toggle"
	}
}

// SetSize stores the terminal dimensions used for horizontal layout.
func (m *Menu) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// updateOptions rebuilds the option slice whenever any relevant field changes.
func (m *Menu) updateOptions() {
	if m.isFocusMode {
		m.options = []keys.KeyName{keys.KeyExitFocus}
		m.systemGroupSize = 0
		return
	}
	switch m.state {
	case StateEmpty:
		if m.focusSlot == MenuSlotSidebar {
			m.addSidebarOptions(false)
		} else {
			m.options = emptyMenuOptions
			m.systemGroupSize = emptySystemGroupSize
		}
	case StateDefault:
		if m.instance != nil {
			m.addInstanceOptions()
		} else {
			m.addSidebarOptions(true)
		}
	case StateNewInstance:
		m.options = newInstanceMenuOptions
		m.systemGroupSize = 0
	case StatePrompt:
		m.options = promptMenuOptions
		m.systemGroupSize = 0
	}
}

// addSidebarOptions populates the option list for sidebar-focused states.
// Pass includeNewPlan=true when a plan exists and the 'n' shortcut is relevant.
func (m *Menu) addSidebarOptions(includeNewPlan bool) {
	capacity := 8
	if !includeNewPlan {
		capacity--
	}
	opts := make([]keys.KeyName, 0, capacity)
	if includeNewPlan {
		opts = append(opts, keys.KeyNewPlan)
	}
	actionGroup := []keys.KeyName{
		keys.KeyEnter, keys.KeySpaceExpand, keys.KeyViewPlan, keys.KeyAuditToggle,
	}
	systemGroup := []keys.KeyName{
		keys.KeySearch, keys.KeyHelp, keys.KeyQuit,
	}
	opts = append(opts, actionGroup...)
	opts = append(opts, systemGroup...)
	m.options = opts
	m.systemGroupSize = len(systemGroup)
}

// addInstanceOptions populates the option list when an instance is selected.
func (m *Menu) addInstanceOptions() {
	mgmt := []keys.KeyName{keys.KeyNewPlan, keys.KeyKill}
	action := []keys.KeyName{keys.KeyEnter, keys.KeySendPrompt, keys.KeySpace}
	if m.instance.PromptDetected && m.instance.Status != session.Paused {
		action = append(action, keys.KeySendYes)
	}
	if m.instance.Status == session.Paused {
		action = append(action, keys.KeyResume)
	}
	sys := []keys.KeyName{keys.KeySearch, keys.KeyTab, keys.KeyHelp, keys.KeyQuit}

	total := len(mgmt) + len(action) + len(sys)
	opts := make([]keys.KeyName, 0, total)
	opts = append(opts, mgmt...)
	opts = append(opts, action...)
	opts = append(opts, sys...)
	m.options = opts
	m.systemGroupSize = len(sys)
}

// tmuxCountRendered returns the rendered tmux count badge and its visual width.
func (m *Menu) tmuxCountRendered() (string, int) {
	if m.tmuxSessionCount <= 0 {
		return "", 0
	}
	text := statusBarTmuxCountStyle.Render(fmt.Sprintf("tmux:%d", m.tmuxSessionCount))
	return text, lipgloss.Width(text)
}

// placeLine composes a single-row string using cursor-tracked positioning.
// It places centeredContent centred on the available width and rightContent
// right-aligned, padding with spaces so the total reaches m.width.
func (m *Menu) placeLine(
	centeredStart, centeredVisualWidth int, centeredContent string,
	rightStart, rightVisualWidth int, rightContent string,
) string {
	var sb strings.Builder
	pos := 0

	emit := func(target, visualWidth int, text string) {
		if text == "" || target < pos {
			return
		}
		if target > pos {
			sb.WriteString(strings.Repeat(" ", target-pos))
		}
		sb.WriteString(text)
		pos = target + visualWidth
	}

	emit(centeredStart, centeredVisualWidth, centeredContent)
	emit(rightStart, rightVisualWidth, rightContent)

	if pos < m.width {
		sb.WriteString(strings.Repeat(" ", m.width-pos))
	}
	return sb.String()
}

// renderFocusMode produces the interactive-mode status row.
func (m *Menu) renderFocusMode() string {
	frame := focusModeFrames[int(time.Now().UnixMilli()/100)%len(focusModeFrames)]
	badge := focusLabelStyle.Render("interactive") + " " + focusDotStyle.Render(frame)
	hint := focusHintKeyStyle.Render("ctrl+space") + " " + focusHintDescStyle.Render("exit")
	content := badge + "  " + hint

	cw := lipgloss.Width(content)
	cs := (m.width - cw) / 2
	if cs < 0 {
		cs = 0
	}

	tr, tw := m.tmuxCountRendered()
	rs := m.width - tw - 1
	if rs < cs+cw {
		rs = cs + cw
	}

	return m.placeLine(cs, cw, content, rs, tw, tr)
}

// buildOptionText assembles the raw option text (before wrapping in menuStyle).
func (m *Menu) buildOptionText() string {
	n := len(m.options)
	if n == 0 {
		return ""
	}

	// Determine effective system group size.
	sysSize := m.systemGroupSize
	if sysSize == 0 {
		sysSize = defaultSystemGroupSize
	}
	actionEnd := n - sysSize

	// Group definitions: {start (inclusive), end (exclusive)}.
	type span struct{ start, end int }
	var groups []span
	if m.state == StateEmpty {
		groups = []span{{0, n}}
	} else {
		groups = []span{
			{0, 2},
			{2, actionEnd},
			{actionEnd, n},
		}
	}

	// isGroupTail reports whether position i is the last element of its group.
	isGroupTail := func(i int) bool {
		for _, g := range groups {
			if i == g.end-1 && i != n-1 {
				return true
			}
		}
		return false
	}

	// isActionItem reports whether item i belongs to the action group (middle).
	isActionItem := func(i int) bool {
		switch m.state {
		case StateEmpty:
			return i < actionEnd
		default:
			if len(groups) < 2 {
				return false
			}
			return i >= groups[1].start && i < groups[1].end
		}
	}

	var sb strings.Builder
	for i, k := range m.options {
		binding := keys.GlobalkeyBindings[k]
		h := binding.Help()
		label, desc := h.Key, h.Desc
		if k == keys.KeySpaceExpand {
			desc = m.sidebarSpaceAction
		}

		// Build per-item styles, optionally underlined for the pressed key.
		akStyle := actionGroupStyle
		kStyle := keyStyle
		dStyle := descStyle
		if m.keyDown == k {
			akStyle = akStyle.Underline(true)
			kStyle = kStyle.Underline(true)
			dStyle = dStyle.Underline(true)
		}

		if isActionItem(i) {
			sb.WriteString(akStyle.Render(label + " " + desc))
		} else {
			sb.WriteString(kStyle.Render(label))
			sb.WriteString(descStyle.Render(" "))
			sb.WriteString(dStyle.Render(desc))
		}

		if i < n-1 {
			if isGroupTail(i) {
				sb.WriteString(sepStyle.Render(verticalSeparator))
			} else {
				sb.WriteString(sepStyle.Render(separator))
			}
		}
	}
	return sb.String()
}

// String renders the full bottom bar row as a string.
func (m *Menu) String() string {
	if m.isFocusMode {
		return m.renderFocusMode()
	}

	optText := m.buildOptionText()
	rendered := menuStyle.Render(optText)
	rw := lipgloss.Width(rendered)

	tr, tw := m.tmuxCountRendered()

	ms := (m.width - rw) / 2
	if ms < 0 {
		ms = 0
	}
	rs := m.width - tw - 1
	if rs < ms+rw {
		rs = ms + rw
	}

	return m.placeLine(ms, rw, rendered, rs, tw, tr)
}
