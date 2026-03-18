package app

import (
	"os"
	"testing"

	"github.com/kastheco/kasmos/session"

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
	h.previewTerminal = session.NewDummyTerminal()
	h.previewTerminalInstance = inst.Title
	h.keySent = true

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: '!', Text: "!"})
	updated := model.(*home)

	assert.Equal(t, stateFocusAgent, updated.state)
	// In the new model, ! enters focus mode without changing the instance tab index.
	assert.Equal(t, 0, updated.tabbedWindow.GetActiveTab())
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

func TestHandleKeyPress_PoundTogglesInfoHeader(t *testing.T) {
	h := newTestHome()
	// showInfo starts as true (from NewTabbedWindow).
	wasShowing := h.tabbedWindow.IsShowingInfo()
	h.keySent = true

	model, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: '#', Text: "#"})
	updated := model.(*home)

	// # toggles the compact info header, not the instance tab index.
	assert.Equal(t, !wasShowing, updated.tabbedWindow.IsShowingInfo())
	assert.Equal(t, stateDefault, updated.state)
	assert.Nil(t, cmd)
}
