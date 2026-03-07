package session

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedTerminal_CapturesOsc52ClipboardReadRequests(t *testing.T) {
	term := NewDummyTerminal()
	defer term.Close()

	_, err := term.emu.Write([]byte(ansi.RequestPrimaryClipboard))
	require.NoError(t, err)

	selection, ok := term.PollClipboardRequest()
	require.True(t, ok)
	require.Equal(t, byte(ansi.PrimaryClipboard), selection)
}
