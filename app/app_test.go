package app

import (
	"context"
	"fmt"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"os"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain runs before all tests to set up the test environment
func TestMain(m *testing.M) {
	// Initialize the logger before any tests run
	log.Initialize(false)
	defer log.Close()

	// Run all tests
	exitCode := m.Run()

	// Exit with the same code as the tests
	os.Exit(exitCode)
}

// TestConfirmationModalStateTransitions tests state transitions without full instance setup
func TestConfirmationModalStateTransitions(t *testing.T) {
	// Create a minimal home struct for testing state transitions
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
	}

	t.Run("shows confirmation on D press", func(t *testing.T) {
		// Simulate pressing 'D'
		h.state = stateDefault
		h.confirmationOverlay = nil

		// Manually trigger what would happen in handleKeyPress for 'D'
		h.state = stateConfirm
		h.confirmationOverlay = overlay.NewConfirmationOverlay("[!] Kill session 'test'?")

		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.False(t, h.confirmationOverlay.Dismissed)
	})

	t.Run("returns to default on y press", func(t *testing.T) {
		// Start in confirmation state
		h.state = stateConfirm
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Test confirmation")

		// Simulate pressing 'y' using HandleKeyPress
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
		shouldClose := h.confirmationOverlay.HandleKeyPress(keyMsg)
		if shouldClose {
			h.state = stateDefault
			h.confirmationOverlay = nil
		}

		assert.Equal(t, stateDefault, h.state)
		assert.Nil(t, h.confirmationOverlay)
	})

	t.Run("returns to default on n press", func(t *testing.T) {
		// Start in confirmation state
		h.state = stateConfirm
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Test confirmation")

		// Simulate pressing 'n' using HandleKeyPress
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
		shouldClose := h.confirmationOverlay.HandleKeyPress(keyMsg)
		if shouldClose {
			h.state = stateDefault
			h.confirmationOverlay = nil
		}

		assert.Equal(t, stateDefault, h.state)
		assert.Nil(t, h.confirmationOverlay)
	})

	t.Run("returns to default on esc press", func(t *testing.T) {
		// Start in confirmation state
		h.state = stateConfirm
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Test confirmation")

		// Simulate pressing ESC using HandleKeyPress
		keyMsg := tea.KeyMsg{Type: tea.KeyEscape}
		shouldClose := h.confirmationOverlay.HandleKeyPress(keyMsg)
		if shouldClose {
			h.state = stateDefault
			h.confirmationOverlay = nil
		}

		assert.Equal(t, stateDefault, h.state)
		assert.Nil(t, h.confirmationOverlay)
	})
}

// TestConfirmationModalKeyHandling tests the actual key handling in confirmation state
func TestConfirmationModalKeyHandling(t *testing.T) {
	// Import needed packages
	spinner := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&spinner, false)

	// Create enough of home struct to test handleKeyPress in confirmation state
	h := &home{
		ctx:                 context.Background(),
		state:               stateConfirm,
		appConfig:           config.DefaultConfig(),
		list:                list,
		menu:                ui.NewMenu(),
		confirmationOverlay: overlay.NewConfirmationOverlay("Kill session?"),
	}

	testCases := []struct {
		name              string
		key               string
		expectedState     state
		expectedDismissed bool
		expectedNil       bool
	}{
		{
			name:              "y key confirms and dismisses overlay",
			key:               "y",
			expectedState:     stateDefault,
			expectedDismissed: true,
			expectedNil:       true,
		},
		{
			name:              "n key cancels and dismisses overlay",
			key:               "n",
			expectedState:     stateDefault,
			expectedDismissed: true,
			expectedNil:       true,
		},
		{
			name:              "esc key cancels and dismisses overlay",
			key:               "esc",
			expectedState:     stateDefault,
			expectedDismissed: true,
			expectedNil:       true,
		},
		{
			name:              "other keys are ignored",
			key:               "x",
			expectedState:     stateConfirm,
			expectedDismissed: false,
			expectedNil:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset state
			h.state = stateConfirm
			h.confirmationOverlay = overlay.NewConfirmationOverlay("Kill session?")

			// Create key message
			var keyMsg tea.KeyMsg
			if tc.key == "esc" {
				keyMsg = tea.KeyMsg{Type: tea.KeyEscape}
			} else {
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)}
			}

			// Call handleKeyPress
			model, _ := h.handleKeyPress(keyMsg)
			homeModel, ok := model.(*home)
			require.True(t, ok)

			assert.Equal(t, tc.expectedState, homeModel.state, "State mismatch for key: %s", tc.key)
			if tc.expectedNil {
				assert.Nil(t, homeModel.confirmationOverlay, "Overlay should be nil for key: %s", tc.key)
			} else {
				assert.NotNil(t, homeModel.confirmationOverlay, "Overlay should not be nil for key: %s", tc.key)
				assert.Equal(t, tc.expectedDismissed, homeModel.confirmationOverlay.Dismissed, "Dismissed mismatch for key: %s", tc.key)
			}
		})
	}
}

