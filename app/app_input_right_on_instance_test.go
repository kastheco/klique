package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRightOnInstance_InfoTab_SwitchesToAgentTab verifies that pressing right
// on an instance row while the info tab is active switches to the agent tab.
func TestRightOnInstance_InfoTab_SwitchesToAgentTab(t *testing.T) {
	h := newTestHome()

	// Add a solo instance so the selected row is an instance row.
	inst := &session.Instance{Title: "test-agent"}
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	// Start on the info tab.
	h.tabbedWindow.SetActiveTab(ui.InfoTab)
	require.Equal(t, ui.InfoTab, h.tabbedWindow.GetActiveTab())

	// Press right.
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := model.(*home)

	assert.Equal(t, ui.PreviewTab, updated.tabbedWindow.GetActiveTab(),
		"right on instance while in info tab should switch to agent tab")
}

// TestRightOnInstance_AgentTab_NoOp verifies that pressing right on an instance
// row while already on the agent tab does not change the active tab.
func TestRightOnInstance_AgentTab_NoOp(t *testing.T) {
	h := newTestHome()

	inst := &session.Instance{Title: "test-agent"}
	h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	// Start on the agent (preview) tab.
	h.tabbedWindow.SetActiveTab(ui.PreviewTab)
	require.Equal(t, ui.PreviewTab, h.tabbedWindow.GetActiveTab())

	// Press right.
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := model.(*home)

	assert.Equal(t, ui.PreviewTab, updated.tabbedWindow.GetActiveTab(),
		"right on instance while already on agent tab should not change tab")
}

// TestRightOnPlanHeader_InfoTab_ExpandsNotSwitchesTab verifies that pressing
// right on a plan header (not an instance) does not switch tabs.
func TestRightOnPlanHeader_InfoTab_ExpandsNotSwitchesTab(t *testing.T) {
	h := newTestHome()

	// Set up a plan header row (no instance selected).
	h.tabbedWindow.SetActiveTab(ui.InfoTab)

	// Press right with no instance selected (plan header or empty).
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := model.(*home)

	// Tab should remain on info tab — no instance means no tab switch.
	assert.Equal(t, ui.InfoTab, updated.tabbedWindow.GetActiveTab(),
		"right on non-instance row should not switch to agent tab")
}
