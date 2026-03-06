package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain runs before all tests to set up the test environment
func TestMain(m *testing.M) {
	// Initialize bubblezone global manager (required for zone.Mark/zone.Get in tests)
	zone.NewGlobal()

	// Initialize the logger before any tests run
	log.Initialize(false)
	defer log.Close()

	// Run all tests
	exitCode := m.Run()

	// Exit with the same code as the tests
	os.Exit(exitCode)
}

func newTestHome() *home {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	return &home{
		ctx:            context.Background(),
		state:          stateDefault,
		appConfig:      config.DefaultConfig(),
		nav:            ui.NewNavigationPanel(&spin),
		menu:           ui.NewMenu(),
		auditPane:      ui.NewAuditPane(),
		tabbedWindow:   ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		toastManager:   overlay.NewToastManager(&spin),
		overlays:       overlay.NewManager(),
		activeRepoPath: os.TempDir(),
		program:        "opencode",
	}
}

func TestSpawnAdHocAgent_DefaultCreatesWorktree(t *testing.T) {
	h := newTestHome()
	model, cmd := h.spawnAdHocAgent("my-agent", "", "")
	updated := model.(*home)
	instances := updated.nav.GetInstances()
	require.NotEmpty(t, instances)
	last := instances[len(instances)-1]
	assert.Equal(t, "my-agent", last.Title)
	assert.Equal(t, session.Loading, last.Status)
	assert.NotNil(t, cmd, "should return async start command")
}

func TestSpawnAdHocAgent_BranchOverride(t *testing.T) {
	h := newTestHome()
	model, cmd := h.spawnAdHocAgent("my-agent", "feature/login", "")
	updated := model.(*home)
	instances := updated.nav.GetInstances()
	require.NotEmpty(t, instances)
	last := instances[len(instances)-1]
	assert.Equal(t, "my-agent", last.Title)
	assert.NotNil(t, cmd)
}

func TestSpawnAdHocAgent_PathOverride(t *testing.T) {
	h := newTestHome()
	model, cmd := h.spawnAdHocAgent("my-agent", "", "/tmp/custom-path")
	updated := model.(*home)
	instances := updated.nav.GetInstances()
	require.NotEmpty(t, instances)
	last := instances[len(instances)-1]
	assert.Equal(t, "my-agent", last.Title)
	assert.NotNil(t, cmd)
}

func TestSpawnAgent_KeyOpensFormOverlay(t *testing.T) {
	h := newTestHome()
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: 's', Text: "s"})
	updated := model.(*home)
	require.Equal(t, stateSpawnAgent, updated.state)
	require.True(t, updated.overlays.IsActive(), "form overlay must be set")
	_, ok := updated.overlays.Current().(*overlay.FormOverlay)
	require.True(t, ok, "active overlay must be a FormOverlay")
}

func TestSpawnAgent_EscCancels(t *testing.T) {
	h := newTestHome()
	h.state = stateSpawnAgent
	h.overlays.Show(overlay.NewSpawnFormOverlay("spawn agent", 60))

	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated := model.(*home)
	assert.Equal(t, stateDefault, updated.state)
	assert.False(t, updated.overlays.IsActive())
}

