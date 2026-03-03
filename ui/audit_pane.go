package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

// AuditEventDisplay is a pre-formatted event record for the audit pane.
type AuditEventDisplay struct {
	Time    string         // wall-clock time formatted as "HH:MM"
	Kind    string         // event kind string (e.g. "agent_spawned")
	Icon    string         // single-character icon glyph
	Message string         // human-readable event description
	Color   lipgloss.Color // icon foreground colour
	Level   string         // "info", "warn", or "error"
}

// AuditPane renders a chronological, scrollable activity feed.
type AuditPane struct {
	events   []AuditEventDisplay
	viewport viewport.Model
	width    int
	height   int
	visible  bool
}

// NewAuditPane returns an AuditPane that is visible by default.
func NewAuditPane() *AuditPane {
	return &AuditPane{
		visible:  true,
		viewport: viewport.New(0, 0),
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
	p.viewport.Width = w
	p.viewport.Height = bodyH
	p.viewport.SetContent(p.renderBody())
}

// SetEvents replaces the event slice, rebuilds body content, and pins the
// viewport to the bottom so the newest event is immediately visible.
func (p *AuditPane) SetEvents(events []AuditEventDisplay) {
	p.events = events
	p.viewport.SetContent(p.renderBody())
	p.viewport.GotoBottom()
}

// Events returns the current event slice (used by tests).
func (p *AuditPane) Events() []AuditEventDisplay {
	return p.events
}

// ScrollDown advances the viewport by n lines.
func (p *AuditPane) ScrollDown(n int) {
	p.viewport.LineDown(n)
}

// ScrollUp retreats the viewport by n lines.
func (p *AuditPane) ScrollUp(n int) {
	p.viewport.LineUp(n)
}

// Visible reports whether the pane is shown.
func (p *AuditPane) Visible() bool { return p.visible }

// Height returns the total height (header + body) set by SetSize.
func (p *AuditPane) Height() int { return p.height }

// ToggleVisible flips the visibility flag.
func (p *AuditPane) ToggleVisible() { p.visible = !p.visible }

// Audit-pane styles — Rosé Pine Moon palette.
var (
	auditDividerStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	auditMinuteStyle  = lipgloss.NewStyle().Foreground(ColorMuted)
	auditMsgStyle     = lipgloss.NewStyle().Foreground(ColorSubtle)
	auditWarnMsgStyle = lipgloss.NewStyle().Foreground(ColorGold)
	auditErrMsgStyle  = lipgloss.NewStyle().Foreground(ColorLove)
	auditEmptyStyle   = lipgloss.NewStyle().Foreground(ColorMuted)
	auditRowPad       = lipgloss.NewStyle().PaddingLeft(1)
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
func (p *AuditPane) renderBody() string {
	if len(p.events) == 0 {
		return auditRowPad.Render(auditEmptyStyle.Render("· no events"))
	}

	const overhead = 4
	msgW := p.width - overhead
	if msgW < 10 {
		msgW = 10
	}

	// Walk events newest-first so that we can detect minute boundaries, then
	// append in reverse order so oldest events end up at the top of the output.
	lines := make([]string, 0, len(p.events))
	var lastMinute string

	for i := len(p.events) - 1; i >= 0; i-- {
		e := p.events[i]

		// Emit a centred minute header whenever the minute value changes.
		if e.Time != lastMinute {
			lines = append(lines, p.renderMinuteHeader(e.Time))
			lastMinute = e.Time
		}

		icon := lipgloss.NewStyle().Foreground(e.Color).Render(e.Icon)
		wrapped := wordwrap.String(e.Message, msgW)
		segments := strings.Split(wrapped, "\n")

		for j, seg := range segments {
			var styledSeg string
			switch e.Level {
			case "error":
				styledSeg = auditErrMsgStyle.Render(seg)
			case "warn":
				styledSeg = auditWarnMsgStyle.Render(seg)
			default:
				styledSeg = auditMsgStyle.Render(seg)
			}

			var row string
			if j == 0 {
				row = auditRowPad.Render(icon + "  " + styledSeg)
			} else {
				row = auditRowPad.Render(strings.Repeat(" ", overhead) + styledSeg)
			}
			lines = append(lines, row)
		}
	}

	return strings.Join(lines, "\n")
}

// EventKindIcon maps an event kind string to a display glyph and colour.
// Used by the app layer when constructing AuditEventDisplay values.
func EventKindIcon(kind string) (icon string, color lipgloss.Color) {
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
		return "⚡", ColorGold
	case "wave_completed":
		return "⚡", ColorFoam
	case "wave_failed":
		return "⚡", ColorLove
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
