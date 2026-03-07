package ui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/muesli/reflow/wordwrap"
)

// AuditEventDisplay is a pre-formatted event record for the audit pane.
type AuditEventDisplay struct {
	Time          string      // wall-clock time formatted as "HH:MM"
	Kind          string      // event kind string (e.g. "agent_spawned")
	Icon          string      // single-character icon glyph
	Message       string      // human-readable event description
	Color         color.Color // icon foreground colour
	Level         string      // "info", "warn", or "error"
	TaskFile      string      // plan filename if event is plan-scoped (may be empty)
	InstanceTitle string      // instance title if event is instance-scoped (may be empty)
	AgentType     string      // agent role (planner, coder, reviewer, fixer, …)
}

// AuditLineActions returns context-menu items available for the given log event.
// Only events that warrant user follow-up (failures, errors, conflicts) get items.
// Returns nil when no actions are applicable.
func AuditLineActions(e AuditEventDisplay) []overlay.ContextMenuItem {
	var items []overlay.ContextMenuItem

	hasPlan := e.TaskFile != ""
	hasInstance := e.InstanceTitle != ""
	msgLower := strings.ToLower(e.Message)
	isMergeConflict := strings.Contains(msgLower, "merge conflict") ||
		strings.Contains(msgLower, "conflict")

	switch e.Kind {
	case "wave_failed":
		if hasPlan {
			items = append(items, overlay.ContextMenuItem{Label: "send to fixer agent", Action: "log_send_to_fixer"})
			items = append(items, overlay.ContextMenuItem{Label: "retry wave", Action: "log_retry_wave"})
		}
	case "error", "fsm_error":
		if hasPlan {
			items = append(items, overlay.ContextMenuItem{Label: "send to fixer agent", Action: "log_send_to_fixer"})
		}
	case "agent_killed":
		if hasInstance {
			items = append(items, overlay.ContextMenuItem{Label: "restart agent", Action: "log_restart_agent"})
		}
	case "agent_finished":
		if hasPlan {
			items = append(items, overlay.ContextMenuItem{Label: "start review", Action: "log_start_review"})
		}
	case "wave_completed":
		if hasPlan {
			items = append(items, overlay.ContextMenuItem{Label: "start review", Action: "log_start_review"})
		}
	}

	// Merge-conflict events get a fixer shortcut regardless of kind.
	if isMergeConflict && hasPlan {
		// Avoid duplicate if already added above.
		alreadyHasFixer := false
		for _, item := range items {
			if item.Action == "log_send_to_fixer" {
				alreadyHasFixer = true
				break
			}
		}
		if !alreadyHasFixer {
			items = append([]overlay.ContextMenuItem{{Label: "send to fixer agent", Action: "log_send_to_fixer"}}, items...)
		}
	}

	return items
}

// AuditPane renders a chronological, scrollable activity feed.
type AuditPane struct {
	events       []AuditEventDisplay
	viewport     viewport.Model
	width        int
	height       int
	visible      bool
	bodyLines    int // rendered body line count (events + minute headers, excluding padding)
	selectedIdx  int // index into events (-1 = none selected)
	cursorActive bool
}

// NewAuditPane returns an AuditPane that is visible by default.
func NewAuditPane() *AuditPane {
	return &AuditPane{
		visible:     true,
		viewport:    viewport.New(),
		selectedIdx: -1,
	}
}

// SetSize stores the total pane dimensions and rebuilds content.
// One line is reserved for the header divider; the viewport gets the rest.
func (p *AuditPane) SetSize(w, h int) {
	p.width = w
	p.height = h
	bodyH := h - 1
	if bodyH < 0 {
		bodyH = 0
	}
	p.viewport.SetWidth(w)
	p.viewport.SetHeight(bodyH)
	p.viewport.SetContent(p.renderBody())
}

// SetEvents replaces the event slice, rebuilds body content, and pins the
// viewport to the bottom so the newest event is immediately visible.
// If cursor is active the selection is preserved (clamped to new length).
func (p *AuditPane) SetEvents(events []AuditEventDisplay) {
	p.events = events
	if !p.cursorActive {
		p.selectedIdx = -1
	} else if p.selectedIdx >= len(events) {
		p.selectedIdx = len(events) - 1
	}
	p.viewport.SetContent(p.renderBody())
	if !p.cursorActive {
		p.viewport.GotoBottom()
	}
}

// SetCursorActive enables or disables cursor mode.
// When deactivated the cursor is cleared and the viewport returns to the bottom.
func (p *AuditPane) SetCursorActive(active bool) {
	p.cursorActive = active
	if !active {
		p.selectedIdx = -1
		p.viewport.SetContent(p.renderBody())
		p.viewport.GotoBottom()
	} else if p.selectedIdx < 0 && len(p.events) > 0 {
		// Default to the most recent actionable event (lowest index = newest),
		// or the newest event if no actionable events exist.
		p.selectedIdx = p.lastActionableIdx()
		if p.selectedIdx < 0 {
			p.selectedIdx = 0 // 0 = newest
		}
		p.viewport.SetContent(p.renderBody())
		p.scrollToSelected()
	}
}

