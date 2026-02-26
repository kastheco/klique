package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfoPane_NoInstance(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 24)
	p.SetData(InfoData{HasInstance: false})
	assert.Contains(t, p.String(), "no instance selected")
}

func TestInfoPane_AdHocInstance(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 24)
	p.SetData(InfoData{
		HasInstance: true,
		HasPlan:     false,
		Title:       "fix-login-bug",
		Program:     "opencode",
		Branch:      "kas/fix-login-bug",
		Path:        "/home/kas/dev/myapp",
		Created:     "2026-02-25 14:30",
		Status:      "running",
	})
	output := p.String()
	assert.Contains(t, output, "fix-login-bug")
	assert.Contains(t, output, "opencode")
	assert.Contains(t, output, "kas/fix-login-bug")
	assert.Contains(t, output, "running")
	assert.NotContains(t, output, "plan")
}

func TestInfoPane_PlanBoundInstance(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 24)
	p.SetData(InfoData{
		HasInstance:     true,
		HasPlan:         true,
		Title:           "my-feature-coder",
		Program:         "claude",
		Branch:          "plan/my-feature",
		Status:          "running",
		PlanName:        "my-feature",
		PlanDescription: "add dark mode toggle",
		PlanStatus:      "implementing",
		PlanTopic:       "ui",
		PlanBranch:      "plan/my-feature",
		PlanCreated:     "2026-02-25",
		AgentType:       "coder",
		WaveNumber:      2,
		TotalWaves:      3,
		TaskNumber:      4,
		TotalTasks:      6,
	})
	output := p.String()
	assert.Contains(t, output, "my-feature")
	assert.Contains(t, output, "add dark mode toggle")
	assert.Contains(t, output, "implementing")
	assert.Contains(t, output, "coder")
}

func TestInfoPane_WaveProgress(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 24)
	p.SetData(InfoData{
		HasInstance: true,
		HasPlan:     true,
		Title:       "test-coder",
		Program:     "claude",
		Status:      "running",
		PlanName:    "test-plan",
		PlanStatus:  "implementing",
		WaveTasks: []WaveTaskInfo{
			{Number: 1, State: "complete"},
			{Number: 2, State: "running"},
			{Number: 3, State: "pending"},
		},
	})
	output := p.String()
	assert.Contains(t, output, "✓")
	assert.Contains(t, output, "●")
	assert.Contains(t, output, "○")
}

func TestInfoPane_Scrolling(t *testing.T) {
	p := NewInfoPane()
	p.SetSize(80, 5)
	p.SetData(InfoData{
		HasInstance:     true,
		HasPlan:         true,
		Title:           "test",
		Program:         "claude",
		PlanName:        "test",
		PlanDescription: "desc",
		PlanStatus:      "ready",
		WaveTasks: []WaveTaskInfo{
			{Number: 1, State: "complete"},
			{Number: 2, State: "complete"},
			{Number: 3, State: "running"},
			{Number: 4, State: "pending"},
			{Number: 5, State: "pending"},
			{Number: 6, State: "pending"},
			{Number: 7, State: "pending"},
			{Number: 8, State: "pending"},
		},
	})
	before := p.String()
	require.NotEmpty(t, before)
	p.ScrollDown()
	after := p.String()
	assert.NotEqual(t, before, after)
}

func TestInfoPane_PlanSummary(t *testing.T) {
	pane := NewInfoPane()
	pane.SetSize(60, 30)
	pane.SetData(InfoData{
		IsPlanHeaderSelected: true,
		PlanName:             "my-feature",
		PlanStatus:           "implementing",
		PlanInstanceCount:    3,
		PlanRunningCount:     2,
		PlanReadyCount:       1,
	})

	output := pane.String()
	assert.Contains(t, output, "my-feature")
	assert.Contains(t, output, "implementing")
	assert.Contains(t, output, "3 (2 running, 1 ready)")
	assert.Contains(t, output, "view plan doc")
}

func TestInfoPane_InstanceWithResources(t *testing.T) {
	pane := NewInfoPane()
	pane.SetSize(60, 30)
	pane.SetData(InfoData{
		HasInstance: true,
		Title:       "task 1",
		Status:      "running",
		CPUPercent:  12.5,
		MemMB:       340,
	})

	output := pane.String()
	assert.Contains(t, output, "13%")
	assert.Contains(t, output, "340M")
}