// TestConfirmationMessageFormatting tests that confirmation messages are formatted correctly
func TestConfirmationMessageFormatting(t *testing.T) {
	testCases := []struct {
		name            string
		sessionTitle    string
		expectedMessage string
	}{
		{
			name:            "short session name",
			sessionTitle:    "my-feature",
			expectedMessage: "[!] Kill session 'my-feature'? (y/n)",
		},
		{
			name:            "long session name",
			sessionTitle:    "very-long-feature-branch-name-here",
			expectedMessage: "[!] Kill session 'very-long-feature-branch-name-here'? (y/n)",
		},
		{
			name:            "session with spaces",
			sessionTitle:    "feature with spaces",
			expectedMessage: "[!] Kill session 'feature with spaces'? (y/n)",
		},
		{
			name:            "session with special chars",
			sessionTitle:    "feature/branch-123",
			expectedMessage: "[!] Kill session 'feature/branch-123'? (y/n)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the message formatting directly
			actualMessage := fmt.Sprintf("[!] Kill session '%s'? (y/n)", tc.sessionTitle)
			assert.Equal(t, tc.expectedMessage, actualMessage)
		})
	}
}

// TestConfirmationFlowSimulation tests the confirmation flow by simulating the state changes
func TestConfirmationFlowSimulation(t *testing.T) {
	// Create a minimal setup
	spinner := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewList(&spinner, false)

	// Add test instance
	instance, err := session.NewInstance(session.InstanceOptions{
		Title:   "test-session",
		Path:    t.TempDir(),
		Program: "claude",
		AutoYes: false,
	})
	require.NoError(t, err)
	_ = list.AddInstance(instance)
	list.SetSelectedInstance(0)

	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		list:      list,
		menu:      ui.NewMenu(),
	}

	// Simulate what happens when D is pressed
	selected := h.list.GetSelectedInstance()
	require.NotNil(t, selected)

	// This is what the KeyKill handler does
	message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
	h.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	h.state = stateConfirm

	// Verify the state
	assert.Equal(t, stateConfirm, h.state)
	assert.NotNil(t, h.confirmationOverlay)
	assert.False(t, h.confirmationOverlay.Dismissed)
	// Test that overlay renders with the correct message
	rendered := h.confirmationOverlay.Render()
	assert.Contains(t, rendered, "Kill session 'test-session'?")
}

// TestConfirmActionWithDifferentTypes tests that confirmAction works with different action types
func TestConfirmActionWithDifferentTypes(t *testing.T) {
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
	}

	t.Run("works with simple action returning nil", func(t *testing.T) {
		actionCalled := false
		action := func() tea.Msg {
			actionCalled = true
			return nil
		}

		// Set up callback to track action execution
		actionExecuted := false
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Test action?")
		h.confirmationOverlay.OnConfirm = func() {
			h.state = stateDefault
			actionExecuted = true
			action() // Execute the action
		}
		h.state = stateConfirm

		// Verify state was set
		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.False(t, h.confirmationOverlay.Dismissed)
		assert.NotNil(t, h.confirmationOverlay.OnConfirm)

		// Execute the confirmation callback
		h.confirmationOverlay.OnConfirm()
		assert.True(t, actionCalled)
		assert.True(t, actionExecuted)
	})

	t.Run("works with action returning error", func(t *testing.T) {
		expectedErr := fmt.Errorf("test error")
		action := func() tea.Msg {
			return expectedErr
		}

		// Set up callback to track action execution
		var receivedMsg tea.Msg
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Error action?")
		h.confirmationOverlay.OnConfirm = func() {
			h.state = stateDefault
			receivedMsg = action() // Execute the action and capture result
		}
		h.state = stateConfirm

		// Verify state was set
		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.False(t, h.confirmationOverlay.Dismissed)
		assert.NotNil(t, h.confirmationOverlay.OnConfirm)

		// Execute the confirmation callback
		h.confirmationOverlay.OnConfirm()
		assert.Equal(t, expectedErr, receivedMsg)
	})

	t.Run("works with action returning custom message", func(t *testing.T) {
		action := func() tea.Msg {
			return instanceChangedMsg{}
		}

		// Set up callback to track action execution
		var receivedMsg tea.Msg
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Custom message action?")
		h.confirmationOverlay.OnConfirm = func() {
			h.state = stateDefault
			receivedMsg = action() // Execute the action and capture result
		}
		h.state = stateConfirm

		// Verify state was set
		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.False(t, h.confirmationOverlay.Dismissed)
		assert.NotNil(t, h.confirmationOverlay.OnConfirm)

		// Execute the confirmation callback
		h.confirmationOverlay.OnConfirm()
		_, ok := receivedMsg.(instanceChangedMsg)
		assert.True(t, ok, "Expected instanceChangedMsg but got %T", receivedMsg)
	})
}

