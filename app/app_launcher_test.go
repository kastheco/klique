package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpaceOpensCommandLauncher(t *testing.T) {
	h := newTestHome()
	h.state = stateDefault

	// Space key with no expandable nav item selected should open the launcher.
	// handleKeyPress requires keySent=false to not trigger menu highlighting.
	h.keySent = true
	result, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m := result.(*home)
	assert.Equal(t, stateLauncher, m.state)
	assert.True(t, m.overlays.IsActive())
	_, ok := m.overlays.Current().(*overlay.CommandLauncherOverlay)
	require.True(t, ok, "expected CommandLauncherOverlay")
}

func TestQuestionMarkOpensKeybindBrowser(t *testing.T) {
	h := newTestHome()
	h.state = stateDefault

	h.keySent = true
	result, _ := h.handleKeyPress(tea.KeyPressMsg{Code: '?', Text: "?"})
	m := result.(*home)
	assert.Equal(t, stateKeybindBrowser, m.state)
	assert.True(t, m.overlays.IsActive())
	_, ok := m.overlays.Current().(*overlay.CommandLauncherOverlay)
	require.True(t, ok, "expected CommandLauncherOverlay for keybind browser")
}

func TestLauncherEscReturnToDefault(t *testing.T) {
	h := newTestHome()
	h.state = stateDefault

	// Open the launcher
	h.keySent = true
	result, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m := result.(*home)
	require.Equal(t, stateLauncher, m.state)

	// Press Esc to dismiss
	result, _ = m.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = result.(*home)
	assert.Equal(t, stateDefault, m.state)
	assert.False(t, m.overlays.IsActive())
}

func TestLauncherViewKeybindsOpensKeybindBrowser(t *testing.T) {
	h := newTestHome()
	h.state = stateDefault

	// Open the launcher
	h.keySent = true
	result, _ := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m := result.(*home)
	require.Equal(t, stateLauncher, m.state)

	// Select "view keybinds" (first item)
	result, _ = m.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(*home)
	assert.Equal(t, stateKeybindBrowser, m.state)
	assert.True(t, m.overlays.IsActive())
}

func TestKeybindBrowserEscReturnToDefault(t *testing.T) {
	h := newTestHome()
	h.state = stateDefault

	h.keySent = true
	result, _ := h.handleKeyPress(tea.KeyPressMsg{Code: '?', Text: "?"})
	m := result.(*home)
	require.Equal(t, stateKeybindBrowser, m.state)

	result, _ = m.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = result.(*home)
	assert.Equal(t, stateDefault, m.state)
	assert.False(t, m.overlays.IsActive())
}
