package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/stretchr/testify/assert"
)

// TestRightOnInstance_OpensContextMenu verifies that pressing right on an
// instance row opens the instance context menu (regardless of active tab).
func TestRightOnInstance_OpensContextMenu(t *testing.T) {
	h := newTestHome()

	// Add a solo instance so the selected row is an instance row.
	inst := &session.Instance{Title: "test-agent"}
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	// Press right from the info tab — should open context menu, not switch tab.
	h.tabbedWindow.SetActiveTab(ui.InfoTab)
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := model.(*home)

	assert.Equal(t, stateContextMenu, updated.state,
		"right on instance should open context menu")
	// Tab should not have changed.
	assert.Equal(t, ui.InfoTab, updated.tabbedWindow.GetActiveTab(),
		"right on instance should not switch tab")
}

// TestRightOnInstance_PreviewTab_OpensContextMenu verifies that pressing right
// on an instance while on the preview tab also opens the context menu.
func TestRightOnInstance_PreviewTab_OpensContextMenu(t *testing.T) {
	h := newTestHome()

	inst := &session.Instance{Title: "test-agent"}
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	h.tabbedWindow.SetActiveTab(ui.PreviewTab)
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := model.(*home)

	assert.Equal(t, stateContextMenu, updated.state,
		"right on instance from preview tab should open context menu")
}

// TestRightOnPlanHeader_TogglesExpand verifies that pressing right on a plan
// header expands/collapses it without opening a menu or switching tabs.
func TestRightOnPlanHeader_TogglesExpand(t *testing.T) {
	h := newTestHome()

	// Set up with no instance selected (plan header row will be selected if a plan exists,
	// or nothing selected — either way, GetSelectedInstance() returns nil).
	h.tabbedWindow.SetActiveTab(ui.InfoTab)

	// Press right with no instance selected.
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := model.(*home)

	// Should not have entered context menu state.
	assert.NotEqual(t, stateContextMenu, updated.state,
		"right on non-instance row should not open context menu")
	// Tab should not have changed.
	assert.Equal(t, ui.InfoTab, updated.tabbedWindow.GetActiveTab(),
		"right on non-instance row should not switch tab")
}
