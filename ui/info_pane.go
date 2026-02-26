package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var (
	infoSectionStyle = lipgloss.NewStyle().Foreground(ColorFoam).Bold(true)
	infoDividerStyle = lipgloss.NewStyle().Foreground(ColorOverlay)
	infoLabelStyle   = lipgloss.NewStyle().Foreground(ColorMuted).Width(20)
	infoValueStyle   = lipgloss.NewStyle().Foreground(ColorText)
)

// InfoData holds the data to render in the info pane.
// Built by the app layer from instance + plan + wave state.
type InfoData struct {
	// Instance fields.
	Title   string
	Program string
	Branch  string
	Path    string
	Created string
	Status  string

	// Plan fields (empty for ad-hoc).
	PlanName        string
	PlanDescription string
	PlanStatus      string
	PlanTopic       string
	PlanBranch      string
	PlanCreated     string

	// Wave fields (zero values = no wave).
	AgentType  string
	WaveNumber int
	TotalWaves int
	TaskNumber int
	TotalTasks int
	WaveTasks  []WaveTaskInfo

	// HasPlan is true when the instance is bound to a plan.
	HasPlan bool
	// HasInstance is true when an instance is selected.
	HasInstance bool
}

// WaveTaskInfo describes a single task in the current wave.
type WaveTaskInfo struct {
	Number int
	State  string // "complete", "running", "failed", "pending"
}

// InfoPane renders instance and plan metadata in the info tab.
type InfoPane struct {
	width, height int
	data          InfoData
	viewport      viewport.Model
}

// NewInfoPane creates a new InfoPane.
func NewInfoPane() *InfoPane {
	vp := viewport.New(0, 0)
	return &InfoPane{viewport: vp}
}

// SetSize updates the pane dimensions.
func (p *InfoPane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.viewport.Width = width
	p.viewport.Height = height
	p.viewport.SetContent(p.render())
}

// SetData updates the data to render.
func (p *InfoPane) SetData(data InfoData) {
	p.data = data
	p.viewport.SetContent(p.render())
	p.viewport.GotoTop()
}

// ScrollUp scrolls the viewport up.
func (p *InfoPane) ScrollUp() {
	p.viewport.LineUp(1)
}

// ScrollDown scrolls the viewport down.
func (p *InfoPane) ScrollDown() {
	p.viewport.LineDown(1)
}

// String renders the info pane content.
func (p *InfoPane) String() string {
	if !p.data.HasInstance {
		return "no instance selected"
	}
	return p.viewport.View()
}

func statusColor(status string) lipgloss.TerminalColor {
	switch status {
	case "implementing", "planning":
		return ColorIris
	case "done", "running":
		return ColorFoam
	case "reviewing":
		return ColorGold
	case "ready", "cancelled", "paused":
		return ColorMuted
	case "failed", "error":
		return ColorLove
	default:
		return ColorText
	}
}

func (p *InfoPane) renderRow(label, value string) string {
	valueWidth := p.width - lipgloss.Width(infoLabelStyle.Render(label))
	if valueWidth < 10 {
		valueWidth = 10
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		infoLabelStyle.Render(label),
		infoValueStyle.Width(valueWidth).Render(value),
	)
}

func (p *InfoPane) renderStatusRow(label, value string) string {
	valueWidth := p.width - lipgloss.Width(infoLabelStyle.Render(label))
	if valueWidth < 10 {
		valueWidth = 10
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		infoLabelStyle.Render(label),
		lipgloss.NewStyle().Foreground(statusColor(value)).Width(valueWidth).Render(value),
	)
}

func (p *InfoPane) renderDivider() string {
	w := p.width - 4
	if w < 10 {
		w = 10
	}
	return infoDividerStyle.Render(strings.Repeat("-", w))
}

func (p *InfoPane) renderPlanSection() string {
	lines := []string{
		infoSectionStyle.Render("plan"),
		p.renderDivider(),
	}
	if p.data.PlanName != "" {
		lines = append(lines, p.renderRow("name", p.data.PlanName))
	}
	if p.data.PlanDescription != "" {
		lines = append(lines, p.renderRow("description", p.data.PlanDescription))
	}
	if p.data.PlanStatus != "" {
		lines = append(lines, p.renderStatusRow("status", p.data.PlanStatus))
	}
	if p.data.PlanTopic != "" {
		lines = append(lines, p.renderRow("topic", p.data.PlanTopic))
	}
	if p.data.PlanBranch != "" {
		lines = append(lines, p.renderRow("branch", p.data.PlanBranch))
	}
	if p.data.PlanCreated != "" {
		lines = append(lines, p.renderRow("created", p.data.PlanCreated))
	}
	return strings.Join(lines, "\n")
}

func (p *InfoPane) renderInstanceSection() string {
	lines := []string{
		infoSectionStyle.Render("instance"),
		p.renderDivider(),
	}
	if p.data.Title != "" {
		lines = append(lines, p.renderRow("title", p.data.Title))
	}
	if p.data.AgentType != "" {
		lines = append(lines, p.renderRow("role", p.data.AgentType))
	}
	if p.data.Program != "" {
		lines = append(lines, p.renderRow("program", p.data.Program))
	}
	if p.data.Status != "" {
		lines = append(lines, p.renderStatusRow("status", p.data.Status))
	}
	if p.data.Branch != "" {
		lines = append(lines, p.renderRow("branch", p.data.Branch))
	}
	if p.data.Path != "" {
		lines = append(lines, p.renderRow("path", p.data.Path))
	}
	if p.data.Created != "" {
		lines = append(lines, p.renderRow("created", p.data.Created))
	}
	if p.data.WaveNumber > 0 {
		lines = append(lines, p.renderRow("wave", fmt.Sprintf("%d/%d", p.data.WaveNumber, p.data.TotalWaves)))
	}
	if p.data.TaskNumber > 0 {
		lines = append(lines, p.renderRow("task", fmt.Sprintf("%d of %d", p.data.TaskNumber, p.data.TotalTasks)))
	}
	return strings.Join(lines, "\n")
}

func (p *InfoPane) renderWaveSection() string {
	lines := []string{
		infoSectionStyle.Render("wave progress"),
		p.renderDivider(),
	}
	for _, task := range p.data.WaveTasks {
		var glyph string
		var glyphColor lipgloss.TerminalColor
		switch task.State {
		case "complete":
			glyph = "✓"
			glyphColor = ColorFoam
		case "running":
			glyph = "●"
			glyphColor = ColorIris
		case "failed":
			glyph = "✗"
			glyphColor = ColorLove
		default:
			glyph = "○"
			glyphColor = ColorMuted
		}
		label := fmt.Sprintf("task %d", task.Number)
		value := lipgloss.NewStyle().Foreground(glyphColor).Render(glyph) + " " + task.State
		lines = append(lines, infoLabelStyle.Render(label)+value)
	}
	return strings.Join(lines, "\n")
}

// render builds the content string. Called internally when data changes.
func (p *InfoPane) render() string {
	if !p.data.HasInstance {
		return "no instance selected"
	}

	var sections []string
	if p.data.HasPlan {
		sections = append(sections, p.renderPlanSection())
	}
	sections = append(sections, p.renderInstanceSection())
	if len(p.data.WaveTasks) > 0 {
		sections = append(sections, p.renderWaveSection())
	}

	return strings.Join(sections, "\n\n")
}