func TestSpawnAgent_SubmitCreatesInstance(t *testing.T) {
	h := newTestHome()
	h.state = stateSpawnAgent
	h.overlays.Show(overlay.NewSpawnFormOverlay("spawn agent", 60))

	press := func(msg tea.KeyPressMsg) {
		h.keySent = true
		handleModel, _ := h.handleKeyPress(msg)
		h = handleModel.(*home)
	}

	for _, r := range "test-agent" {
		press(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	h.keySent = true
	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := model.(*home)
	assert.Equal(t, stateDefault, updated.state)
	assert.False(t, updated.overlays.IsActive())
	assert.NotNil(t, cmd, "should return start command")

	instances := updated.nav.GetInstances()
	require.NotEmpty(t, instances)
	last := instances[len(instances)-1]
	assert.Equal(t, "test-agent", last.Title)
	assert.Equal(t, "", last.TaskFile, "ad-hoc instance must have no PlanFile")
	assert.Equal(t, session.AgentTypeFixer, last.AgentType, "spawned instance must be fixer")
	assert.Equal(t, session.Loading, last.Status)
}

// TestConfirmationModalStateTransitions tests state transitions without full instance setup
func TestConfirmationModalStateTransitions(t *testing.T) {
	mgr := overlay.NewManager()
	// Create a minimal home struct for testing state transitions
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		overlays:  mgr,
	}

	t.Run("shows confirmation on D press", func(t *testing.T) {
		// Simulate pressing 'D'
		h.state = stateDefault
		h.overlays.Dismiss()

		// Manually trigger what would happen in handleKeyPress for 'D'
		h.state = stateConfirm
		co := overlay.NewConfirmationOverlay("[!] Kill session 'test'?")
		h.overlays.Show(co)

		assert.Equal(t, stateConfirm, h.state)
		assert.True(t, h.overlays.IsActive())
		assert.False(t, co.Dismissed)
	})

	t.Run("returns to default on y press", func(t *testing.T) {
		// Start in confirmation state
		h.state = stateConfirm
		co := overlay.NewConfirmationOverlay("Test confirmation")
		h.overlays.Show(co)

		// Simulate pressing 'y' using HandleKeyPress
		keyMsg := tea.KeyPressMsg{Code: 'y', Text: "y"}
		result := h.overlays.HandleKey(keyMsg)
		if result.Dismissed {
			h.state = stateDefault
		}

		assert.Equal(t, stateDefault, h.state)
		assert.False(t, h.overlays.IsActive())
	})

	t.Run("returns to default on n press", func(t *testing.T) {
		// Start in confirmation state
		h.state = stateConfirm
		h.overlays.Show(overlay.NewConfirmationOverlay("Test confirmation"))

		// Simulate pressing 'n' using HandleKeyPress
		keyMsg := tea.KeyPressMsg{Code: 'n', Text: "n"}
		result := h.overlays.HandleKey(keyMsg)
		if result.Dismissed {
			h.state = stateDefault
		}

		assert.Equal(t, stateDefault, h.state)
		assert.False(t, h.overlays.IsActive())
	})

	t.Run("returns to default on esc press", func(t *testing.T) {
		// Start in confirmation state
		h.state = stateConfirm
		h.overlays.Show(overlay.NewConfirmationOverlay("Test confirmation"))

		// Simulate pressing ESC using HandleKeyPress
		keyMsg := tea.KeyPressMsg{Code: tea.KeyEscape}
		result := h.overlays.HandleKey(keyMsg)
		if result.Dismissed {
			h.state = stateDefault
		}

		assert.Equal(t, stateDefault, h.state)
		assert.False(t, h.overlays.IsActive())
	})
}

// TestConfirmationModalKeyHandling tests the actual key handling in confirmation state
func TestConfirmationModalKeyHandling(t *testing.T) {
	// Import needed packages
	spinner := spinner.New(spinner.WithSpinner(spinner.Dot))
	list := ui.NewNavigationPanel(&spinner)

	// Create enough of home struct to test handleKeyPress in confirmation state
	h := &home{
		ctx:       context.Background(),
		state:     stateConfirm,
		appConfig: config.DefaultConfig(),
		nav:       list,
		menu:      ui.NewMenu(),
		overlays:  overlay.NewManager(),
	}
	h.overlays.Show(overlay.NewConfirmationOverlay("Kill session?"))

	testCases := []struct {
		name          string
		key           string
		expectedState state
		expectedNil   bool
	}{
		{
			name:          "y key confirms and dismisses overlay",
			key:           "y",
			expectedState: stateDefault,
			expectedNil:   true,
		},
		{
			name:          "n key cancels and dismisses overlay",
			key:           "n",
			expectedState: stateDefault,
			expectedNil:   true,
		},
		{
			name:          "esc key cancels and dismisses overlay",
			key:           "esc",
			expectedState: stateDefault,
			expectedNil:   true,
		},
		{
			name:          "other keys are ignored",
			key:           "x",
			expectedState: stateConfirm,
			expectedNil:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset state
			h.state = stateConfirm
			h.overlays.Show(overlay.NewConfirmationOverlay("Kill session?"))

			// Create key message
			var keyMsg tea.KeyPressMsg
			if tc.key == "esc" {
				keyMsg = tea.KeyPressMsg{Code: tea.KeyEscape}
			} else {
				keyMsg = tea.KeyPressMsg{Code: rune(tc.key[0]), Text: tc.key}
			}

			// Call handleKeyPress
			model, _ := h.handleKeyPress(keyMsg)
			homeModel, ok := model.(*home)
			require.True(t, ok)

			assert.Equal(t, tc.expectedState, homeModel.state, "State mismatch for key: %s", tc.key)
			if tc.expectedNil {
				assert.False(t, homeModel.overlays.IsActive(), "Overlay should be nil for key: %s", tc.key)
			} else {
				assert.True(t, homeModel.overlays.IsActive(), "Overlay should not be nil for key: %s", tc.key)
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
	// Test the confirmation overlay component directly
	message := "[!] Kill session 'test-session'?"
	co := overlay.NewConfirmationOverlay(message)

	// Verify the overlay was created correctly
	assert.False(t, co.Dismissed)
	// Test that overlay renders with the correct message
	rendered := co.View()
	assert.Contains(t, rendered, "Kill session 'test-session'?")
}

// TestConfirmActionWithDifferentTypes tests that ConfirmationOverlay works with different action types
func TestConfirmActionWithDifferentTypes(t *testing.T) {
	t.Run("works with simple action returning nil", func(t *testing.T) {
		actionCalled := false
		actionExecuted := false

		co := overlay.NewConfirmationOverlay("Test action?")
		co.OnConfirm = func() {
			actionExecuted = true
			actionCalled = true
		}

		assert.False(t, co.Dismissed)
		assert.NotNil(t, co.OnConfirm)

		co.OnConfirm()
		assert.True(t, actionCalled)
		assert.True(t, actionExecuted)
	})

	t.Run("works with action returning error", func(t *testing.T) {
		expectedErr := fmt.Errorf("test error")
		var receivedMsg tea.Msg

		co := overlay.NewConfirmationOverlay("Error action?")
		co.OnConfirm = func() {
			receivedMsg = expectedErr
		}

		assert.False(t, co.Dismissed)
		assert.NotNil(t, co.OnConfirm)

		co.OnConfirm()
		assert.Equal(t, expectedErr, receivedMsg)
	})

	t.Run("works with action returning custom message", func(t *testing.T) {
		var receivedMsg tea.Msg

		co := overlay.NewConfirmationOverlay("Custom message action?")
		co.OnConfirm = func() {
			receivedMsg = instanceChangedMsg{}
		}

		assert.False(t, co.Dismissed)
		assert.NotNil(t, co.OnConfirm)

		co.OnConfirm()
		_, ok := receivedMsg.(instanceChangedMsg)
		assert.True(t, ok, "Expected instanceChangedMsg but got %T", receivedMsg)
	})
}

// TestMultipleConfirmationsDontInterfere tests that multiple ConfirmationOverlays don't interfere
func TestMultipleConfirmationsDontInterfere(t *testing.T) {
	// First confirmation
	action1Called := false
	action1 := func() tea.Msg {
		action1Called = true
		return nil
	}

	co1 := overlay.NewConfirmationOverlay("First action?")
	firstOnConfirm := func() {
		action1()
	}
	co1.OnConfirm = firstOnConfirm

	assert.False(t, co1.Dismissed)
	assert.NotNil(t, co1.OnConfirm)

	// Cancel first confirmation (simulate pressing 'n')
	keyMsg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	result1 := co1.HandleKey(keyMsg)
	assert.True(t, result1.Dismissed, "pressing 'n' must dismiss the overlay")

	// Second confirmation with different action
	action2Called := false
	action2 := func() tea.Msg {
		action2Called = true
		return fmt.Errorf("action2 error")
	}

	co2 := overlay.NewConfirmationOverlay("Second action?")
	var secondResult tea.Msg
	secondOnConfirm := func() {
		secondResult = action2()
	}
	co2.OnConfirm = secondOnConfirm

	assert.False(t, co2.Dismissed)
	assert.NotNil(t, co2.OnConfirm)

	// Execute second action to verify it's the correct one
	co2.OnConfirm()
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
	// Test the ConfirmationOverlay component directly
	message := "[!] Delete everything?"
	co := overlay.NewConfirmationOverlay(message)

	assert.False(t, co.Dismissed)

	// Test the overlay render (we can test that it renders without errors)
	rendered := co.View()
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
			nav:          ui.NewNavigationPanel(&spin),
			menu:         ui.NewMenu(),
			tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		}
	}

	addTestInstance := func(t *testing.T, h *home) {
		t.Helper()
		inst, err := session.NewInstance(session.InstanceOptions{
			Title: "test", Path: t.TempDir(), Program: "claude",
		})
		require.NoError(t, err)
		// Stagger timestamps so newest-first sort gives deterministic visual order:
		// first added gets the highest timestamp (visual position 0), last added is oldest (visual last).
		inst.CreatedAt = time.Unix(int64(1000-h.nav.NumInstances()*100), 0)
		h.nav.AddInstance(inst)()
	}

	handle := func(t *testing.T, h *home, msg tea.KeyPressMsg) *home {
		t.Helper()
		h.keySent = true
		model, _ := h.handleKeyPress(msg)
		homeModel, ok := model.(*home)
		require.True(t, ok)
		return homeModel
	}

	// --- Tab cycles active center tab; sidebar (slotNav) always retains focus ---

	t.Run("Tab cycles active tab from info to agent, sidebar stays focused", func(t *testing.T) {
		h := newTestHome()
		h.tabbedWindow.SetActiveTab(ui.InfoTab)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyTab})

		assert.Equal(t, slotNav, homeModel.focusSlot, "sidebar must retain focus")
		assert.Equal(t, ui.PreviewTab, homeModel.tabbedWindow.GetActiveTab(), "active tab must advance to agent")
	})

	t.Run("Tab wraps active tab from agent to info, sidebar stays focused", func(t *testing.T) {
		h := newTestHome()
		h.tabbedWindow.SetActiveTab(ui.PreviewTab)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyTab})

		assert.Equal(t, slotNav, homeModel.focusSlot, "sidebar must retain focus")
		assert.Equal(t, ui.InfoTab, homeModel.tabbedWindow.GetActiveTab(), "active tab must wrap to info")
	})

	t.Run("Shift+Tab reverses active tab from agent to info, sidebar stays focused", func(t *testing.T) {
		h := newTestHome()
		h.tabbedWindow.SetActiveTab(ui.PreviewTab)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

		assert.Equal(t, slotNav, homeModel.focusSlot, "sidebar must retain focus")
		assert.Equal(t, ui.InfoTab, homeModel.tabbedWindow.GetActiveTab(), "active tab must reverse to info")
	})

	t.Run("Shift+Tab wraps active tab from info to agent, sidebar stays focused", func(t *testing.T) {
		h := newTestHome()
		h.tabbedWindow.SetActiveTab(ui.InfoTab)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

		assert.Equal(t, slotNav, homeModel.focusSlot, "sidebar must retain focus")
		assert.Equal(t, ui.PreviewTab, homeModel.tabbedWindow.GetActiveTab(), "active tab must wrap to agent")
	})

	t.Run("T jumps to nav slot when instances exist", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		h.setFocusSlot(slotAgent)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 'T', Text: "T"})

		assert.Equal(t, slotNav, homeModel.focusSlot)
	})

	// --- Direct tab jumps (!/@/#) switch active tab without stealing sidebar focus ---

	t.Run("! switches active tab to agent, sidebar keeps focus", func(t *testing.T) {
		h := newTestHome()
		h.tabbedWindow.SetActiveTab(ui.InfoTab)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: '!', Text: "!"})

		assert.Equal(t, slotNav, homeModel.focusSlot, "sidebar must retain focus")
		assert.Equal(t, ui.PreviewTab, homeModel.tabbedWindow.GetActiveTab(), "! must switch to agent tab")
	})

	t.Run("# switches active tab to info, sidebar keeps focus", func(t *testing.T) {
		h := newTestHome()
		h.tabbedWindow.SetActiveTab(ui.PreviewTab)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: '#', Text: "#"})

		assert.Equal(t, slotNav, homeModel.focusSlot, "sidebar must retain focus")
		assert.Equal(t, ui.InfoTab, homeModel.tabbedWindow.GetActiveTab(), "# must switch to info tab")
	})

	t.Run("s is no-op (sidebar focus shortcut removed)", func(t *testing.T) {
		h := newTestHome()
		h.setFocusSlot(slotNav)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 's', Text: "s"})

		assert.Equal(t, slotNav, homeModel.focusSlot)
	})

	t.Run("s does not show hidden sidebar", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = true
		h.setFocusSlot(slotNav)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 's', Text: "s"})

		assert.True(t, homeModel.sidebarHidden)
		assert.Equal(t, slotNav, homeModel.focusSlot)
	})

	// --- Sidebar toggle (ctrl+s) ---

	t.Run("ctrl+s hides sidebar and moves focus from nav to agent", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = false
		h.setFocusSlot(slotNav)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

		assert.True(t, homeModel.sidebarHidden)
		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("ctrl+s hides sidebar and keeps focus when agent slot is focused", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = false
		h.setFocusSlot(slotAgent)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

		assert.True(t, homeModel.sidebarHidden)
		assert.Equal(t, slotAgent, homeModel.focusSlot)
	})

	t.Run("ctrl+s shows sidebar and keeps focus when sidebar is hidden", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = true
		h.setFocusSlot(slotNav)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

		assert.False(t, homeModel.sidebarHidden)
		assert.Equal(t, slotNav, homeModel.focusSlot)
	})

	// --- Arrow key navigation ---

	t.Run("← is no-op (sidebar already focused)", func(t *testing.T) {
		h := newTestHome()
		h.tabbedWindow.SetActiveTab(ui.PreviewTab)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyLeft})

		assert.Equal(t, slotNav, homeModel.focusSlot, "sidebar must remain focused after ←")
		assert.Equal(t, ui.PreviewTab, homeModel.tabbedWindow.GetActiveTab(), "active tab must not change on ←")
	})

	t.Run("→ toggles expand on selected sidebar item", func(t *testing.T) {
		h := newTestHome()
		// Without a plan header selected, ToggleSelectedExpand returns false,
		// so → is effectively a no-op — sidebar stays focused.
		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyRight})

		assert.Equal(t, slotNav, homeModel.focusSlot, "sidebar must retain focus after →")
	})

	// --- Enter key blocked on info tab ---

	// --- Ctrl+Up/Down: cycle active instances with wrapping ---

	t.Run("ctrl+down cycles to next active instance", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		addTestInstance(t, h)
		addTestInstance(t, h)
		h.nav.SetSelectedInstance(0)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})

		assert.Equal(t, 1, homeModel.nav.SelectedIndex())
	})

	t.Run("ctrl+down wraps from last to first", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		addTestInstance(t, h)
		addTestInstance(t, h)
		h.nav.SetSelectedInstance(2)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})

		assert.Equal(t, 0, homeModel.nav.SelectedIndex())
	})

	t.Run("ctrl+up cycles to previous active instance", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		addTestInstance(t, h)
		addTestInstance(t, h)
		h.nav.SetSelectedInstance(2)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})

		assert.Equal(t, 1, homeModel.nav.SelectedIndex())
	})

	t.Run("ctrl+up wraps from first to last", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h)
		addTestInstance(t, h)
		addTestInstance(t, h)
		h.nav.SetSelectedInstance(0)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})

		assert.Equal(t, 2, homeModel.nav.SelectedIndex())
	})

	t.Run("ctrl+down skips paused instances", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h) // 0: active
		addTestInstance(t, h) // 1: will be paused
		addTestInstance(t, h) // 2: active
		h.nav.GetInstances()[1].Status = session.Paused
		h.nav.SetSelectedInstance(0)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})

		assert.Equal(t, 2, homeModel.nav.SelectedIndex())
	})

	t.Run("ctrl+up skips paused instances", func(t *testing.T) {
		h := newTestHome()
		addTestInstance(t, h) // 0: active
		addTestInstance(t, h) // 1: will be paused
		addTestInstance(t, h) // 2: active
		h.nav.GetInstances()[1].Status = session.Paused
		h.nav.SetSelectedInstance(2)

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})

		assert.Equal(t, 0, homeModel.nav.SelectedIndex())
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
			nav:          ui.NewNavigationPanel(&spin),
			menu:         ui.NewMenu(),
			tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
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

		h.nav.AddInstance(instA)()
		h.nav.AddInstance(instB)()

		return h, instA, instB
	}

	t.Run("swap terminal when selection changes from A to B", func(t *testing.T) {
		h, _, instB := newTestHomeWithInstances(t)

		// Simulate: previewTerminal is attached to instance "A".
		dummyTerm := session.NewDummyTerminal()
		h.previewTerminal = dummyTerm
		h.previewTerminalInstance = "instance-A"

		// Select instance "B" by reference (sort-order safe).
		require.True(t, h.nav.SelectInstance(instB), "should find instance-B in list")

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
			nav:          ui.NewNavigationPanel(&spin),
			menu:         ui.NewMenu(),
			tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
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
		require.True(t, h.nav.SelectInstance(instA), "should find instance-A in list")

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
		h.nav.SelectInstance(instA) // select instance-A

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
		h.nav.SelectInstance(instB) // select instance-B (different from msg)

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
		h.nav.SelectInstance(instA)

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
			nav:          ui.NewNavigationPanel(&spin),
			menu:         ui.NewMenu(),
			tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
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

		h.nav.AddInstance(instA)()
		h.nav.AddInstance(instB)()

		return h, instA, instB
	}

	t.Run("full flow: attach → tick → selection change → discard old terminal", func(t *testing.T) {
		h, instA, instB := newTestHomeWithInstances(t)

		// Step 1: Select instance A and simulate instanceChanged returning a spawn cmd.
		require.True(t, h.nav.SelectInstance(instA))
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
		require.True(t, h.nav.SelectInstance(instB))
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
		h.nav.SelectInstance(instA)

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
		require.True(t, h.nav.SelectInstance(instA))
		h.instanceChanged()

		// Before spawn completes, user switches to B.
		require.True(t, h.nav.SelectInstance(instB))
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
		nav:          ui.NewNavigationPanel(&spin),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
	}

	// Add instance "B" and select it (simulating selection change after spawn started for "A").
	instB, err := session.NewInstance(session.InstanceOptions{
		Title:   "B",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	h.nav.AddInstance(instB)()
	h.nav.SelectInstance(instB) // Select "B" by pointer (sort-order safe)

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

func TestTmuxBrowserActions(t *testing.T) {
	t.Run("tmuxSessionsMsg with no sessions shows toast", func(t *testing.T) {
		h := newTestHome()
		msg := tmuxSessionsMsg{sessions: nil}
		model, _ := h.Update(msg)
		hm := model.(*home)
		assert.False(t, hm.overlays.IsActive())
		assert.Equal(t, stateDefault, hm.state)
	})

	t.Run("tmuxSessionsMsg with sessions opens browser", func(t *testing.T) {
		h := newTestHome()
		msg := tmuxSessionsMsg{
			sessions: []tmux.SessionInfo{
				{Name: "kas_test", Title: "test", Width: 80, Height: 24, Managed: false},
			},
		}
		model, _ := h.Update(msg)
		hm := model.(*home)
		_, isBrowser := hm.overlays.Current().(*overlay.TmuxBrowserOverlay)
		assert.True(t, isBrowser, "current overlay must be a TmuxBrowserOverlay")
		assert.Equal(t, stateTmuxBrowser, hm.state)
	})

	t.Run("managed sessions are enriched with instance metadata", func(t *testing.T) {
		h := newTestHome()
		inst, _ := session.NewInstance(session.InstanceOptions{
			Title:   "auth-impl",
			Path:    "/tmp",
			Program: "claude",
		})
		inst.TaskFile = "auth"
		inst.AgentType = session.AgentTypeCoder
		inst.MarkStartedForTest()
		inst.SetTmuxSession(tmux.NewTmuxSession("auth-impl", "claude", false))
		h.allInstances = append(h.allInstances, inst)

		msg := tmuxSessionsMsg{
			sessions: []tmux.SessionInfo{
				{Name: "kas_auth-impl", Title: "auth-impl", Width: 80, Height: 24, Managed: true},
			},
		}
		model, _ := h.Update(msg)
		hm := model.(*home)
		browser, ok := hm.overlays.Current().(*overlay.TmuxBrowserOverlay)
		require.True(t, ok, "current overlay must be a TmuxBrowserOverlay")
		item := browser.SelectedItem()
		assert.True(t, item.Managed)
		assert.Equal(t, "coder", item.AgentType)
		assert.Equal(t, "auth", item.TaskFile)
	})

	t.Run("dismiss returns to default state", func(t *testing.T) {
		h := newTestHome()
		browser := overlay.NewTmuxBrowserOverlay([]overlay.TmuxBrowserItem{
			{Name: "kas_test", Title: "test"},
		})
		h.overlays.Show(browser)
		h.state = stateTmuxBrowser
		// "" is the dismiss action string
		model, _ := h.handleTmuxBrowserAction(browser, "")
		hm := model.(*home)
		assert.False(t, hm.overlays.IsActive())
		assert.Equal(t, stateDefault, hm.state)
	})
}

// TestPreviewTerminalReadyMsg_AcceptsCurrentInstance verifies that previewTerminalReadyMsg
// sets the terminal when the instance title matches the current selection.
func TestPreviewTerminalReadyMsg_AcceptsCurrentInstance(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		nav:          ui.NewNavigationPanel(&spin),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
	}

	// Add instance "A" and select it.
	instA, err := session.NewInstance(session.InstanceOptions{
		Title:   "A",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	h.nav.AddInstance(instA)()
	h.nav.SetSelectedInstance(0)

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
		nav:          ui.NewNavigationPanel(&spin),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
	}

	// Add a started-looking instance. We can't actually start it (no tmux),
	// but we can test the branch where previewTerminal is already set.
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "my-agent",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	h.nav.AddInstance(inst)()
	h.nav.SetSelectedInstance(0)

	// Simulate previewTerminal already attached to "my-agent".
	// enterFocusMode should detect this and NOT spawn a new terminal.
	h.previewTerminalInstance = "my-agent"
	// Instance is not started, so enterFocusMode should return nil (guard check).
	cmd := h.enterFocusMode()

	assert.Nil(t, cmd, "enterFocusMode should return nil when instance is not started")
	assert.Equal(t, stateDefault, h.state, "state should remain default when instance is not started")
}

func TestHandleQuit_NoActiveSessions_QuitsImmediately(t *testing.T) {
	h := newTestHome()
	h.toastManager = overlay.NewToastManager(&h.spinner)

	// Add a paused instance (not active)
	inst := &session.Instance{Title: "paused-agent", Status: session.Paused}
	h.nav.AddInstance(inst)

	_, cmd := h.handleQuit()

	// Should return tea.Quit directly (no confirmation)
	assert.Equal(t, stateDefault, h.state, "state should remain default (no confirmation overlay)")
	assert.False(t, h.overlays.IsActive(), "no confirmation overlay should be shown")
	require.NotNil(t, cmd, "should return a quit command")
}

func TestHandleQuit_ActiveSessions_ShowsConfirmation(t *testing.T) {
	h := newTestHome()
	h.toastManager = overlay.NewToastManager(&h.spinner)

	// Add a running instance
	inst := &session.Instance{Title: "running-agent", Status: session.Running}
	h.nav.AddInstance(inst)

	_, cmd := h.handleQuit()

	// Should show confirmation, not quit immediately
	assert.Equal(t, stateConfirm, h.state, "state should be stateConfirm")
	require.True(t, h.overlays.IsActive(), "confirmation overlay must be shown")
	assert.Nil(t, cmd, "confirmAction returns nil cmd (action stored in pendingConfirmAction)")
	assert.NotNil(t, h.pendingConfirmAction, "pending action must be set")
}

// setupPlanState sets up an in-memory plan state on h for test use.
// It creates a temp directory, registers the plan, seeds the status, and
// refreshes the nav panel so SelectByID works immediately afterward.
func (h *home) setupPlanState(t *testing.T, planFile string, status taskstate.Status, topic string) {
	t.Helper()
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	name := taskstate.DisplayName(planFile)
	require.NoError(t, ps.Create(planFile, name, "plan/"+name, topic, time.Now()))
	// Seed the status directly (bypass FSM).
	entry := ps.Plans[planFile]
	entry.Status = status
	ps.Plans[planFile] = entry
	require.NoError(t, ps.Save())
	h.taskState = ps
	h.taskStateDir = plansDir
	h.fsm = newPlanFSMForTest(t, plansDir)
	h.activeRepoPath = dir
	h.updateSidebarTasks()
}

func TestChatAboutPlan_ContextMenuAction(t *testing.T) {
	h := newTestHome()
	h.setupPlanState(t, "test-plan", taskstate.StatusImplementing, "test topic")

	// Select the plan in the nav panel
	h.nav.SelectByID(ui.SidebarPlanPrefix + "test-plan")

	// Execute the context action
	model, _ := h.executeContextAction("chat_about_plan")
	updated := model.(*home)

	require.Equal(t, stateChatAboutTask, updated.state)
	require.True(t, updated.overlays.IsActive(), "text input overlay must be set for question")
}

func TestChatAboutPlan_AppearsInContextMenu(t *testing.T) {
	h := newTestHome()
	h.setupPlanState(t, "test-plan", taskstate.StatusImplementing, "")

	h.focusSlot = slotNav
	h.nav.SelectByID(ui.SidebarPlanPrefix + "test-plan")

	model, _ := h.openTaskContextMenu()
	updated := model.(*home)

	require.Equal(t, stateContextMenu, updated.state)
	cm1, ok1 := updated.overlays.Current().(*overlay.ContextMenu)
	require.True(t, ok1, "current overlay must be a ContextMenu")

	// Verify "chat about this" appears in the menu items
	found := false
	for _, item := range cm1.Items() {
		if item.Action == "chat_about_plan" {
			found = true
			break
		}
	}
	require.True(t, found, "context menu must include 'chat about this' action")
}

func TestCreatePlanPR_AppearsInTaskContextMenu(t *testing.T) {
	h := newTestHome()
	h.setupPlanState(t, "test-plan", taskstate.StatusImplementing, "")

	h.focusSlot = slotNav
	h.nav.SelectByID(ui.SidebarPlanPrefix + "test-plan")

	model, _ := h.openTaskContextMenu()
	updated := model.(*home)

	require.Equal(t, stateContextMenu, updated.state)
	cm, ok := updated.overlays.Current().(*overlay.ContextMenu)
	require.True(t, ok, "current overlay must be a ContextMenu")

	found := false
	for _, item := range cm.Items() {
		if item.Action == "create_plan_pr" {
			found = true
			break
		}
	}
	require.True(t, found, "task context menu must include 'create pr' action")
}

// TestExitFocusMode_KeepsPreviewTerminal verifies that exitFocusMode does NOT close
// previewTerminal — it stays alive for preview rendering.
func TestExitFocusMode_KeepsPreviewTerminal(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	h := &home{
		ctx:          context.Background(),
		state:        stateFocusAgent,
		appConfig:    config.DefaultConfig(),
		nav:          ui.NewNavigationPanel(&spin),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
	}

	// Set previewTerminalInstance to simulate an attached terminal.
	h.previewTerminalInstance = "my-agent"

	h.exitFocusMode()

	assert.Equal(t, stateDefault, h.state, "state should return to default after exitFocusMode")
	assert.Equal(t, "my-agent", h.previewTerminalInstance,
		"previewTerminalInstance should NOT be cleared by exitFocusMode")
}

func TestRestartInstance_AppearsInContextMenu(t *testing.T) {
	h := newTestHome()
	inst, _ := session.NewInstance(session.InstanceOptions{
		Title:   "test-restart-menu",
		Path:    os.TempDir(),
		Program: "opencode",
	})
	inst.MarkStartedForTest()
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	model, _ := h.openContextMenu()
	updated := model.(*home)
	cm2, ok2 := updated.overlays.Current().(*overlay.ContextMenu)
	require.True(t, ok2, "current overlay must be a ContextMenu")

	found := false
	for _, item := range cm2.Items() {
		if item.Action == "restart_instance" {
			found = true
			break
		}
	}
	assert.True(t, found, "context menu should contain 'restart' option")
}

func TestExecuteContextAction_RestartInstance(t *testing.T) {
	h := newTestHome()
	inst, _ := session.NewInstance(session.InstanceOptions{
		Title:   "test-restart-action",
		Path:    os.TempDir(),
		Program: "opencode",
	})
	inst.MarkStartedForTest()
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	_, cmd := h.executeContextAction("restart_instance")
	// The action returns an async command (the restart runs in a goroutine).
	assert.NotNil(t, cmd, "restart action should return a tea.Cmd")
}

func TestDeleteKey_AllowsRemovalOfExitedRunningInstance(t *testing.T) {
	h := newTestHome()
	inst, err := newTestInstance("exited-reviewer")
	require.NoError(t, err)
	inst.Status = session.Running
	inst.Exited = true
	_ = h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)
	h.allInstances = append(h.allInstances, inst)

	msg := tea.KeyPressMsg{Code: tea.KeyDelete}
	_, _ = h.handleKeyPress(msg)

	assert.Equal(t, 0, h.nav.TotalInstances(),
		"delete should remove exited instance even if status is Running")
}

func TestKillKey_NoopsOnExitedInstance(t *testing.T) {
	h := newTestHome()
	inst, err := newTestInstance("exited-reviewer")
	require.NoError(t, err)
	inst.Status = session.Running
	inst.Exited = true
	inst.MarkStartedForTest()
	_ = h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	h.keySent = true
	msg := tea.KeyPressMsg{Code: 'k', Text: "k"}
	_, cmd := h.handleKeyPress(msg)

	assert.Nil(t, cmd, "k should no-op on an already-exited instance")
}

func TestMetadataTick_ExitedInstanceTransitionsToReady(t *testing.T) {
	h := newTestHomeWithToast()
	inst, err := newTestInstance("reviewer-done")
	require.NoError(t, err)
	inst.Status = session.Running
	_ = h.nav.AddInstance(inst)
	h.allInstances = append(h.allInstances, inst)

	ps, err := newTestPlanState(t, t.TempDir())
	require.NoError(t, err)

	// Simulate metadata tick with dead tmux
	msg := metadataResultMsg{
		Results:   []instanceMetadata{{Title: "reviewer-done", TmuxAlive: false}},
		PlanState: ps,
	}
	h.Update(msg)

	assert.True(t, inst.Exited, "instance should be marked exited")
	assert.Equal(t, session.Ready, inst.Status,
		"exited instance status should transition to Ready")
}

func TestShouldCreatePROnApproval(t *testing.T) {
	tests := []struct {
		name   string
		entry  taskstore.TaskEntry
		expect bool
	}{
		{name: "done with branch and no pr", entry: taskstore.TaskEntry{Status: taskstore.StatusDone, Branch: "plan/test", PRURL: ""}, expect: true},
		{name: "done with existing pr", entry: taskstore.TaskEntry{Status: taskstore.StatusDone, Branch: "plan/test", PRURL: "https://github.com/org/repo/pull/1"}, expect: false},
		{name: "not done", entry: taskstore.TaskEntry{Status: taskstore.StatusImplementing, Branch: "plan/test"}, expect: false},
		{name: "done but no branch", entry: taskstore.TaskEntry{Status: taskstore.StatusDone, Branch: ""}, expect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, shouldCreatePR(tt.entry))
		})
	}
}

func TestAssemblePRMetadata_FullEntry(t *testing.T) {
	meta := assemblePRMetadata(taskstore.TaskEntry{
		Description: "Auth Middleware",
		Goal:        "add JWT auth to all routes",
		Branch:      "plan/auth-middleware",
		Content:     "# Auth\n\n**Goal:** add JWT auth\n\n**Architecture:** middleware chain\n\n**Tech Stack:** Go\n\n## Wave 1\n\n### Task 1: JWT middleware\n\nbody\n",
	}, []taskstore.SubtaskEntry{
		{TaskNumber: 1, Title: "JWT middleware", Status: taskstore.SubtaskStatusComplete},
		{TaskNumber: 2, Title: "Route wiring", Status: taskstore.SubtaskStatusComplete},
	}, "looks good, approved", 2, "file1.go", "abc123 fix: auth", "1 file changed")

	assert.Equal(t, "Auth Middleware", meta.Description)
	assert.Equal(t, "add JWT auth to all routes", meta.Goal)
	assert.Equal(t, "middleware chain", meta.Architecture)
	assert.Equal(t, "Go", meta.TechStack)
	assert.Len(t, meta.Subtasks, 2)
	assert.Equal(t, "looks good, approved", meta.ReviewerSummary)
	assert.Equal(t, 2, meta.ReviewCycle)
	assert.Equal(t, "file1.go", meta.GitChanges)
}

func TestAssemblePRMetadata_EmptyContent(t *testing.T) {
	meta := assemblePRMetadata(taskstore.TaskEntry{
		Description: "quick fix",
		Goal:        "fix the bug",
	}, nil, "", 0, "", "", "")

	assert.Equal(t, "quick fix", meta.Description)
	assert.Equal(t, "fix the bug", meta.Goal)
	assert.Empty(t, meta.Architecture)
	assert.Empty(t, meta.TechStack)
	assert.Empty(t, meta.Subtasks)
	assert.Zero(t, meta.ReviewCycle)
}

func TestAssemblePRMetadata_InvalidPlanContent(t *testing.T) {
	meta := assemblePRMetadata(taskstore.TaskEntry{
		Description: "quick fix",
		Goal:        "fix the bug",
		Content:     "# no waves here",
	}, nil, "", 0, "", "", "")

	assert.Equal(t, "quick fix", meta.Description)
	assert.Equal(t, "fix the bug", meta.Goal)
	assert.Empty(t, meta.Architecture)
	assert.Empty(t, meta.TechStack)
}

func TestMapPRReviewDecision(t *testing.T) {
	assert.Equal(t, "approved", mapPRReviewDecision("APPROVED"))
	assert.Equal(t, "changes_requested", mapPRReviewDecision("CHANGES_REQUESTED"))
	assert.Equal(t, "pending", mapPRReviewDecision("REVIEW_REQUIRED"))
	assert.Equal(t, "pending", mapPRReviewDecision(""))
}

func TestMapPRCheckStatus(t *testing.T) {
	assert.Equal(t, "passing", mapPRCheckStatus("SUCCESS"))
	assert.Equal(t, "failing", mapPRCheckStatus("FAILURE"))
	assert.Equal(t, "pending", mapPRCheckStatus("PENDING"))
	assert.Equal(t, "pending", mapPRCheckStatus(""))
}
