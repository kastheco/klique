package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditPaneToggle(t *testing.T) {
	h := newTestHome()
	// Audit pane should be visible by default
	require.NotNil(t, h.auditPane, "auditPane must be initialized in newTestHome")
	assert.True(t, h.auditPane.Visible())

	// Simulate 'L' keybind to toggle (keySent=true skips menu highlight animation)
	h.keySent = true
	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	updated := model.(*home)
	assert.False(t, updated.auditPane.Visible())

	// Toggle back
	updated.keySent = true
	model2, _ := updated.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	updated2 := model2.(*home)
	assert.True(t, updated2.auditPane.Visible())
}

func TestAuditPaneRefresh_EmptyWithNilLogger(t *testing.T) {
	h := newTestHome()
	// With nil auditLogger, refreshAuditPane should not panic
	assert.NotPanics(t, func() {
		h.refreshAuditPane()
	})
}