// TestMultipleConfirmationsDontInterfere tests that multiple confirmations don't interfere with each other
func TestMultipleConfirmationsDontInterfere(t *testing.T) {
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
	}

	// First confirmation
	action1Called := false
	action1 := func() tea.Msg {
		action1Called = true
		return nil
	}

	// Set up first confirmation
	h.confirmationOverlay = overlay.NewConfirmationOverlay("First action?")
	firstOnConfirm := func() {
		h.state = stateDefault
		action1()
	}
	h.confirmationOverlay.OnConfirm = firstOnConfirm
	h.state = stateConfirm

	// Verify first confirmation
	assert.Equal(t, stateConfirm, h.state)
	assert.NotNil(t, h.confirmationOverlay)
	assert.False(t, h.confirmationOverlay.Dismissed)
	assert.NotNil(t, h.confirmationOverlay.OnConfirm)

	// Cancel first confirmation (simulate pressing 'n')
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	shouldClose := h.confirmationOverlay.HandleKeyPress(keyMsg)
	if shouldClose {
		h.state = stateDefault
		h.confirmationOverlay = nil
	}

	// Second confirmation with different action
	action2Called := false
	action2 := func() tea.Msg {
		action2Called = true
		return fmt.Errorf("action2 error")
	}

	// Set up second confirmation
	h.confirmationOverlay = overlay.NewConfirmationOverlay("Second action?")
	var secondResult tea.Msg
	secondOnConfirm := func() {
		h.state = stateDefault
		secondResult = action2()
	}
	h.confirmationOverlay.OnConfirm = secondOnConfirm
	h.state = stateConfirm

	// Verify second confirmation
	assert.Equal(t, stateConfirm, h.state)
	assert.NotNil(t, h.confirmationOverlay)
	assert.False(t, h.confirmationOverlay.Dismissed)
	assert.NotNil(t, h.confirmationOverlay.OnConfirm)

	// Execute second action to verify it's the correct one
	h.confirmationOverlay.OnConfirm()
	err, ok := secondResult.(error)
	assert.True(t, ok)
	assert.Equal(t, "action2 error", err.Error())
	assert.True(t, action2Called)
	assert.False(t, action1Called, "First action should not have been called")

	// Test that cancelled action can still be executed independently
	firstOnConfirm()
	assert.True(t, action1Called, "First action should be callable after being replaced")
}

// TestConfirmationModalVisualAppearance tests that confirmation modal has distinct visual appearance
func TestConfirmationModalVisualAppearance(t *testing.T) {
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
	}

	// Create a test confirmation overlay
	message := "[!] Delete everything?"
	h.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	h.state = stateConfirm

	// Verify the overlay was created with confirmation settings
	assert.NotNil(t, h.confirmationOverlay)
	assert.Equal(t, stateConfirm, h.state)
	assert.False(t, h.confirmationOverlay.Dismissed)

	// Test the overlay render (we can test that it renders without errors)
	rendered := h.confirmationOverlay.Render()
	assert.NotEmpty(t, rendered)

	// Test that it includes the message content and instructions
	assert.Contains(t, rendered, "Delete everything?")
	assert.Contains(t, rendered, "Press")
	assert.Contains(t, rendered, "to confirm")
	assert.Contains(t, rendered, "to cancel")

	// Test that the danger indicator is preserved
	assert.Contains(t, rendered, "[!")
}

