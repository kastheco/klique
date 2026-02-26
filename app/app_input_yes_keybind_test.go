package app

import (
	"os"
	"testing"

	"github.com/kastheco/kasmos/session"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleKeyPress_YesKeyQueuesPromptForPromptDetectedInstance(t *testing.T) {
	h := newTestHome()

	inst, err := session.NewInstance(session.InstanceOptions{Title: "t1", Path: os.TempDir(), Program: "opencode"})
	require.NoError(t, err)
	inst.MarkStartedForTest()
	inst.SetStatus(session.Running)
	inst.PromptDetected = true

	h.nav.AddInstance(inst)()
	h.nav.SetSelectedInstance(0)
	h.keySent = true

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	require.IsType(t, &home{}, model)
	assert.Nil(t, cmd)
	assert.Equal(t, "yes", inst.QueuedPrompt)
	assert.True(t, inst.AwaitingWork)
}

func TestHandleKeyPress_YesKeyIgnoredWhenInstanceIsNotPromptDetected(t *testing.T) {
	h := newTestHome()

	inst, err := session.NewInstance(session.InstanceOptions{Title: "t2", Path: os.TempDir(), Program: "opencode"})
	require.NoError(t, err)
	inst.MarkStartedForTest()
	inst.SetStatus(session.Running)

	h.nav.AddInstance(inst)()
	h.nav.SetSelectedInstance(0)
	h.keySent = true

	model, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	require.IsType(t, &home{}, model)
	assert.Nil(t, cmd)
	assert.Empty(t, inst.QueuedPrompt)
	assert.False(t, inst.AwaitingWork)
}
