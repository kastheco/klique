package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kastheco/kasmos/session"
	"github.com/stretchr/testify/require"
)

func TestPreviewTick_BridgesEmbeddedClipboardRequest(t *testing.T) {
	h := newTestHome()
	h.previewTerminal = session.NewDummyTerminal()
	h.previewTerminal.EnqueueClipboardRequest(ansi.PrimaryClipboard)

	model, cmd := h.Update(previewTickMsg{})
	updated := model.(*home)

	require.True(t, updated.previewClipboardPending)
	require.Equal(t, byte(ansi.PrimaryClipboard), updated.previewClipboardTarget)
	require.NotNil(t, cmd)
}

func TestClipboardMsg_ForwardsResponseToEmbeddedTerminal(t *testing.T) {
	h := newTestHome()
	term := session.NewDummyTerminal()
	h.previewTerminal = term
	h.previewClipboardPending = true
	h.previewClipboardTarget = ansi.SystemClipboard

	model, cmd := h.Update(tea.ClipboardMsg{Content: "image-bytes"})
	updated := model.(*home)

	require.False(t, updated.previewClipboardPending)
	require.Zero(t, updated.previewClipboardTarget)
	require.Nil(t, cmd)

	sent := term.SentKeys()
	require.Len(t, sent, 1)
	require.Equal(t, []byte(ansi.SetSystemClipboard("image-bytes")), sent[0])
}