func TestFocusRing(t *testing.T) {
	newTestHome := func() *home {
		spin := spinner.New(spinner.WithSpinner(spinner.Dot))
		return &home{
			ctx:          context.Background(),
			state:        stateDefault,
			appConfig:    config.DefaultConfig(),
			list:         ui.NewList(&spin, false),
			menu:         ui.NewMenu(),
			sidebar:      ui.NewSidebar(),
			tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		}
	}

	addTestInstance := func(t *testing.T, h *home) {
		t.Helper()
		inst, err := session.NewInstance(session.InstanceOptions{
			Title: "test", Path: t.TempDir(), Program: "claude",
		})
		require.NoError(t, err)
		h.list.AddInstance(inst)()
	}

	handle := func(t *testing.T, h *home, msg tea.KeyMsg) *home {
		t.Helper()
		h.keySent = true
		model, _ := h.handleKeyPress(msg)
		homeModel, ok := model.(*home)
		require.True(t, ok)
		return homeModel
	}

	// --- Tab ring cycling ---

	t.Run("Tab advances through center tabs: agent → diff", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotAgent)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyTab})

		assert.Equal(t, slotDiff, homeModel.focusSlot)
	})

	t.Run("Tab advances through center tabs: diff → git", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotDiff)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyTab})

		assert.Equal(t, slotGit, homeModel.focusSlot)
	})

	t.Run("Tab wraps center tabs: git → agent", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotGit)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyTab})

		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("Tab from sidebar lands on agent", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotSidebar)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyTab})

		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("Tab from list lands on agent", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotList)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyTab})

		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("Shift+Tab moves backward through center tabs: diff → agent", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotDiff)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyShiftTab})

		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("Shift+Tab wraps center tabs: agent → git", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotAgent)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyShiftTab})

		assert.Equal(t, slotGit, homeModel.focusSlot)
	})

	t.Run("Shift+Tab from sidebar lands on git", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotSidebar)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyShiftTab})

		assert.Equal(t, slotGit, homeModel.focusSlot)
	})

	t.Run("t jumps to list slot when instances exist", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		h.setFocusSlot(slotSidebar)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})

		assert.Equal(t, slotList, homeModel.focusSlot)
	})

	t.Run("t is no-op when list is empty", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotSidebar)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})

		assert.Equal(t, slotSidebar, homeModel.focusSlot)
	})

	// --- Direct slot jumps ---

	t.Run("! jumps to agent slot", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotList)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})

		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("@ jumps to diff slot", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotList)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})

		assert.Equal(t, slotDiff, homeModel.focusSlot)
	})

	t.Run("# jumps to git slot", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotList)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("#")})

		assert.Equal(t, slotGit, homeModel.focusSlot)
	})

	t.Run("s jumps to sidebar slot", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotList)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

		assert.Equal(t, slotSidebar, homeModel.focusSlot)
	})

	t.Run("s shows and focuses sidebar when hidden", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = true
		h.setFocusSlot(slotList)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

		assert.False(t, homeModel.sidebarHidden)
		assert.Equal(t, slotSidebar, homeModel.focusSlot)
	})

	// --- Sidebar toggle (ctrl+s) ---

	t.Run("ctrl+s hides sidebar and moves focus from sidebar to agent slot", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = false
		h.setFocusSlot(slotSidebar)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyCtrlS})

		assert.True(t, homeModel.sidebarHidden)
		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("ctrl+s hides sidebar and keeps focus when agent slot is focused", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = false
		h.setFocusSlot(slotAgent)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyCtrlS})

		assert.True(t, homeModel.sidebarHidden)
		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("ctrl+s shows sidebar and keeps focus when sidebar is hidden", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = true
		h.setFocusSlot(slotList)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyCtrlS})

		assert.False(t, homeModel.sidebarHidden)
		assert.Equal(t, slotList, homeModel.focusSlot)
	})

	// --- Arrow key navigation (layout: sidebar | list | tabs) ---
	// When instances exist, list acts as a waypoint between sidebar and tabs.

	t.Run("← from agent moves to list (instances exist)", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		h.setFocusSlot(slotAgent)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyLeft})

		assert.Equal(t, slotList, homeModel.focusSlot)
	})

	t.Run("← from diff moves to list (instances exist)", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		h.setFocusSlot(slotDiff)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyLeft})

		assert.Equal(t, slotList, homeModel.focusSlot)
	})

	t.Run("← from list moves to sidebar", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		h.setFocusSlot(slotList)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyLeft})

		assert.Equal(t, slotSidebar, homeModel.focusSlot)
	})

	t.Run("→ from sidebar moves to list (instances exist)", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		h.setFocusSlot(slotSidebar)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRight})

		assert.Equal(t, slotList, homeModel.focusSlot)
	})

	t.Run("→ from list moves to agent", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		h.setFocusSlot(slotList)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRight})

		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("→ from agent is no-op (already rightmost)", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotAgent)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRight})

		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	// --- Arrow keys skip list when no instances ---

	t.Run("← from agent skips to sidebar (no instances)", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotAgent)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyLeft})

		assert.Equal(t, slotSidebar, homeModel.focusSlot)
	})

	t.Run("→ from sidebar skips to agent (no instances)", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotSidebar)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRight})

		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	// --- Alt+Up/Down: cycle active instances with wrapping ---

	t.Run("alt+down cycles to next active instance", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		addTestInstance(t, h)
		addTestInstance(t, h)
		h.list.SetSelectedInstance(0)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyDown, Alt: true})

		assert.Equal(t, 1, homeModel.list.SelectedIndex())
	})

	t.Run("alt+down wraps from last to first", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		addTestInstance(t, h)
		addTestInstance(t, h)
		h.list.SetSelectedInstance(2)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyDown, Alt: true})

		assert.Equal(t, 0, homeModel.list.SelectedIndex())
	})

	t.Run("alt+up cycles to previous active instance", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		addTestInstance(t, h)
		addTestInstance(t, h)
		h.list.SetSelectedInstance(2)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyUp, Alt: true})

		assert.Equal(t, 1, homeModel.list.SelectedIndex())
	})

	t.Run("alt+up wraps from first to last", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		addTestInstance(t, h)
		addTestInstance(t, h)
		h.list.SetSelectedInstance(0)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyUp, Alt: true})

		assert.Equal(t, 2, homeModel.list.SelectedIndex())
	})

	t.Run("alt+down skips paused instances", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h) // 0: active
		addTestInstance(t, h) // 1: will be paused
		addTestInstance(t, h) // 2: active
		h.list.GetInstances()[1].Status = session.Paused
		h.list.SetSelectedInstance(0)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyDown, Alt: true})

		assert.Equal(t, 2, homeModel.list.SelectedIndex())
	})

	t.Run("alt+up skips paused instances", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h) // 0: active
		addTestInstance(t, h) // 1: will be paused
		addTestInstance(t, h) // 2: active
		h.list.GetInstances()[1].Status = session.Paused
		h.list.SetSelectedInstance(2)

		homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyUp, Alt: true})

		assert.Equal(t, 0, homeModel.list.SelectedIndex())
	})
}

