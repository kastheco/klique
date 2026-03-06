package app

import (
	"os"
	"testing"

	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleKeyPress_ExclamationEntersFocusMode(t *testing.T) {
	h := newTestHome()

	inst, err := session.NewInstance(session.InstanceOptions{
		Title: "test-inst", Path: os.TempDir(), Program: "opencode",
	})
	require.NoError(t, err)
	inst.MarkStartedForTest()
	inst.SetStatus(session.Running)

	h.nav.AddInstance(inst)()
	h.nav.SetSelectedInstance(0)
	h.tabbedWindow.SetActiveTab(ui.InfoTab)
	h.previewTerminal = session.NewDummyTerminal()
	h.previewTerminalInstance = inst.Title
	h.keySent = true

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: '!', Text: "!"})
	updated := model.(*home)

	assert.Equal(t, stateFocusAgent, updated.state)
	assert.Equal(t, ui.PreviewTab, updated.tabbedWindow.GetActiveTab())
	assert.Nil(t, cmd)
}

func TestHandleKeyPress_ExclamationNoOpWithoutRunningInstance(t *testing.T) {
	h := newTestHome()
	h.keySent = true

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: '!', Text: "!"})
	updated := model.(*home)

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
