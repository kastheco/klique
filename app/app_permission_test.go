package app

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestHomeWithCache returns a home with a real permissionCache backed by a temp dir.
func newTestHomeWithCache(t *testing.T) *home {
	t.Helper()
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	return &home{
		ctx:               context.Background(),
		state:             stateDefault,
		appConfig:         config.DefaultConfig(),
		nav:               ui.NewNavigationPanel(&spin),
		menu:              ui.NewMenu(),
		tabbedWindow:      ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		toastManager:      overlay.NewToastManager(&spin),
		activeRepoPath:    t.TempDir(),
		program:           "opencode",
		permissionCache:   config.NewPermissionCache(t.TempDir()),
		permissionHandled: make(map[*session.Instance]string),
	}
}

// collectAutoApproveMsgs runs a tea.Cmd recursively and collects all permissionAutoApproveMsg values.
func collectAutoApproveMsgs(cmd tea.Cmd) []permissionAutoApproveMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	var results []permissionAutoApproveMsg
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			results = append(results, collectAutoApproveMsgs(sub)...)
		}
	} else if pam, ok := msg.(permissionAutoApproveMsg); ok {
		results = append(results, pam)
	}
	return results
}

// --- Update() cycle integration tests ---

// TestUpdate_PermissionPromptDetection_ShowsOverlay exercises the real metadata-tick
// detection path through Update(), rather than manually setting m.state.
func TestUpdate_PermissionPromptDetection_ShowsOverlay(t *testing.T) {
	m := newTestHomeWithCache(t)
	inst := &session.Instance{Title: "test-agent", Program: "opencode"}
	inst.MarkStartedForTest()
	m.nav.AddInstance(inst)()

	pp := &session.PermissionPrompt{Pattern: "/opt/*", Description: "Access /opt"}
	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "test-agent", PermissionPrompt: pp},
		},
	}

	_, _ = m.Update(msg)

	assert.Equal(t, statePermission, m.state)
	require.NotNil(t, m.permissionOverlay)
	assert.NotNil(t, m.pendingPermissionInstance)
}

// TestUpdate_PermissionAutoApprove_FiresOnCachedPattern verifies that a cached pattern
// fires permissionAutoApproveMsg (not the modal) on the first tick.
func TestUpdate_PermissionAutoApprove_FiresOnCachedPattern(t *testing.T) {
	m := newTestHomeWithCache(t)
	m.permissionCache.Remember("/opt/*")

	inst := &session.Instance{Title: "test-agent", Program: "opencode"}
	inst.MarkStartedForTest()
	m.nav.AddInstance(inst)()

	pp := &session.PermissionPrompt{Pattern: "/opt/*", Description: "Access /opt"}
	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "test-agent", PermissionPrompt: pp},
		},
	}

	_, cmd := m.Update(msg)
	approvals := collectAutoApproveMsgs(cmd)

	assert.Len(t, approvals, 1, "first tick should queue exactly one auto-approve")
	assert.Equal(t, stateDefault, m.state, "auto-approve should not change state")
}

// TestUpdate_PermissionAutoApprove_DeduplicatesOnMultipleTicks is the critical regression test:
// a second metadata tick with the same prompt (before opencode clears it) must NOT fire
// a second auto-approve, which would corrupt opencode's input state.
func TestUpdate_PermissionAutoApprove_DeduplicatesOnMultipleTicks(t *testing.T) {
	m := newTestHomeWithCache(t)
	m.permissionCache.Remember("/opt/*")

	inst := &session.Instance{Title: "test-agent", Program: "opencode"}
	inst.MarkStartedForTest()
	m.nav.AddInstance(inst)()

	pp := &session.PermissionPrompt{Pattern: "/opt/*", Description: "Access /opt"}
	msg := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "test-agent", PermissionPrompt: pp},
		},
	}

	// First tick — should fire once.
	_, cmd1 := m.Update(msg)
	approvals1 := collectAutoApproveMsgs(cmd1)
	assert.Len(t, approvals1, 1, "first tick should queue one auto-approve")

	// Second tick — pane still shows the prompt (opencode hasn't processed the keys yet).
	// Must NOT fire again.
	_, cmd2 := m.Update(msg)
	approvals2 := collectAutoApproveMsgs(cmd2)
	assert.Len(t, approvals2, 0, "second tick must not queue a duplicate auto-approve")
}

// TestUpdate_PermissionAutoApprove_ClearsGuardWhenPromptGone verifies that once the
// permission prompt disappears from the pane the deduplication guard is cleared,
// allowing a future prompt to trigger auto-approve again.
func TestUpdate_PermissionAutoApprove_ClearsGuardWhenPromptGone(t *testing.T) {
	m := newTestHomeWithCache(t)
	m.permissionCache.Remember("/opt/*")

	inst := &session.Instance{Title: "test-agent", Program: "opencode"}
	inst.MarkStartedForTest()
	m.nav.AddInstance(inst)()

	pp := &session.PermissionPrompt{Pattern: "/opt/*", Description: "Access /opt"}
	withPrompt := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "test-agent", PermissionPrompt: pp},
		},
	}
	noPrompt := metadataResultMsg{
		Results: []instanceMetadata{
			{Title: "test-agent", PermissionPrompt: nil},
		},
	}

	// First tick — fires, guard set.
	_, _ = m.Update(withPrompt)

	// Prompt clears — guard should be removed.
	_, _ = m.Update(noPrompt)

	// New prompt (e.g. a second permission request later) — should fire again.
	_, cmd3 := m.Update(withPrompt)
	approvals := collectAutoApproveMsgs(cmd3)
	assert.Len(t, approvals, 1, "should fire again after guard is cleared")
}