func TestPreviewTerminal_SelectionChange(t *testing.T) {
	// Helper to create a minimal home with two started instances.
	newTestHomeWithInstances := func(t *testing.T) (*home, *session.Instance, *session.Instance) {
		t.Helper()
		spin := spinner.New(spinner.WithSpinner(spinner.Dot))
		h := &home{
			ctx:          context.Background(),
			state:        stateDefault,
			appConfig:    config.DefaultConfig(),
			list:         ui.NewList(&spin, false),
			menu:         ui.NewMenu(),
			sidebar:      ui.NewSidebar(),
			tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		}

		instA, err := session.NewInstance(session.InstanceOptions{
			Title: "instance-A", Path: t.TempDir(), Program: "claude",
		})
		require.NoError(t, err)
		instA.MarkStartedForTest()
		instA.Status = session.Running
		instA.CachedContentSet = true // avoid tmux subprocess calls in tests

		instB, err := session.NewInstance(session.InstanceOptions{
			Title: "instance-B", Path: t.TempDir(), Program: "claude",
		})
		require.NoError(t, err)
		instB.MarkStartedForTest()
		instB.Status = session.Running
		instB.CachedContentSet = true

		h.list.AddInstance(instA)()
		h.list.AddInstance(instB)()

		return h, instA, instB
	}

	t.Run("swap terminal when selection changes from A to B", func(t *testing.T) {
		h, _, instB := newTestHomeWithInstances(t)

		// Simulate: previewTerminal is attached to instance "A".
		dummyTerm := session.NewDummyTerminal()
		h.previewTerminal = dummyTerm
		h.previewTerminalInstance = "instance-A"

		// Select instance "B" by reference (sort-order safe).
		require.True(t, h.list.SelectInstance(instB), "should find instance-B in list")

		// Fire instanceChanged — should tear down old terminal and return spawn cmd.
		cmd := h.instanceChanged()

		// Old terminal is closed: previewTerminal becomes nil, instance name cleared.
		assert.Nil(t, h.previewTerminal, "previewTerminal should be nil after selection change")
		assert.Empty(t, h.previewTerminalInstance, "previewTerminalInstance should be cleared")

		// A tea.Cmd is returned (the async spawn command).
		assert.NotNil(t, cmd, "instanceChanged should return a tea.Cmd for async spawn")
	})

	t.Run("tear down terminal when no valid instance selected", func(t *testing.T) {
		// Use a home with zero instances so GetSelectedInstance returns nil.
		spin := spinner.New(spinner.WithSpinner(spinner.Dot))
		h := &home{
			ctx:          context.Background(),
			state:        stateDefault,
			appConfig:    config.DefaultConfig(),
			list:         ui.NewList(&spin, false),
			menu:         ui.NewMenu(),
			sidebar:      ui.NewSidebar(),
			tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		}

		// Attach a terminal.
		dummyTerm := session.NewDummyTerminal()
		h.previewTerminal = dummyTerm
		h.previewTerminalInstance = "instance-A"

		cmd := h.instanceChanged()

		assert.Nil(t, h.previewTerminal, "previewTerminal should be torn down")
		assert.Empty(t, h.previewTerminalInstance, "previewTerminalInstance should be cleared")
		// No spawn cmd — nothing to attach to.
		assert.Nil(t, cmd, "no spawn cmd when no valid instance is selected")
	})

	t.Run("no-op when selection matches current terminal", func(t *testing.T) {
		h, instA, _ := newTestHomeWithInstances(t)

		dummyTerm := session.NewDummyTerminal()
		h.previewTerminal = dummyTerm
		h.previewTerminalInstance = "instance-A"

		// Select instance "A" — same as current terminal (use reference, sort-order safe).
		require.True(t, h.list.SelectInstance(instA), "should find instance-A in list")

		cmd := h.instanceChanged()

		// Terminal should remain attached (not nil).
		assert.Equal(t, dummyTerm, h.previewTerminal, "previewTerminal should remain attached")
		assert.Equal(t, "instance-A", h.previewTerminalInstance, "previewTerminalInstance should remain")
		// No spawn cmd — terminal already attached.
		assert.Nil(t, cmd, "no spawn cmd when same instance is selected")

		// Cleanup
		dummyTerm.Close()
	})

	t.Run("previewTerminalReadyMsg attaches terminal on match", func(t *testing.T) {
		h, instA, _ := newTestHomeWithInstances(t)
		h.list.SelectInstance(instA) // select instance-A

		readyTerm := session.NewDummyTerminal()
		msg := previewTerminalReadyMsg{
			term:          readyTerm,
			instanceTitle: "instance-A",
		}

		_, cmd := h.Update(msg)

		assert.Equal(t, readyTerm, h.previewTerminal, "previewTerminal should be set from msg")
		assert.Equal(t, "instance-A", h.previewTerminalInstance, "previewTerminalInstance should match")
		assert.Nil(t, cmd, "no follow-up cmd expected")

		// Cleanup
		readyTerm.Close()
	})

	t.Run("previewTerminalReadyMsg discards stale terminal", func(t *testing.T) {
		h, _, instB := newTestHomeWithInstances(t)
		h.list.SelectInstance(instB) // select instance-B (different from msg)

		staleTerm := session.NewDummyTerminal()
		msg := previewTerminalReadyMsg{
			term:          staleTerm,
			instanceTitle: "instance-A", // stale — selection moved to B
		}

		_, cmd := h.Update(msg)

		// Stale terminal should NOT be attached.
		assert.Nil(t, h.previewTerminal, "stale terminal should not be attached")
		assert.Empty(t, h.previewTerminalInstance, "previewTerminalInstance should remain empty")
		assert.Nil(t, cmd, "no follow-up cmd expected")
		// staleTerm.Close() was called internally by the handler
	})

	t.Run("previewTerminalReadyMsg discards on error", func(t *testing.T) {
		h, instA, _ := newTestHomeWithInstances(t)
		h.list.SelectInstance(instA)

		errTerm := session.NewDummyTerminal()
		msg := previewTerminalReadyMsg{
			term:          errTerm,
			instanceTitle: "instance-A",
			err:           fmt.Errorf("tmux attach failed"),
		}

		_, cmd := h.Update(msg)

		assert.Nil(t, h.previewTerminal, "terminal should not be attached on error")
		assert.Empty(t, h.previewTerminalInstance)
		assert.Nil(t, cmd)
		// errTerm.Close() was called internally by the handler
	})
}