// CursorActive reports whether cursor mode is on.
func (p *AuditPane) CursorActive() bool { return p.cursorActive }

// CursorUp moves the cursor toward older events (up the visual display).
// events[0] is newest (bottom), events[len-1] is oldest (top).
func (p *AuditPane) CursorUp() {
	if !p.cursorActive || len(p.events) == 0 {
		return
	}
	// Moving UP visually = moving toward older events = increasing index.
	if p.selectedIdx < len(p.events)-1 {
		p.selectedIdx++
	}
	p.viewport.SetContent(p.renderBody())
	p.scrollToSelected()
}

// CursorDown moves the cursor toward newer events (down the visual display).
func (p *AuditPane) CursorDown() {
	if !p.cursorActive || len(p.events) == 0 {
		return
	}
	// Moving DOWN visually = moving toward newer events = decreasing index.
	if p.selectedIdx > 0 {
		p.selectedIdx--
	}
	p.viewport.SetContent(p.renderBody())
	p.scrollToSelected()
}

// SelectedEvent returns the currently selected event and true, or the zero
// value and false when no event is selected (cursor inactive or no events).
func (p *AuditPane) SelectedEvent() (AuditEventDisplay, bool) {
	if !p.cursorActive || p.selectedIdx < 0 || p.selectedIdx >= len(p.events) {
		return AuditEventDisplay{}, false
	}
	return p.events[p.selectedIdx], true
}

// SelectedEventHasActions reports whether the currently selected event has
// at least one available log action.
func (p *AuditPane) SelectedEventHasActions() bool {
	e, ok := p.SelectedEvent()
	if !ok {
		return false
	}
	return len(AuditLineActions(e)) > 0
}

// lastActionableIdx returns the index of the most-recent event (smallest index
// since events[0] is newest) that has available log actions. Returns -1 if none.
func (p *AuditPane) lastActionableIdx() int {
	for i := 0; i < len(p.events); i++ {
		if len(AuditLineActions(p.events[i])) > 0 {
			return i
		}
	}
	return -1
}

// scrollToSelected adjusts the viewport so the selected row is visible.
// renderBody renders events in oldest-first order (len-1 at top, 0 at bottom).
func (p *AuditPane) scrollToSelected() {
	if p.selectedIdx < 0 {
		return
	}
	// Count rendered lines up to the selected event.
	// renderBody iterates i = len-1 down to 0 (oldest to newest, top to bottom).
	linePos := 1 // blank line at top
	var lastMinute string
	msgW := p.width - 4
	if msgW < 10 {
		msgW = 10
	}
	for i := len(p.events) - 1; i >= 0; i-- {
		e := p.events[i]
		if e.Time != lastMinute {
			linePos++
			lastMinute = e.Time
		}
		wrapped := wordwrap.String(e.Message, msgW)
		linePos += len(strings.Split(wrapped, "\n"))
		if i == p.selectedIdx {
			break
		}
	}
	// Ensure selected line is within the visible viewport window.
	viewH := p.viewport.Height()
	yOff := p.viewport.YOffset()
	if linePos > yOff+viewH {
		p.viewport.SetYOffset(linePos - viewH + 1)
	} else if linePos < yOff+1 {
		p.viewport.SetYOffset(linePos - 1)
	}
}

// Events returns the current event slice (used by tests).
func (p *AuditPane) Events() []AuditEventDisplay {
	return p.events
}

// ScrollDown advances the viewport by n lines.
func (p *AuditPane) ScrollDown(n int) {
	p.viewport.ScrollDown(n)
}

// ScrollUp retreats the viewport by n lines.
func (p *AuditPane) ScrollUp(n int) {
	p.viewport.ScrollUp(n)
}

// Visible reports whether the pane is shown.
func (p *AuditPane) Visible() bool { return p.visible }

// Height returns the total height (header + body) set by SetSize.
func (p *AuditPane) Height() int { return p.height }

// ContentLines returns the number of rendered body lines (events + minute
// headers) excluding blank padding.  This is cached on every SetSize/SetEvents.
func (p *AuditPane) ContentLines() int { return p.bodyLines }

// ToggleVisible flips the visibility flag.
func (p *AuditPane) ToggleVisible() { p.visible = !p.visible }

// Audit-pane styles — Rosé Pine Moon palette.
var (
	auditDividerStyle  = lipgloss.NewStyle().Foreground(ColorSubtle)
	auditMinuteStyle   = lipgloss.NewStyle().Foreground(ColorOverlay)
	auditMsgStyle      = lipgloss.NewStyle().Foreground(ColorSubtle)
	auditWarnMsgStyle  = lipgloss.NewStyle().Foreground(ColorGold)
	auditErrMsgStyle   = lipgloss.NewStyle().Foreground(ColorLove)
	auditEmptyStyle    = lipgloss.NewStyle().Foreground(ColorMuted)
	auditRowPad        = lipgloss.NewStyle().PaddingLeft(1)
	auditSelectedStyle = lipgloss.NewStyle().Foreground(ColorText).Background(ColorOverlay)
	auditActionable    = lipgloss.NewStyle().Foreground(ColorIris) // indicator for actionable events
)

