package app

import (
	"testing"

	"github.com/kastheco/kasmos/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenContextMenu_InstanceIncludesRestart verifies that the instance context
// menu contains the "restart" option when an instance is selected.
func TestOpenContextMenu_InstanceIncludesRestart(t *testing.T) {
	h := newTestHome()
	h.focusSlot = slotNav

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "test-agent",
		Path:    t.TempDir(),
		Program: "opencode",
	})
	require.NoError(t, err)
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	model, _ := h.openContextMenu()
	updated := model.(*home)

	require.Equal(t, stateContextMenu, updated.state, "openContextMenu should set stateContextMenu")
	require.NotNil(t, updated.contextMenu, "context menu should not be nil")

	items := updated.contextMenu.Items()
	var hasRestart bool
	for _, item := range items {
		if item.Action == "restart_instance" {
			hasRestart = true
		}
	}
	assert.True(t, hasRestart, "context menu should include restart_instance action")
}

// TestExecuteContextAction_RestartInstance_ShowsConfirmation verifies that the
// restart_instance action shows a confirmation dialog before restarting.
func TestExecuteContextAction_RestartInstance_ShowsConfirmation(t *testing.T) {
	h := newTestHome()

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "test-agent",
		Path:    t.TempDir(),
		Program: "opencode",
	})
	require.NoError(t, err)
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	model, _ := h.executeContextAction("restart_instance")
	updated := model.(*home)

	assert.Equal(t, stateConfirm, updated.state,
		"restart_instance should show a confirmation dialog")
	assert.NotNil(t, updated.confirmationOverlay,
		"confirmation overlay should be set for restart_instance")
}

// TestExecuteContextAction_RestartInstance_NilSelected verifies graceful no-op
// when no instance is selected.
func TestExecuteContextAction_RestartInstance_NilSelected(t *testing.T) {
	h := newTestHome()

	model, cmd := h.executeContextAction("restart_instance")
	updated := model.(*home)

	assert.Equal(t, stateDefault, updated.state,
		"restart_instance with nil selection should be a no-op")
	assert.Nil(t, cmd)
}
