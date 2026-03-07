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

func TestKeyToBytes_ForwardsAltRuneFromCodeWhenTextEmpty(t *testing.T) {
	msg := tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt}

	got := keyToBytes(msg)

	require.Equal(t, []byte("\x1bv"), got)
}

func TestKeyToBytes_ForwardsPlainRuneWithoutEscapePrefix(t *testing.T) {
	msg := tea.KeyPressMsg{Code: 'v', Text: "v"}

	got := keyToBytes(msg)

	require.Equal(t, []byte("v"), got)
}

func TestKeyToBytes_EscapeKey(t *testing.T) {
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}

	got := keyToBytes(msg)

	require.Equal(t, []byte{0x1b}, got, "ESC must produce raw 0x1b byte")
}

func TestKeyToBytes_CtrlShiftV(t *testing.T) {
	// Ctrl+Shift+V: Code='V' (uppercase due to Shift), Mod has both Ctrl and Shift.
	// The Ctrl handler only matches lowercase a-z, so this falls through.
	// With Shift, the switch only handles Tab/arrows/Home/End.
	// msg.Text is empty for modifier-only combos, so it returns nil.
	msg := tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl | tea.ModShift}

	got := keyToBytes(msg)

	// Ctrl+Shift+V is a terminal paste shortcut — the terminal emulator
	// handles it and sends bracketed paste, not a key event. If bubbletea
	// does deliver it as a KeyPressMsg, we should forward Ctrl+V (0x16).
	// Currently returns Ctrl+V because Ctrl+letter matches before Shift check.
	require.Equal(t, []byte{0x16}, got, "Ctrl+Shift+V should produce Ctrl+V (0x16)")
}

func TestKeyToBytes_ShiftEnterUsesKittySequence(t *testing.T) {
	msg := tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}

	got := keyToBytes(msg)

	require.Equal(t, []byte("\x1b[13;2u"), got)
}
