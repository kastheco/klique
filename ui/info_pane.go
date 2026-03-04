package ui

import (
	"fmt"
	"math"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone"
)

var (
	infoSectionStyle = lipgloss.NewStyle().Foreground(ColorFoam).Bold(true)
	infoDividerStyle = lipgloss.NewStyle().Foreground(ColorOverlay)
	infoLabelStyle   = lipgloss.NewStyle().Foreground(ColorMuted).Width(20)
	infoValueStyle   = lipgloss.NewStyle().Foreground(ColorText)
)

// InfoData carries display data for the info pane.
// Populated by the app layer from instance + plan + wave state.
type InfoData struct {
	// Instance fields
	Title   string
	Program string
	Branch  string
	Path    string
	Created string
	Status  string

	// Plan fields (empty when no plan is associated)
	PlanName        string
	PlanDescription string
	PlanStatus      string
	PlanTopic       string
	PlanBranch      string
	PlanCreated     string

	// Plan summary fields (rendered when plan header row is selected)
	PlanInstanceCount int
	PlanRunningCount  int
	PlanReadyCount    int
	PlanPausedCount   int
	PlanAddedLines    int
	PlanRemovedLines  int

	// Resource utilisation
	CPUPercent float64
	MemMB      float64

	// Wave / task context (zero values mean no wave info)
	AgentType  string
	WaveNumber int
	TotalWaves int
	TaskNumber int
	TotalTasks int
	WaveTasks  []WaveTaskInfo

	// Selection state flags
	HasPlan              bool
	HasInstance          bool
	IsPlanHeaderSelected bool
}

// WaveTaskInfo describes a single task slot in the current wave.
type WaveTaskInfo struct {
	Number int
	State  string // "complete", "running", "failed", or "pending"
}

// InfoPane renders instance and plan metadata inside a scrollable viewport.
type InfoPane struct {
	width, height int
	data          InfoData
	viewport      viewport.Model
}

// NewInfoPane returns a zero-sized InfoPane ready for use.
func NewInfoPane() *InfoPane {
	return &InfoPane{viewport: viewport.New(0, 0)}
}

// SetSize stores dimensions and refreshes the viewport.
func (p *InfoPane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.viewport.Width = width
	p.viewport.Height = height
	p.viewport.SetContent(p.render())
}

// SetData replaces the current data and re-renders from the top.
func (p *InfoPane) SetData(data InfoData) {
	p.data = data
	p.viewport.SetContent(p.render())
	p.viewport.GotoTop()
}

// ScrollUp moves the viewport one line toward the top.
func (p *InfoPane) ScrollUp() {
	p.viewport.LineUp(1)
}

// ScrollDown moves the viewport one line toward the bottom.
func (p *InfoPane) ScrollDown() {
	p.viewport.LineDown(1)
}

// String returns the visible portion of the pane, or a placeholder when
// neither an instance nor a plan header is selected.
func (p *InfoPane) String() string {
	if !p.data.HasInstance && !p.data.IsPlanHeaderSelected {
		return "no instance selected"
	}
	return p.viewport.View()
}

// statusColor maps known status strings to palette colors.
func statusColor(status string) lipgloss.TerminalColor {
	switch status {
	case "implementing":
		return ColorIris
	case "planning", "running":
		return ColorFoam
	case "reviewing", "done":
		return ColorGold
	case "ready", "cancelled", "paused":
		return ColorMuted
	case "failed", "error":
		return ColorLove
	default:
		return ColorText
	}
}

// renderRow renders a single label+value row.
func (p *InfoPane) renderRow(label, value string) string {
	valW := p.width - lipgloss.Width(infoLabelStyle.Render(label))
	if valW < 10 {
		valW = 10
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		infoLabelStyle.Render(label),
		infoValueStyle.Width(valW).Render(value),
	)
}

// renderStatusRow renders a label+value row where the value is coloured by status.
func (p *InfoPane) renderStatusRow(label, value string) string {
	valW := p.width - lipgloss.Width(infoLabelStyle.Render(label))
	if valW < 10 {
		valW = 10
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		infoLabelStyle.Render(label),
		lipgloss.NewStyle().Foreground(statusColor(value)).Width(valW).Render(value),
	)
}

// renderDivider renders a horizontal rule sized to the pane width.
func (p *InfoPane) renderDivider() string {
	w := p.width - 4
	if w < 10 {
		w = 10
	}
	return infoDividerStyle.Render(strings.Repeat("-", w))
}

// renderPlanSection renders the plan metadata block for instance-bound views.
func (p *InfoPane) renderPlanSection() string {
	rows := []string{
		infoSectionStyle.Render("plan"),
		p.renderDivider(),
	}
	if p.data.PlanName != "" {
		rows = append(rows, p.renderRow("name", p.data.PlanName))
	}
	if p.data.PlanDescription != "" {
		rows = append(rows, p.renderRow("description", p.data.PlanDescription))
	}
	if p.data.PlanStatus != "" {
		rows = append(rows, p.renderStatusRow("status", p.data.PlanStatus))
	}
	if p.data.PlanTopic != "" {
		rows = append(rows, p.renderRow("topic", p.data.PlanTopic))
	}
	if p.data.PlanBranch != "" {
		rows = append(rows, p.renderRow("branch", p.data.PlanBranch))
	}
	if p.data.PlanCreated != "" {
		rows = append(rows, p.renderRow("created", p.data.PlanCreated))
	}
	return strings.Join(rows, "\n")
}

