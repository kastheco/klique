package ui

import (
	"strings"
	"time"

	"github.com/kastheco/kasmos/keys"
	"github.com/kastheco/kasmos/session"

	"github.com/charmbracelet/lipgloss"
)

var keyStyle = lipgloss.NewStyle().Foreground(ColorSubtle)

var descStyle = lipgloss.NewStyle().Foreground(ColorMuted)

var sepStyle = lipgloss.NewStyle().Foreground(ColorOverlay)

var actionGroupStyle = lipgloss.NewStyle().Foreground(ColorRose)

var separator = " • "
var verticalSeparator = " │ "

var menuStyle = lipgloss.NewStyle().
	Foreground(ColorFoam)

// MenuState represents different states the menu can be in
type MenuState int

const (
	StateDefault MenuState = iota
	StateEmpty
	StateNewInstance
	StatePrompt
)

type Menu struct {
	options       []keys.KeyName
	height, width int
	state         MenuState
	instance      *session.Instance
	isInDiffTab   bool
	isFocusMode   bool
	focusSlot     int // which pane is focused (-1 = unknown)

	// sidebarSpaceAction controls the help label for KeySpaceExpand when the
	// sidebar is focused ("expand" or "collapse").
	sidebarSpaceAction string

	// keyDown is the key which is pressed. The default is -1.
	keyDown keys.KeyName

	// systemGroupSize is the number of items in the trailing system group
	// (used for separator placement). Defaults to 4 if unset.
	systemGroupSize int
}

var defaultMenuOptions = []keys.KeyName{keys.KeyNewPlan, keys.KeySearch, keys.KeySpace, keys.KeyHelp, keys.KeyQuit}
var defaultSystemGroupSize = 4 // search, space shortcuts, ? help, q quit
var emptyMenuOptions = []keys.KeyName{keys.KeySearch, keys.KeyHelp, keys.KeyQuit}
var emptySystemGroupSize = 3
var newInstanceMenuOptions = []keys.KeyName{keys.KeySubmitName}
var promptMenuOptions = []keys.KeyName{keys.KeySubmitName}

func NewMenu() *Menu {
	return &Menu{
		options:            defaultMenuOptions,
		state:              StateEmpty,
		isInDiffTab:        false,
		keyDown:            -1,
		sidebarSpaceAction: "toggle",
	}
}

func (m *Menu) Keydown(name keys.KeyName) {
	m.keyDown = name
}

func (m *Menu) ClearKeydown() {
	m.keyDown = -1
}

// SetState updates the menu state and options accordingly
func (m *Menu) SetState(state MenuState) {
	m.state = state
	m.updateOptions()
}

// SetInstance updates the current instance and refreshes menu options
func (m *Menu) SetInstance(instance *session.Instance) {
	m.instance = instance
	// Only change the state if we're not in a special state (NewInstance or Prompt)
	if m.state != StateNewInstance && m.state != StatePrompt {
		if m.instance != nil {
			m.state = StateDefault
		} else {
			m.state = StateEmpty
		}
	}
	m.updateOptions()
}

// SetInDiffTab updates whether we're currently in the diff tab
func (m *Menu) SetInDiffTab(inDiffTab bool) {
	m.isInDiffTab = inDiffTab
	m.updateOptions()
}

// SetFocusMode updates whether the agent pane is in focus/interactive mode.
func (m *Menu) SetFocusMode(focused bool) {
	m.isFocusMode = focused
	m.updateOptions()
}

// FocusSlot constants mirrored from app package to avoid import cycle.
const (
	MenuSlotSidebar = 0
	MenuSlotInfo    = 1
	MenuSlotAgent   = 2
	MenuSlotDiff    = 3
	MenuSlotList    = 4
)

// SetFocusSlot updates which pane is focused so the menu can show context-sensitive keybinds.
func (m *Menu) SetFocusSlot(slot int) {
	m.focusSlot = slot
	m.updateOptions()
}

// SetSidebarSpaceAction sets the sidebar-specific space-key label.
func (m *Menu) SetSidebarSpaceAction(action string) {
	switch action {
	case "expand", "collapse":
		m.sidebarSpaceAction = action
	default:
		m.sidebarSpaceAction = "toggle"
	}
}

// updateOptions updates the menu options based on current state and instance
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
		// Sidebar always has focus; show instance actions when an instance is
		// selected so the user can see relevant keybinds without switching panes.
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

func (m *Menu) addSidebarOptions(includeNewPlan bool) {
	// Sidebar-focused: show plan navigation keybinds
	options := make([]keys.KeyName, 0, 7)
	if includeNewPlan {
		options = append(options, keys.KeyNewPlan)
	}
	actionGroup := []keys.KeyName{keys.KeyEnter, keys.KeySpaceExpand, keys.KeyViewPlan}
	systemGroup := []keys.KeyName{keys.KeySearch, keys.KeyHelp, keys.KeyQuit}

	options = append(options, actionGroup...)
	options = append(options, systemGroup...)
	m.options = options
	m.systemGroupSize = len(systemGroup)
}

