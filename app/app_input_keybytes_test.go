package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestKeyToBytes_ForwardsAltRuneAsEscapeSequence(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true}

	got := keyToBytes(msg)

	require.Equal(t, []byte("\x1bv"), got)
}

func TestKeyToBytes_ForwardsPlainRuneWithoutEscapePrefix(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}

	got := keyToBytes(msg)

	require.Equal(t, []byte("v"), got)
}