// renderInstanceSection renders the instance metadata block.
func (p *InfoPane) renderInstanceSection() string {
	rows := []string{
		infoSectionStyle.Render("instance"),
		p.renderDivider(),
	}
	if p.data.Title != "" {
		rows = append(rows, p.renderRow("title", p.data.Title))
	}
	if p.data.AgentType != "" {
		rows = append(rows, p.renderRow("role", p.data.AgentType))
	}
	if p.data.Program != "" {
		rows = append(rows, p.renderRow("program", p.data.Program))
	}
	if p.data.Status != "" {
		rows = append(rows, p.renderStatusRow("status", p.data.Status))
	}
	if p.data.Branch != "" {
		rows = append(rows, p.renderRow("branch", p.data.Branch))
	}
	if p.data.Path != "" {
		rows = append(rows, p.renderRow("path", p.data.Path))
	}
	if p.data.Created != "" {
		rows = append(rows, p.renderRow("created", p.data.Created))
	}
	if p.data.WaveNumber > 0 {
		rows = append(rows, p.renderRow("wave", fmt.Sprintf("%d/%d", p.data.WaveNumber, p.data.TotalWaves)))
	}
	if p.data.TaskNumber > 0 {
		rows = append(rows, p.renderRow("task", fmt.Sprintf("%d of %d", p.data.TaskNumber, p.data.TotalTasks)))
	}
	if p.data.CPUPercent > 0 || p.data.MemMB > 0 {
		rows = append(rows, p.renderRow("cpu", fmt.Sprintf("%.0f%%", math.Round(p.data.CPUPercent))))
		rows = append(rows, p.renderRow("memory", fmt.Sprintf("%.0fM", p.data.MemMB)))
	}
	return strings.Join(rows, "\n")
}

// renderPlanSummary renders the plan header view: metadata, instance counts,
// line-change totals, and a "view plan doc" action button.
func (p *InfoPane) renderPlanSummary() string {
	rows := []string{
		infoSectionStyle.Render("plan"),
		p.renderDivider(),
	}
	if p.data.PlanName != "" {
		rows = append(rows, p.renderRow("name", p.data.PlanName))
	}
	if p.data.PlanStatus != "" {
		rows = append(rows, p.renderStatusRow("status", p.data.PlanStatus))
	}
	if p.data.PlanTopic != "" {
		rows = append(rows, p.renderRow("topic", p.data.PlanTopic))
	}
	if p.data.PlanBranch != "" {
		rows = append(rows, p.renderRow("branch", p.data.PlanBranch))
	}
	if p.data.PlanCreated != "" {
		rows = append(rows, p.renderRow("created", p.data.PlanCreated))
	}

	if p.data.PlanInstanceCount > 0 {
		summary := fmt.Sprintf("%d", p.data.PlanInstanceCount)
		var parts []string
		if p.data.PlanRunningCount > 0 {
			parts = append(parts, fmt.Sprintf("%d running", p.data.PlanRunningCount))
		}
		if p.data.PlanReadyCount > 0 {
			parts = append(parts, fmt.Sprintf("%d ready", p.data.PlanReadyCount))
		}
		if p.data.PlanPausedCount > 0 {
			parts = append(parts, fmt.Sprintf("%d paused", p.data.PlanPausedCount))
		}
		if len(parts) > 0 {
			summary += " (" + strings.Join(parts, ", ") + ")"
		}
		rows = append(rows, p.renderRow("instances", summary))
	}

	if p.data.PlanAddedLines > 0 || p.data.PlanRemovedLines > 0 {
		rows = append(rows, p.renderRow("lines changed",
			fmt.Sprintf("+%d -%d", p.data.PlanAddedLines, p.data.PlanRemovedLines)))
	}

	btnStyle := lipgloss.NewStyle().
		Foreground(ColorFoam).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorOverlay).
		Padding(0, 2)
	rows = append(rows, "")
	rows = append(rows, zone.Mark(ZoneViewPlan, btnStyle.Render("view plan doc")))

	return strings.Join(rows, "\n")
}

// renderWaveSection renders the task-state grid for the current wave.
func (p *InfoPane) renderWaveSection() string {
	rows := []string{
		infoSectionStyle.Render("wave progress"),
		p.renderDivider(),
	}
	for _, t := range p.data.WaveTasks {
		var glyph string
		var col lipgloss.TerminalColor
		switch t.State {
		case "complete":
			glyph, col = "✓", ColorFoam
		case "running":
			glyph, col = "●", ColorIris
		case "failed":
			glyph, col = "✗", ColorLove
		default:
			glyph, col = "○", ColorMuted
		}
		icon := lipgloss.NewStyle().Foreground(col).Render(glyph)
		rows = append(rows, infoLabelStyle.Render(fmt.Sprintf("task %d", t.Number))+icon+" "+t.State)
	}
	return strings.Join(rows, "\n")
}

// render assembles the full content string placed into the viewport.
func (p *InfoPane) render() string {
	if !p.data.HasInstance && !p.data.IsPlanHeaderSelected {
		return "no instance selected"
	}

	var sections []string
	switch {
	case p.data.IsPlanHeaderSelected:
		sections = append(sections, p.renderPlanSummary())
		if len(p.data.WaveTasks) > 0 {
			sections = append(sections, p.renderWaveSection())
		}
	default:
		if p.data.HasPlan {
			sections = append(sections, p.renderPlanSection())
		}
		sections = append(sections, p.renderInstanceSection())
		if len(p.data.WaveTasks) > 0 {
			sections = append(sections, p.renderWaveSection())
		}
	}

	return strings.Join(sections, "\n\n")
}
