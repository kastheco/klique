package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/session"
	"github.com/stretchr/testify/assert"
)

// TestRightOnInstance_OpensContextMenu verifies that pressing right on an
// instance row opens the instance context menu.
func TestRightOnInstance_OpensContextMenu(t *testing.T) {
	h := newTestHome()

	// Add a solo instance so the selected row is an instance row.
	inst := &session.Instance{Title: "test-agent"}
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := model.(*home)

	assert.Equal(t, stateContextMenu, updated.state,
		"right on instance should open context menu")
}

// TestRightOnInstance_PreviewTab_OpensContextMenu verifies that pressing right
// on an instance also opens the context menu.
func TestRightOnInstance_PreviewTab_OpensContextMenu(t *testing.T) {
	h := newTestHome()

	inst := &session.Instance{Title: "test-agent"}
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := model.(*home)

	assert.Equal(t, stateContextMenu, updated.state,
		"right on instance should open context menu")
}

// TestRightOnPlanHeader_TogglesExpand verifies that pressing right on a plan
// header expands/collapses it without opening a menu.
func TestRightOnPlanHeader_TogglesExpand(t *testing.T) {
	h := newTestHome()

	// Press right with no instance selected.
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := model.(*home)

	// Should not have entered context menu state.
	assert.NotEqual(t, stateContextMenu, updated.state,
		"right on non-instance row should not open context menu")
}
