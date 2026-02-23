package ui

import (
	"github.com/kastheco/klique/keys"
	"strings"

	"github.com/kastheco/klique/session"

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

	// keyDown is the key which is pressed. The default is -1.
	keyDown keys.KeyName

	// systemGroupSize is the number of items in the trailing system group
	// (used for separator placement). Defaults to 4 if unset.
	systemGroupSize int
}

var defaultMenuOptions = []keys.KeyName{keys.KeyNew, keys.KeyNewPlan, keys.KeySearch, keys.KeySpace, keys.KeyRepoSwitch, keys.KeyHelp, keys.KeyQuit}
var defaultSystemGroupSize = 5 // search, space actions, R repo switch, ? help, q quit
var newInstanceMenuOptions = []keys.KeyName{keys.KeySubmitName}
var promptMenuOptions = []keys.KeyName{keys.KeySubmitName}

func NewMenu() *Menu {
	return &Menu{
		options:     defaultMenuOptions,
		state:       StateEmpty,
		isInDiffTab: false,
		keyDown:     -1,
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

// updateOptions updates the menu options based on current state and instance
func (m *Menu) updateOptions() {
	switch m.state {
	case StateEmpty:
		m.options = defaultMenuOptions
		m.systemGroupSize = defaultSystemGroupSize
	case StateDefault:
		if m.instance != nil {
			// When there is an instance, show that instance's options
			m.addInstanceOptions()
		} else {
			// When there is no instance, show the empty state
			m.options = defaultMenuOptions
			m.systemGroupSize = defaultSystemGroupSize
		}
	case StateNewInstance:
		m.options = newInstanceMenuOptions
		m.systemGroupSize = 0
	case StatePrompt:
		m.options = promptMenuOptions
		m.systemGroupSize = 0
	}
}

func (m *Menu) addInstanceOptions() {
	// Instance management group
	options := []keys.KeyName{keys.KeyNew, keys.KeyKill}

	// Action group
	actionGroup := []keys.KeyName{keys.KeyEnter, keys.KeySendPrompt, keys.KeySpace, keys.KeyCreatePR}
	if m.instance.Status == session.Paused {
		actionGroup = append(actionGroup, keys.KeyResume)
	} else {
		actionGroup = append(actionGroup, keys.KeyCheckout)
	}

	// Navigation group (when in diff tab): up/down navigate when diff is focused via Tab ring

	// System group
	systemGroup := []keys.KeyName{keys.KeySearch, keys.KeyRepoSwitch, keys.KeyTab, keys.KeyHelp, keys.KeyQuit}

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

func (m *Menu) String() string {
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
			s.WriteString(localActionStyle.Render(binding.Help().Key + " " + binding.Help().Desc))
		} else {
			s.WriteString(localKeyStyle.Render(binding.Help().Key))
			s.WriteString(descStyle.Render(" "))
			s.WriteString(localDescStyle.Render(binding.Help().Desc))
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
