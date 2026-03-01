package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/kasmos/config/auditlog"
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

// TestRefreshAuditPane_TimestampInLocalTime verifies that audit event timestamps
// are displayed in local time, not UTC. Timestamps are stored as UTC in the DB
// and must be converted to local time before formatting for display.
func TestRefreshAuditPane_TimestampInLocalTime(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	// Use a fixed UTC time: 20:00 UTC = 14:00 Central (UTC-6 in winter).
	// We emit with an explicit timestamp so we can predict the local display.
	utcTime := time.Date(2026, 2, 28, 20, 0, 0, 0, time.UTC)
	logger.Emit(auditlog.Event{
		Kind:      auditlog.EventPlanTransition,
		Project:   "test",
		Message:   "ready → implementing",
		Timestamp: utcTime,
	})

	h := newTestHome()
	h.auditLogger = logger
	h.planStoreProject = "test"
	h.refreshAuditPane()

	// The displayed time must match local time, not UTC.
	localTime := utcTime.Local()
	expectedTimeStr := localTime.Format("15:04")
	utcTimeStr := utcTime.Format("15:04")

	// Inspect the formatted events directly (no sized viewport needed).
	events := h.auditPane.Events()
	require.Len(t, events, 1, "expected one audit event in pane")
	assert.Equal(t, expectedTimeStr, events[0].Time,
		"audit timestamp must be displayed in local time (got %q, UTC would be %q)",
		events[0].Time, utcTimeStr)
}
