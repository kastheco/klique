package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestKeyToBytes_ForwardsAltRuneAsEscapeSequence(t *testing.T) {
	msg := tea.KeyPressMsg{Code: 'v', Text: "v", Mod: tea.ModAlt}

	got := keyToBytes(msg)

	require.Equal(t, []byte("\x1bv"), got)
}

func TestKeyToBytes_ForwardsPlainRuneWithoutEscapePrefix(t *testing.T) {
	msg := tea.KeyPressMsg{Code: 'v', Text: "v"}

	got := keyToBytes(msg)

	require.Equal(t, []byte("v"), got)
}