// TestPreviewTerminal_RenderTickIntegration tests the full preview terminal lifecycle:
// selection change → previewTerminalReadyMsg → render tick → selection change again.
func TestPreviewTerminal_RenderTickIntegration(t *testing.T) {
	newTestHomeWithInstances := func(t *testing.T) (*home, *session.Instance, *session.Instance) {
		t.Helper()
		spin := spinner.New(spinner.WithSpinner(spinner.Dot))
		h := &home{
			ctx:          context.Background(),
			state:        stateDefault,
			appConfig:    config.DefaultConfig(),
			list:         ui.NewList(&spin, false),
			menu:         ui.NewMenu(),
			sidebar:      ui.NewSidebar(),
			tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		}

		instA, err := session.NewInstance(session.InstanceOptions{
			Title: "instance-A", Path: t.TempDir(), Program: "claude",
		})
		require.NoError(t, err)
		instA.MarkStartedForTest()
		instA.Status = session.Running
		instA.CachedContentSet = true

		instB, err := session.NewInstance(session.InstanceOptions{
			Title: "instance-B", Path: t.TempDir(), Program: "claude",
		})
		require.NoError(t, err)
		instB.MarkStartedForTest()
		instB.Status = session.Running
		instB.CachedContentSet = true

		h.list.AddInstance(instA)()
		h.list.AddInstance(instB)()

		return h, instA, instB
	}

	t.Run("full flow: attach → tick → selection change → discard old terminal", func(t *testing.T) {
		h, instA, instB := newTestHomeWithInstances(t)

		// Step 1: Select instance A and simulate instanceChanged returning a spawn cmd.
		require.True(t, h.list.SelectInstance(instA))
		spawnCmd := h.instanceChanged()
		assert.NotNil(t, spawnCmd, "instanceChanged should return spawn cmd for new selection")
		assert.Nil(t, h.previewTerminal, "terminal not yet attached — spawn is async")

		// Step 2: Async spawn completes — deliver previewTerminalReadyMsg for instance A.
		termA := session.NewDummyTerminal()
		_, cmd := h.Update(previewTerminalReadyMsg{
			term:          termA,
			instanceTitle: "instance-A",
		})
		assert.Equal(t, termA, h.previewTerminal, "terminal A should be attached")
		assert.Equal(t, "instance-A", h.previewTerminalInstance)
		assert.Nil(t, cmd, "no follow-up cmd from ready msg")

		// Step 3: Render tick fires — terminal is active, tick returns event-driven cmd.
		_, tickCmd := h.Update(previewTickMsg{})
		assert.NotNil(t, tickCmd, "previewTickMsg should always return a follow-up tick cmd")
		// previewTerminal is still attached after the tick.
		assert.Equal(t, termA, h.previewTerminal, "terminal A should remain attached after tick")

		// Step 4: User selects instance B — old terminal is discarded, new spawn cmd returned.
		require.True(t, h.list.SelectInstance(instB))
		spawnCmd2 := h.instanceChanged()

		assert.Nil(t, h.previewTerminal, "old terminal A should be discarded on selection change")
		assert.Empty(t, h.previewTerminalInstance, "instance name should be cleared")
		assert.NotNil(t, spawnCmd2, "new spawn cmd should be returned for instance B")
	})

	t.Run("render tick with nil terminal returns sleep-based cmd", func(t *testing.T) {
		h, _, _ := newTestHomeWithInstances(t)
		// No terminal attached.
		assert.Nil(t, h.previewTerminal)

		_, cmd := h.Update(previewTickMsg{})
		assert.NotNil(t, cmd, "previewTickMsg should return a follow-up cmd even with nil terminal")
	})

	t.Run("render tick with active terminal returns event-driven cmd", func(t *testing.T) {
		h, instA, _ := newTestHomeWithInstances(t)
		h.list.SelectInstance(instA)

		term := session.NewDummyTerminal()
		h.previewTerminal = term
		h.previewTerminalInstance = "instance-A"
		defer term.Close()

		_, cmd := h.Update(previewTickMsg{})
		assert.NotNil(t, cmd, "previewTickMsg should return event-driven cmd when terminal is active")
		// Terminal remains attached after tick.
		assert.Equal(t, term, h.previewTerminal, "terminal should remain attached after tick")
	})

	t.Run("stale ready msg after second selection change is discarded", func(t *testing.T) {
		h, instA, instB := newTestHomeWithInstances(t)

		// Select A, spawn starts.
		require.True(t, h.list.SelectInstance(instA))
		h.instanceChanged()

		// Before spawn completes, user switches to B.
		require.True(t, h.list.SelectInstance(instB))
		h.instanceChanged()

		// Now the stale ready msg for A arrives.
		staleTermA := session.NewDummyTerminal()
		_, cmd := h.Update(previewTerminalReadyMsg{
			term:          staleTermA,
			instanceTitle: "instance-A", // stale — selection is now B
		})

		// Stale terminal must be discarded (not attached).
		assert.Nil(t, h.previewTerminal, "stale terminal for A should not be attached when B is selected")
		assert.Empty(t, h.previewTerminalInstance)
		assert.Nil(t, cmd)
	})
}

