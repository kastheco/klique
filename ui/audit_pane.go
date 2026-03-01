package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

// AuditEventDisplay is a pre-formatted event for rendering in the audit pane.
type AuditEventDisplay struct {
	Time    string         // formatted as "HH:MM"
	Kind    string         // event kind string (e.g. "agent_spawned")
	Icon    string         // single-char icon
	Message string         // human-readable message
	Color   lipgloss.Color // icon color
	Level   string         // "info", "warn", "error"
}

// AuditPane renders a scrollable activity feed inside the navigation panel border.
type AuditPane struct {
	events   []AuditEventDisplay
	viewport viewport.Model
	width    int
	height   int
	visible  bool
}

// NewAuditPane creates a new AuditPane (visible by default).
func NewAuditPane() *AuditPane {
	vp := viewport.New(0, 0)
	return &AuditPane{
		visible:  true,
		viewport: vp,
	}
}

// SetSize updates the pane dimensions and rebuilds the viewport content.
func (p *AuditPane) SetSize(w, h int) {
	p.width = w
	// Reserve 1 line for the header divider.
	bodyH := h - 1
	if bodyH < 0 {
		bodyH = 0
	}
	p.height = h
	p.viewport.Width = w
	p.viewport.Height = bodyH
	p.viewport.SetContent(p.renderBody())
}

// SetEvents replaces the event list and refreshes the viewport.
func (p *AuditPane) SetEvents(events []AuditEventDisplay) {
	p.events = events
	p.viewport.SetContent(p.renderBody())
	p.viewport.GotoTop()
}

// Events returns the current list of pre-formatted audit event displays.
// Primarily used in tests to inspect formatted event data without needing
// a sized viewport.
func (p *AuditPane) Events() []AuditEventDisplay {
	return p.events
}

// ScrollDown scrolls the viewport down by n lines.
func (p *AuditPane) ScrollDown(n int) {
	p.viewport.LineDown(n)
}

// ScrollUp scrolls the viewport up by n lines.
func (p *AuditPane) ScrollUp(n int) {
	p.viewport.LineUp(n)
}

// Visible returns whether the pane is currently shown.
func (p *AuditPane) Visible() bool {
	return p.visible
}

// Height returns the current total height of the pane (header + body).
func (p *AuditPane) Height() int {
	return p.height
}

// ToggleVisible flips the visibility state.
func (p *AuditPane) ToggleVisible() {
	p.visible = !p.visible
}

// Styles for the audit pane — Rosé Pine Moon palette.
var (
	auditDividerStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	auditTimeStyle    = lipgloss.NewStyle().Foreground(ColorMuted).Faint(true)
	auditMsgStyle     = lipgloss.NewStyle().Foreground(ColorSubtle)
	auditWarnMsgStyle = lipgloss.NewStyle().Foreground(ColorGold)
	auditErrMsgStyle  = lipgloss.NewStyle().Foreground(ColorLove)
	auditEmptyStyle   = lipgloss.NewStyle().Foreground(ColorMuted)
	auditRowPad       = lipgloss.NewStyle().PaddingLeft(1)
)

// String renders the audit pane: a 1-line header divider + scrollable body.
func (p *AuditPane) String() string {
	header := p.renderHeader()
	body := p.viewport.View()
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

// renderHeader builds a centered divider: ────── log ──────
// Matches the nav panel's navDividerLine aesthetic exactly.
func (p *AuditPane) renderHeader() string {
	inner := " log "
	innerW := len(inner)
	remaining := p.width - innerW
	if remaining < 2 {
		return auditDividerStyle.Render(inner)
	}
	left := remaining / 2
	right := remaining - left
	return auditDividerStyle.Render(strings.Repeat("─", left) + inner + strings.Repeat("─", right))
}

// renderBody builds the scrollable event list content.
func (p *AuditPane) renderBody() string {
	if len(p.events) == 0 {
		return auditRowPad.Render(
			auditEmptyStyle.Render("· no events"),
		)
	}

	// Available width for message text after time + icon + padding.
	// Layout per row: " HH:MM  ◆  message"
	//                  1  5   2 1 2 = 11 chars of overhead
	const overhead = 11
	msgWidth := p.width - overhead
	if msgWidth < 10 {
		msgWidth = 10
	}
	// Continuation lines indent to align under the message text.
	const contIndent = overhead

	lines := make([]string, 0, len(p.events))
	for _, e := range p.events {
		icon := lipgloss.NewStyle().Foreground(e.Color).Render(e.Icon)
		ts := auditTimeStyle.Render(e.Time)

		// Word-wrap the message at msgWidth, then render each wrapped segment.
		wrapped := wordwrap.String(e.Message, msgWidth)
		segments := strings.Split(wrapped, "\n")

		for i, seg := range segments {
			var styledSeg string
			switch e.Level {
			case "error":
				styledSeg = auditErrMsgStyle.Render(seg)
			case "warn":
				styledSeg = auditWarnMsgStyle.Render(seg)
			default:
				styledSeg = auditMsgStyle.Render(seg)
			}
			var line string
			if i == 0 {
				line = auditRowPad.Render(ts + "  " + icon + "  " + styledSeg)
			} else {
				line = auditRowPad.Render(strings.Repeat(" ", contIndent) + styledSeg)
			}
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// EventKindIcon returns the icon and color for a given event kind string.
// Used by the app layer when building AuditEventDisplay values.
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
	case "fsm_error", "error":
		return "!", ColorLove
	default:
		return "·", ColorMuted
	}
}
