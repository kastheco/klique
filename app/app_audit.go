package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
)

// enterAuditCursorMode activates cursor navigation in the audit pane.
// If the pane is not visible or has no events, it is a no-op.
func (m *home) enterAuditCursorMode() (tea.Model, tea.Cmd) {
	if m.auditPane == nil || !m.auditPane.Visible() {
		m.toastManager.Info("audit log is hidden — press L to show it")
		return m, m.toastTickCmd()
	}
	if len(m.auditPane.Events()) == 0 {
		m.toastManager.Info("no log events yet")
		return m, m.toastTickCmd()
	}
	m.auditPane.SetCursorActive(true)
	m.state = stateAuditCursor
	m.refreshAuditPane()
	return m, tea.RequestWindowSize
}

// exitAuditCursorMode deactivates cursor navigation and returns to stateDefault.
func (m *home) exitAuditCursorMode() (tea.Model, tea.Cmd) {
	if m.auditPane != nil {
		m.auditPane.SetCursorActive(false)
	}
	m.pendingLogEvent = nil
	m.state = stateDefault
	m.refreshAuditPane()
	return m, tea.RequestWindowSize
}

// handleAuditCursorKey handles keypresses while in stateAuditCursor.
// ↑/↓ navigate log lines, enter/space opens context menu, esc exits.
func (m *home) handleAuditCursorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.auditPane == nil {
		return m.exitAuditCursorMode()
	}

	switch msg.String() {
	case "esc", "q":
		return m.exitAuditCursorMode()

	case "up":
		m.auditPane.CursorUp()
		m.refreshAuditPane()
		return m, tea.RequestWindowSize

	case "down":
		m.auditPane.CursorDown()
		m.refreshAuditPane()
		return m, tea.RequestWindowSize

	case "enter", " ":
		return m.openAuditLogContextMenu()
	}

	return m, nil
}

// openAuditLogContextMenu opens a context menu for the currently selected log
// event. If the event has no applicable actions it shows a toast instead.
func (m *home) openAuditLogContextMenu() (tea.Model, tea.Cmd) {
	if m.auditPane == nil {
		return m.exitAuditCursorMode()
	}
	e, ok := m.auditPane.SelectedEvent()
	if !ok {
		return m, nil
	}
	items := ui.AuditLineActions(e)
	if len(items) == 0 {
		m.toastManager.Info("no actions available for this log event")
		return m, m.toastTickCmd()
	}

	// Store event so log action handlers can read plan/instance context.
	m.pendingLogEvent = &e

	// Exit cursor mode before showing the overlay so Esc in the menu returns
	// to stateDefault rather than re-entering cursor mode.
	m.auditPane.SetCursorActive(false)
	m.state = stateContextMenu

	// Position context menu near the bottom-left of the nav area (audit section).
	x := 0
	y := m.contentHeight - 6
	if y < 0 {
		y = 0
	}
	m.overlays.ShowPositioned(overlay.NewContextMenu(items), x, y, false)
	m.refreshAuditPane()
	return m, tea.RequestWindowSize
}

// handleAuditCursorContextMenuDismissed is called when the log context menu is
// dismissed without selecting an action. Clears pendingLogEvent.
func (m *home) handleAuditCursorContextMenuDismissed() {
	if m.pendingLogEvent != nil {
		m.pendingLogEvent = nil
	}
}

// auditPaneSummary returns a short human-readable summary string for toast/status use.
func auditEventSummary(e ui.AuditEventDisplay) string {
	if e.TaskFile != "" {
		return fmt.Sprintf("[%s] %s", e.TaskFile, e.Kind)
	}
	return e.Kind
}