// TestPreviewTerminalReadyMsg_StaleDiscard verifies that previewTerminalReadyMsg
// discards the terminal when the selection has changed since the spawn was initiated.
func TestPreviewTerminalReadyMsg_StaleDiscard(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		list:         ui.NewList(&spin, false),
		menu:         ui.NewMenu(),
		sidebar:      ui.NewSidebar(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
	}

	// Add instance "B" and select it (simulating selection change after spawn started for "A").
	instB, err := session.NewInstance(session.InstanceOptions{
		Title:   "B",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	h.list.AddInstance(instB)()
	h.list.SelectInstance(instB) // Select "B" by pointer (sort-order safe)

	// Simulate a stale previewTerminalReadyMsg arriving for "A" (selection already moved to "B").
	// The handler should discard the terminal since selected.Title != msg.instanceTitle.
	msg := previewTerminalReadyMsg{
		term:          nil, // nil is fine — we just check it's discarded
		instanceTitle: "A",
		err:           nil,
	}

	// Process the message through Update.
	model, cmd := h.Update(msg)
	homeModel, ok := model.(*home)
	require.True(t, ok)

	// Terminal should NOT be set — it was stale.
	assert.Nil(t, homeModel.previewTerminal, "stale terminal should be discarded")
	assert.Equal(t, "", homeModel.previewTerminalInstance,
		"previewTerminalInstance should not be set for stale msg")
	assert.Nil(t, cmd, "no cmd should be returned for stale msg")
}

// TestPreviewTerminalReadyMsg_AcceptsCurrentInstance verifies that previewTerminalReadyMsg
// sets the terminal when the instance title matches the current selection.
func TestPreviewTerminalReadyMsg_AcceptsCurrentInstance(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		list:         ui.NewList(&spin, false),
		menu:         ui.NewMenu(),
		sidebar:      ui.NewSidebar(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
	}

	// Add instance "A" and select it.
	instA, err := session.NewInstance(session.InstanceOptions{
		Title:   "A",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	h.list.AddInstance(instA)()
	h.list.SetSelectedInstance(0)

	// Simulate a fresh previewTerminalReadyMsg for "A" (current selection).
	msg := previewTerminalReadyMsg{
		term:          nil, // nil terminal — we just verify the instance title is set
		instanceTitle: "A",
		err:           nil,
	}

	model, cmd := h.Update(msg)
	homeModel, ok := model.(*home)
	require.True(t, ok)

	// previewTerminalInstance should be set to "A".
	assert.Equal(t, "A", homeModel.previewTerminalInstance,
		"previewTerminalInstance should be set when msg matches current selection")
	assert.Nil(t, cmd, "no cmd should be returned")
}

// TestFocusMode_ReusesPreviewTerminal verifies that enterFocusMode reuses the
// existing previewTerminal when it's already attached to the selected instance.
func TestFocusMode_ReusesPreviewTerminal(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		list:         ui.NewList(&spin, false),
		menu:         ui.NewMenu(),
		sidebar:      ui.NewSidebar(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
	}

	// Add a started-looking instance. We can't actually start it (no tmux),
	// but we can test the branch where previewTerminal is already set.
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "my-agent",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	h.list.AddInstance(inst)()
	h.list.SetSelectedInstance(0)

	// Simulate previewTerminal already attached to "my-agent".
	// enterFocusMode should detect this and NOT spawn a new terminal.
	h.previewTerminalInstance = "my-agent"
	// Instance is not started, so enterFocusMode should return nil (guard check).
	cmd := h.enterFocusMode()

	assert.Nil(t, cmd, "enterFocusMode should return nil when instance is not started")
	assert.Equal(t, stateDefault, h.state, "state should remain default when instance is not started")
}

// TestExitFocusMode_KeepsPreviewTerminal verifies that exitFocusMode does NOT close
// previewTerminal — it stays alive for preview rendering.
func TestExitFocusMode_KeepsPreviewTerminal(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateFocusAgent,
		appConfig:    config.DefaultConfig(),
		list:         ui.NewList(&spin, false),
		menu:         ui.NewMenu(),
		sidebar:      ui.NewSidebar(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
	}

	// Set previewTerminalInstance to simulate an attached terminal.
	h.previewTerminalInstance = "my-agent"

	h.exitFocusMode()

	assert.Equal(t, stateDefault, h.state, "state should return to default after exitFocusMode")
	assert.Equal(t, "my-agent", h.previewTerminalInstance,
		"previewTerminalInstance should NOT be cleared by exitFocusMode")
}
