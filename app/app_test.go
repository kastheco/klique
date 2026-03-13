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
		toastManager:   overlay.NewToastManager(&spin),
		overlays:       overlay.NewManager(),
		activeRepoPath: os.TempDir(),
		program:        "opencode",
		daemonStatusChecker: func(string) daemonStatusMsg {
			return daemonStatusMsg{ready: true}
		},
		daemonRepoRegistrar: func(string) error { return nil },
	}
}

func TestShowDaemonRequiredDialog_RegistersRepoOnConfirm(t *testing.T) {
	registeredPath := ""
	h := newTestHome()
	h.activeRepoPath = filepath.Join(os.TempDir(), "kasmos-test-repo")
	h.daemonRepoRegistrar = func(path string) error {
		registeredPath = path
		return nil
	}

	h.showDaemonRequiredDialog(daemonStatusMsg{
		message:         "the kasmos daemon is running, but this repo is not registered.",
		canRegisterRepo: true,
	})
	require.NotNil(t, h.pendingConfirmAction)

	msg := h.pendingConfirmAction()
	registered, ok := msg.(daemonRepoRegisteredMsg)
	require.True(t, ok)
	assert.Equal(t, h.activeRepoPath, registered.path)
	assert.Equal(t, h.activeRepoPath, registeredPath)

	co, ok := h.overlays.Current().(*overlay.ConfirmationOverlay)
	require.True(t, ok)
	assert.Contains(t, co.View(), "to confirm")
	assert.Contains(t, co.View(), "register")
}

func TestShowDaemonRequiredDialog_DoesNotRegisterWhenUnavailable(t *testing.T) {
	h := newTestHome()
	h.showDaemonRequiredDialog(daemonStatusMsg{message: "start the daemon first"})
	assert.Nil(t, h.pendingConfirmAction)
	assert.Equal(t, stateConfirm, h.state)
	assert.True(t, h.overlays.IsActive())
	co, ok := h.overlays.Current().(*overlay.ConfirmationOverlay)
	require.True(t, ok)
	assert.Contains(t, co.View(), "start the daemon first")
}

func TestView_UsesCellMotionMouseMode(t *testing.T) {
	h := newTestHome()
	h.termHeight = 20
	h.contentHeight = 10
	h.nav.SetSize(24, 10)

	v := h.View()
	assert.Equal(t, tea.MouseModeCellMotion, v.MouseMode)
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
			ctx:       context.Background(),
			state:     stateDefault,
			appConfig: config.DefaultConfig(),
			nav:       ui.NewNavigationPanel(&spin),
			menu:      ui.NewMenu(),
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

	t.Run("s is no-op (sidebar focus shortcut removed)", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = false

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 's', Text: "s"})

		assert.False(t, homeModel.sidebarHidden)
	})

	t.Run("s does not show hidden sidebar", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = true

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 's', Text: "s"})

		assert.True(t, homeModel.sidebarHidden)
	})

	// --- Sidebar toggle (ctrl+s) ---

	t.Run("ctrl+s hides sidebar", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = false

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

		assert.True(t, homeModel.sidebarHidden)
	})

	t.Run("ctrl+s shows sidebar when hidden", func(t *testing.T) {
		h := newTestHome()
		h.sidebarHidden = true

		homeModel := handle(t, h, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

		assert.False(t, homeModel.sidebarHidden)
	})

	// --- Arrow key navigation ---

	t.Run("← is no-op", func(t *testing.T) {
		h := newTestHome()

		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyLeft})

		assert.NotNil(t, homeModel)
	})

	t.Run("→ toggles expand on selected sidebar item", func(t *testing.T) {
		h := newTestHome()
		// Without a plan header selected, ToggleSelectedExpand returns false,
		// so → is effectively a no-op.
		homeModel := handle(t, h, tea.KeyPressMsg{Code: tea.KeyRight})

		assert.NotNil(t, homeModel)
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

func TestHandleKeyPress_CtrlSpaceFocusesWorkspacePane(t *testing.T) {
	// Ctrl+Space issues an async tmux pane-focus command (the nav-only layout redesign).
	// When TMUX is unset (as in tests), OuterSessionName() returns "" and the
	// goroutine is a silent no-op — state stays stateDefault, cmd is non-nil.
	h := newTestHome()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "test-focus-toggle",
		Path:    os.TempDir(),
		Program: "opencode",
	})
	require.NoError(t, err)
	inst.MarkStartedForTest()
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)
	h.keySent = true

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl})
	updated := model.(*home)

	assert.Equal(t, stateDefault, updated.state)
	assert.NotNil(t, cmd)
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
