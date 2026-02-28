package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
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

// AuditPane renders a scrollable list of recent audit events below the sidebar.
type AuditPane struct {
	events      []AuditEventDisplay
	viewport    viewport.Model
	width       int
	height      int
	visible     bool
	filterLabel string
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
	// Reserve 1 line for the header.
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

// SetFilter updates the filter label shown in the header.
func (p *AuditPane) SetFilter(label string) {
	p.filterLabel = label
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

// ToggleVisible flips the visibility state.
func (p *AuditPane) ToggleVisible() {
	p.visible = !p.visible
}

var (
	auditHeaderStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	auditTimeStyle   = lipgloss.NewStyle().Foreground(ColorMuted)
	auditMsgStyle    = lipgloss.NewStyle().Foreground(ColorText)
	auditEmptyStyle  = lipgloss.NewStyle().Foreground(ColorMuted)
)

// String renders the audit pane: a 1-line header + scrollable body.
func (p *AuditPane) String() string {
	header := p.renderHeader()
	body := p.viewport.View()
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

func (p *AuditPane) renderHeader() string {
	left := "── log ──"
	right := p.filterLabel
	if right == "" {
		right = "all"
	}

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := p.width - leftW - rightW
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	return auditHeaderStyle.Render(line)
}

func (p *AuditPane) renderBody() string {
	if len(p.events) == 0 {
		return auditEmptyStyle.Render("no events")
	}

	lines := make([]string, 0, len(p.events))
	for _, e := range p.events {
		icon := lipgloss.NewStyle().Foreground(e.Color).Render(e.Icon)
		time := auditTimeStyle.Render(e.Time)
		msg := auditMsgStyle.Render(e.Message)
		line := time + " " + icon + " " + msg
		lines = append(lines, line)
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