// String returns the full rendered output: header divider + viewport body.
func (p *AuditPane) String() string {
	return lipgloss.JoinVertical(lipgloss.Left, p.renderHeader(), p.viewport.View())
}

// renderHeader builds the centred "────── log ──────" divider line.
func (p *AuditPane) renderHeader() string {
	const label = " log "
	remaining := p.width - len(label)
	if remaining < 2 {
		return auditDividerStyle.Render(label)
	}
	left := remaining / 2
	right := remaining - left
	return auditDividerStyle.Render(
		strings.Repeat("─", left) + label + strings.Repeat("─", right),
	)
}

// renderMinuteHeader builds a centred "────── HH:MM ──────" divider.
func (p *AuditPane) renderMinuteHeader(minute string) string {
	inner := " " + minute + " "
	remaining := p.width - len(inner)
	if remaining < 2 {
		return auditMinuteStyle.Render(inner)
	}
	left := remaining / 2
	right := remaining - left
	return auditMinuteStyle.Render(
		strings.Repeat("─", left) + inner + strings.Repeat("─", right),
	)
}

// renderBody builds the scrollable event list.
//
// Layout per event row:
//
//	" ◆  message text"
//	 1 1 2 = 4 chars of overhead before message
//
// Continuation lines from word-wrapped messages are indented by the same
// overhead so they align under the first character of the message text.
// When cursor is active, the selected event is highlighted; actionable events
// carry a leading "›" indicator so users know they can open a context menu.
func (p *AuditPane) renderBody() string {
	if len(p.events) == 0 {
		p.bodyLines = 1
		return auditRowPad.Render(auditEmptyStyle.Render("· no events"))
	}

	const overhead = 4
	msgW := p.width - overhead
	if msgW < 10 {
		msgW = 10
	}

	// Walk events newest-first so that we can detect minute boundaries, then
	// append in reverse order so oldest events end up at the top of the output.
	lines := make([]string, 0, len(p.events)+1)
	lines = append(lines, "") // blank line below section header
	var lastMinute string

	for i := len(p.events) - 1; i >= 0; i-- {
		e := p.events[i]

		// Emit a centred minute header whenever the minute value changes.
		if e.Time != lastMinute {
			lines = append(lines, p.renderMinuteHeader(e.Time))
			lastMinute = e.Time
		}

		selected := p.cursorActive && i == p.selectedIdx
		hasActions := len(AuditLineActions(e)) > 0

		icon := lipgloss.NewStyle().Foreground(e.Color).Render(e.Icon)
		wrapped := wordwrap.String(e.Message, msgW)
		segments := strings.Split(wrapped, "\n")

		for j, seg := range segments {
			var styledSeg string
			if selected {
				styledSeg = auditSelectedStyle.Render(seg)
			} else {
				switch e.Level {
				case "error":
					styledSeg = auditErrMsgStyle.Render(seg)
				case "warn":
					styledSeg = auditWarnMsgStyle.Render(seg)
				default:
					styledSeg = auditMsgStyle.Render(seg)
				}
			}

			var row string
			if j == 0 {
				actionIndicator := " "
				if hasActions && !selected {
					actionIndicator = auditActionable.Render("›")
				} else if hasActions && selected {
					actionIndicator = auditSelectedStyle.Render("›")
				}
				row = auditRowPad.Render(actionIndicator + icon + " " + styledSeg)
			} else {
				if selected {
					row = auditRowPad.Render(auditSelectedStyle.Render(strings.Repeat(" ", overhead) + seg))
				} else {
					row = auditRowPad.Render(strings.Repeat(" ", overhead) + styledSeg)
				}
			}
			lines = append(lines, row)
		}
	}

	p.bodyLines = len(lines)
	return strings.Join(lines, "\n")
}

// EventKindIcon maps an event kind string to a display glyph and colour.
// Used by the app layer when constructing AuditEventDisplay values.
func EventKindIcon(kind string) (icon string, clr color.Color) {
	switch kind {
	case "agent_spawned":
		return "◆", ColorFoam
	case "agent_finished":
		return "✓", ColorGold
	case "agent_killed":
		return "✕", ColorLove
	case "agent_paused":
		return "⏸", ColorMuted
	case "agent_resumed":
		return "▶", ColorFoam
	case "plan_transition":
		return "⟳", ColorIris
	case "plan_created":
		return "✦", ColorFoam
	case "plan_merged":
		return "⇒", ColorGold
	case "plan_cancelled":
		return "✕", ColorLove
	case "wave_started":
		return "↯", ColorGold
	case "wave_completed":
		return "↯", ColorFoam
	case "wave_failed":
		return "↯", ColorLove
	case "prompt_sent":
		return "→", ColorFoam
	case "git_push":
		return "↑", ColorFoam
	case "pr_created":
		return "⎇", ColorIris
	case "permission_detected":
		return "!", ColorGold
	case "permission_answered":
		return "✓", ColorGold
	case "session_started":
		return "▶", ColorFoam
	case "session_stopped":
		return "■", ColorMuted
	case "fsm_error", "error":
		return "!", ColorLove
	default:
		return "·", ColorMuted
	}
}
