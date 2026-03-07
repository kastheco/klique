package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuditLineActions_WaveFailed verifies that a wave_failed event with a plan
// file produces "send to fixer agent" and "retry wave" actions.
func TestAuditLineActions_WaveFailed(t *testing.T) {
	e := ui.AuditEventDisplay{
		Kind:     "wave_failed",
		Message:  "wave 2 failed: task 3 exited non-zero",
		TaskFile: "auth-refactor.md",
	}
	items := ui.AuditLineActions(e)
	require.NotEmpty(t, items, "wave_failed with TaskFile should produce actions")
	actions := make(map[string]bool)
	for _, item := range items {
		actions[item.Action] = true
	}
	assert.True(t, actions["log_send_to_fixer"], "should include send-to-fixer action")
	assert.True(t, actions["log_retry_wave"], "should include retry-wave action")
}

// TestAuditLineActions_WaveFailedNoTaskFile ensures no actions when plan is unknown.
func TestAuditLineActions_WaveFailedNoTaskFile(t *testing.T) {
	e := ui.AuditEventDisplay{
		Kind:    "wave_failed",
		Message: "wave 1 failed",
		// no TaskFile
	}
	items := ui.AuditLineActions(e)
	assert.Empty(t, items, "wave_failed without TaskFile should produce no actions")
}

// TestAuditLineActions_MergeConflict verifies that a message containing
// "merge conflict" adds a fixer action regardless of event kind.
func TestAuditLineActions_MergeConflict(t *testing.T) {
	e := ui.AuditEventDisplay{
		Kind:     "error",
		Message:  "git: merge conflict in main.go",
		TaskFile: "feature.md",
	}
	items := ui.AuditLineActions(e)
	require.NotEmpty(t, items, "merge conflict should produce actions")
	assert.Equal(t, "log_send_to_fixer", items[0].Action, "first action should be send-to-fixer")
}

// TestAuditLineActions_AgentKilled verifies that a killed agent event with a
// title produces a restart action.
func TestAuditLineActions_AgentKilled(t *testing.T) {
	e := ui.AuditEventDisplay{
		Kind:          "agent_killed",
		Message:       "agent stopped",
		InstanceTitle: "auth-coder-1",
	}
	items := ui.AuditLineActions(e)
	require.NotEmpty(t, items, "agent_killed with InstanceTitle should produce actions")
	assert.Equal(t, "log_restart_agent", items[0].Action)
}

// TestAuditLineActions_AgentKilledNoTitle ensures no actions when title is unknown.
func TestAuditLineActions_AgentKilledNoTitle(t *testing.T) {
	e := ui.AuditEventDisplay{
		Kind:    "agent_killed",
		Message: "agent stopped",
	}
	items := ui.AuditLineActions(e)
	assert.Empty(t, items)
}

// TestAuditLineActions_ErrorWithPlan verifies fixer action on generic error.
func TestAuditLineActions_ErrorWithPlan(t *testing.T) {
	e := ui.AuditEventDisplay{
		Kind:     "error",
		Message:  "unexpected exit",
		TaskFile: "login.md",
	}
	items := ui.AuditLineActions(e)
	require.NotEmpty(t, items)
	assert.Equal(t, "log_send_to_fixer", items[0].Action)
}

// TestAuditLineActions_InertEvent verifies that an event with no applicable
// actions (e.g. plan_created) returns an empty slice.
func TestAuditLineActions_InertEvent(t *testing.T) {
	e := ui.AuditEventDisplay{
		Kind:     "plan_created",
		Message:  "new plan created",
		TaskFile: "some.md",
	}
	items := ui.AuditLineActions(e)
	assert.Empty(t, items, "plan_created has no log actions")
}

// TestAuditPane_CursorCycling verifies that cursor navigates through events.
func TestAuditPane_CursorCycling(t *testing.T) {
	p := ui.NewAuditPane()
	events := []ui.AuditEventDisplay{
		{Kind: "wave_failed", Message: "wave 1 failed", TaskFile: "a.md", Time: "10:00"},
		{Kind: "plan_created", Message: "plan made", Time: "10:01"},
		{Kind: "agent_killed", Message: "killed", InstanceTitle: "inst-1", Time: "10:02"},
	}
	// events[0] = newest (index 0), events[2] = oldest (index 2) in the audit pane
	// (Query returns newest-first, so index 0 is newest)
	p.SetEvents(events)
	p.SetCursorActive(true)

	// Cursor should start at first actionable event (index 0, wave_failed).
	e, ok := p.SelectedEvent()
	require.True(t, ok)
	assert.Equal(t, "wave_failed", e.Kind, "cursor should start at newest actionable event")

	// CursorUp moves toward older events (index increases).
	p.CursorUp()
	e, ok = p.SelectedEvent()
	require.True(t, ok)
	assert.Equal(t, "plan_created", e.Kind, "CursorUp should move to next older event")

	p.CursorUp()
	e, ok = p.SelectedEvent()
	require.True(t, ok)
	assert.Equal(t, "agent_killed", e.Kind, "CursorUp should reach oldest event")

	// Cannot go past the oldest event.
	p.CursorUp()
	e, ok = p.SelectedEvent()
	require.True(t, ok)
	assert.Equal(t, "agent_killed", e.Kind, "CursorUp at oldest should stay")

	// CursorDown moves toward newer events (index decreases).
	p.CursorDown()
	e, ok = p.SelectedEvent()
	require.True(t, ok)
	assert.Equal(t, "plan_created", e.Kind, "CursorDown should move to newer event")
}

