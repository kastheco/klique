package app

import (
	"testing"

	"github.com/kastheco/kasmos/ui"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestHandleKeyPress_ExclamationSwitchesToPreviewTab(t *testing.T) {
	h := newTestHome()
	h.tabbedWindow.SetActiveTab(ui.InfoTab)
	h.keySent = true

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: '!', Text: "!"})
	updated := model.(*home)

	assert.Equal(t, ui.PreviewTab, updated.tabbedWindow.GetActiveTab())
	assert.Equal(t, stateDefault, updated.state)
	assert.Nil(t, cmd)
}

func TestHandleKeyPress_PoundStillSwitchesToInfoTab(t *testing.T) {
	h := newTestHome()
	h.tabbedWindow.SetActiveTab(ui.PreviewTab)
	h.keySent = true

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: '#', Text: "#"})
	updated := model.(*home)

	assert.Equal(t, ui.InfoTab, updated.tabbedWindow.GetActiveTab())
	assert.Equal(t, stateDefault, updated.state)
	assert.Nil(t, cmd)
}