func (m *Menu) addInstanceOptions() {
	// Instance management group
	options := []keys.KeyName{keys.KeyNewPlan, keys.KeyKill}

	// Action group
	actionGroup := []keys.KeyName{keys.KeyEnter, keys.KeySendPrompt, keys.KeySpace}
	if m.instance.PromptDetected && m.instance.Status != session.Paused {
		actionGroup = append(actionGroup, keys.KeySendYes)
	}
	if m.instance.Status == session.Paused {
		actionGroup = append(actionGroup, keys.KeyResume)
	}

	// Navigation group (when in diff tab): up/down navigate when diff is focused via Tab ring

	// System group
	systemGroup := []keys.KeyName{keys.KeySearch, keys.KeyTab, keys.KeyHelp, keys.KeyQuit}

	// Combine all groups
	options = append(options, actionGroup...)
	options = append(options, systemGroup...)

	m.options = options
	m.systemGroupSize = len(systemGroup)
}

// SetSize sets the width of the window. The menu will be centered horizontally within this width.
func (m *Menu) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// focusModeFrames defines the animation frames for the interactive mode indicator.
var focusModeFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var focusDotStyle = lipgloss.NewStyle().Foreground(ColorLove).Bold(true)
var focusLabelStyle = lipgloss.NewStyle().Foreground(ColorLove).Bold(true)
var focusHintKeyStyle = lipgloss.NewStyle().Foreground(ColorSubtle)
var focusHintDescStyle = lipgloss.NewStyle().Foreground(ColorMuted)

func (m *Menu) renderFocusMode() string {
	// Animated spinner frame based on wall-clock time (~100ms per frame).
	frame := focusModeFrames[int(time.Now().UnixMilli()/100)%len(focusModeFrames)]
	badge := focusLabelStyle.Render("interactive") + " " + focusDotStyle.Render(frame)
	hint := focusHintKeyStyle.Render("ctrl+space") + " " + focusHintDescStyle.Render("exit")
	content := badge + "  " + hint

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m *Menu) String() string {
	if m.isFocusMode {
		return m.renderFocusMode()
	}

	var s strings.Builder

	// Define group boundaries dynamically based on option count
	// Instance management: n, D (2 items)
	// Action group: enter, space, submit, P, pause/resume [+ shift-up if diff tab] (variable)
	// System group: X, /, tab, ?, q (at the end)
	sysSize := m.systemGroupSize
	if sysSize == 0 {
		sysSize = 4
	}
	actionEnd := len(m.options) - sysSize

	var groups []struct {
		start int
		end   int
	}
	if m.state == StateEmpty {
		// No group separators in empty state — all items are one flat group.
		groups = []struct {
			start int
			end   int
		}{
			{0, len(m.options)},
		}
	} else {
		groups = []struct {
			start int
			end   int
		}{
			{0, 2},                      // Instance management group
			{2, actionEnd},              // Action group
			{actionEnd, len(m.options)}, // System group
		}
	}

	for i, k := range m.options {
		binding := keys.GlobalkeyBindings[k]
		help := binding.Help()
		helpKey := help.Key
		helpDesc := help.Desc
		if k == keys.KeySpaceExpand {
			helpDesc = m.sidebarSpaceAction
		}

		var (
			localActionStyle = actionGroupStyle
			localKeyStyle    = keyStyle
			localDescStyle   = descStyle
		)
		if m.keyDown == k {
			localActionStyle = localActionStyle.Underline(true)
			localKeyStyle = localKeyStyle.Underline(true)
			localDescStyle = localDescStyle.Underline(true)
		}

		var inActionGroup bool
		switch m.state {
		case StateEmpty:
			// For empty state, action group is everything before the system group
			inActionGroup = i < actionEnd
		default:
			// For other states, the action group is the second group
			inActionGroup = i >= groups[1].start && i < groups[1].end
		}

		if inActionGroup {
			s.WriteString(localActionStyle.Render(helpKey + " " + helpDesc))
		} else {
			s.WriteString(localKeyStyle.Render(helpKey))
			s.WriteString(descStyle.Render(" "))
			s.WriteString(localDescStyle.Render(helpDesc))
		}

		// Add appropriate separator
		if i != len(m.options)-1 {
			isGroupEnd := false
			for _, group := range groups {
				if i == group.end-1 {
					s.WriteString(sepStyle.Render(verticalSeparator))
					isGroupEnd = true
					break
				}
			}
			if !isGroupEnd {
				s.WriteString(sepStyle.Render(separator))
			}
		}
	}

	centeredMenuText := menuStyle.Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, centeredMenuText)
}