// TestAuditPane_CursorDeactivate verifies cursor is cleared on deactivation.
func TestAuditPane_CursorDeactivate(t *testing.T) {
	p := ui.NewAuditPane()
	p.SetEvents([]ui.AuditEventDisplay{
		{Kind: "wave_failed", Message: "wave 1 failed", TaskFile: "a.md", Time: "10:00"},
	})
	p.SetCursorActive(true)

	_, ok := p.SelectedEvent()
	require.True(t, ok, "cursor should be active")

	p.SetCursorActive(false)
	_, ok = p.SelectedEvent()
	assert.False(t, ok, "after deactivation cursor should be gone")
	assert.False(t, p.CursorActive())
}

// TestEnterAuditCursorMode_SetsState verifies that pressing A activates the audit cursor.
func TestEnterAuditCursorMode_SetsState(t *testing.T) {
	h := newTestHome()
	h.auditPane.SetEvents([]ui.AuditEventDisplay{
		{Kind: "wave_failed", Message: "wave 1 failed", TaskFile: "a.md", Time: "10:00"},
	})
	h.auditPane.SetSize(40, 10)

	// keySent=true bypasses the menu-highlighting double-dispatch (same pattern as TestAuditPaneToggle).
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: 'A', Text: "A"})
	updated := model.(*home)
	assert.Equal(t, stateAuditCursor, updated.state)
	assert.True(t, updated.auditPane.CursorActive())
}

// TestHandleAuditCursorKey_EscExits verifies Esc exits audit cursor mode.
func TestHandleAuditCursorKey_EscExits(t *testing.T) {
	h := newTestHome()
	h.auditPane.SetEvents([]ui.AuditEventDisplay{
		{Kind: "wave_failed", Message: "wave 1 failed", TaskFile: "a.md", Time: "10:00"},
	})
	h.state = stateAuditCursor
	h.auditPane.SetCursorActive(true)

	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"})
	updated := model.(*home)
	assert.Equal(t, stateDefault, updated.state)
	assert.False(t, updated.auditPane.CursorActive())
}

// TestHandleAuditCursorKey_EnterOpensMenu verifies Enter on an actionable event
// transitions to stateContextMenu.
func TestHandleAuditCursorKey_EnterOpensMenu(t *testing.T) {
	h := newTestHome()
	h.auditPane.SetEvents([]ui.AuditEventDisplay{
		{Kind: "wave_failed", Message: "wave 1 failed", TaskFile: "a.md", Time: "10:00"},
	})
	h.auditPane.SetSize(40, 10)
	h.state = stateAuditCursor
	h.auditPane.SetCursorActive(true)

	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	updated := model.(*home)
	assert.Equal(t, stateContextMenu, updated.state, "enter on actionable event should open context menu")
	assert.True(t, updated.overlays.IsActive(), "overlay should be active")
	require.NotNil(t, updated.pendingLogEvent, "pending log event should be set")
	assert.Equal(t, "wave_failed", updated.pendingLogEvent.Kind)
}

// TestHandleAuditCursorKey_EnterNoActions shows toast when no actions available.
func TestHandleAuditCursorKey_EnterNoActions(t *testing.T) {
	h := newTestHome()
	h.auditPane.SetEvents([]ui.AuditEventDisplay{
		{Kind: "plan_created", Message: "new plan", TaskFile: "a.md", Time: "10:00"},
	})
	h.auditPane.SetSize(40, 10)
	h.state = stateAuditCursor
	h.auditPane.SetCursorActive(true)

	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	updated := model.(*home)
	// Should NOT have opened a context menu — still in audit cursor mode (or toast shown)
	assert.NotEqual(t, stateContextMenu, updated.state, "inert event should not open context menu")
	assert.Nil(t, updated.pendingLogEvent)
}