// TestHandleKeyPress_PermissionEnter_SendsResponse verifies that pressing Enter while
// in statePermission triggers a SendPermissionResponse cmd and returns to stateDefault.
func TestHandleKeyPress_PermissionEnter_SendsResponse(t *testing.T) {
	m := newTestHomeWithCache(t)
	inst := &session.Instance{Title: "test-agent", Program: "opencode"}
	inst.MarkStartedForTest()
	m.nav.AddInstance(inst)()

	// Set up statePermission (overlay open, instance pending)
	m.state = statePermission
	m.permissionOverlay = overlay.NewPermissionOverlay(inst.Title, "Access /opt", "/opt/*")
	m.pendingPermissionInstance = inst

	// Enter confirms with current selection (default: allow always)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Equal(t, stateDefault, m.state, "enter should return to stateDefault")
	assert.NotNil(t, cmd, "enter should return a permission response cmd")
}

// TestHandleKeyPress_PermissionEsc_DismissesWithoutSending verifies that Esc closes
// the modal without sending any keys to the tmux pane.
func TestHandleKeyPress_PermissionEsc_DismissesWithoutSending(t *testing.T) {
	m := newTestHomeWithCache(t)
	inst := &session.Instance{Title: "test-agent", Program: "opencode"}
	inst.MarkStartedForTest()
	m.nav.AddInstance(inst)()

	m.state = statePermission
	m.permissionOverlay = overlay.NewPermissionOverlay(inst.Title, "Access /opt", "/opt/*")
	m.pendingPermissionInstance = inst

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	assert.Equal(t, stateDefault, m.state, "esc should return to stateDefault")
	assert.Nil(t, m.permissionOverlay, "overlay should be cleared")
	assert.Nil(t, m.pendingPermissionInstance, "pending instance should be cleared")
	// Esc must not send any permission response (nil cmd is fine; no auto-approve)
	approvals := collectAutoApproveMsgs(cmd)
	assert.Len(t, approvals, 0, "esc should not trigger any auto-approve")
}

// TestPermissionOverlay_PatternExposed verifies the overlay exposes its pattern
// so app_input.go can read it on confirm without re-parsing CachedContent.
func TestPermissionOverlay_PatternExposed(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")
	assert.Equal(t, "/opt/*", po.Pattern())
}

// --- Legacy unit tests (kept for overlay component coverage) ---

func TestPermissionDetection_ShowsOverlayForOpenCode(t *testing.T) {
	m := newTestHomeWithCache(t)
	inst := &session.Instance{
		Title:   "test-agent",
		Program: "opencode",
	}
	inst.MarkStartedForTest()
	m.nav.AddInstance(inst)()
	m.nav.SetSelectedInstance(0)

	// Simulate metadata tick with permission prompt detected
	inst.CachedContent = "△ Permission required\n  ← Access external directory /opt\n\nPatterns\n\n- /opt/*\n\n Allow once   Allow always   Reject\n"
	inst.CachedContentSet = true

	pp := session.ParsePermissionPrompt(inst.CachedContent, inst.Program)
	assert.NotNil(t, pp)

	// Simulate the detection path
	m.permissionOverlay = overlay.NewPermissionOverlay(inst.Title, pp.Description, pp.Pattern)
	m.pendingPermissionInstance = inst
	m.state = statePermission

	assert.Equal(t, statePermission, m.state)
	assert.NotNil(t, m.permissionOverlay)
}

func TestPermissionOverlay_ArrowKeysNavigate(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")

	// Default is "allow once" (index 0 — matches opencode's default cursor position)
	assert.Equal(t, overlay.PermissionAllowOnce, po.Choice())

	// Right → "allow always"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, overlay.PermissionAllowAlways, po.Choice())

	// Right → "reject"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, overlay.PermissionReject, po.Choice())

	// Right at end → stays on "reject"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, overlay.PermissionReject, po.Choice())

	// Left → back to "allow always"
	po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, overlay.PermissionAllowAlways, po.Choice())
}

func TestPermissionOverlay_EnterConfirms(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")
	closed := po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, closed)
	assert.True(t, po.IsConfirmed())
	assert.Equal(t, overlay.PermissionAllowOnce, po.Choice()) // default matches opencode
}

func TestPermissionOverlay_EscDismisses(t *testing.T) {
	po := overlay.NewPermissionOverlay("test", "Access /opt", "/opt/*")
	closed := po.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, closed)
	assert.False(t, po.IsConfirmed())
}

func TestPermissionCache_AutoApprovesCachedPattern(t *testing.T) {
	m := newTestHomeWithCache(t)
	m.permissionCache.Remember("/opt/*")
	assert.True(t, m.permissionCache.IsAllowedAlways("/opt/*"))
}
