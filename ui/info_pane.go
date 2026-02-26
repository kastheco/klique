package ui

import "github.com/charmbracelet/bubbles/viewport"

// InfoData holds the data to render in the info pane.
// Built by the app layer from instance + plan + wave state.
type InfoData struct {
	// Instance fields
	Title   string
	Program string
	Branch  string
	Path    string
	Created string
	Status  string

	// Plan fields (empty for ad-hoc)
	PlanName        string
	PlanDescription string
	PlanStatus      string
	PlanTopic       string
	PlanBranch      string
	PlanCreated     string

	// Wave fields (zero values = no wave)
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

// render builds the styled content string. Called internally when data changes.
func (p *InfoPane) render() string {
	return "info pane placeholder"
}
