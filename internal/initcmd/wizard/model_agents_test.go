package wizard

import (
	"strings"
	"testing"

	"github.com/kastheco/kasmos/config"
	"github.com/stretchr/testify/assert"
)

func TestAgentStep_BrowseNavigation(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "gpt-5.3-codex", Enabled: true},
		{Role: "planner", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude", "opencode"}, nil)
	assert.Equal(t, 0, s.cursor)
	assert.Equal(t, agentBrowseMode, s.mode)

	s.cursorDown()
	assert.Equal(t, 1, s.cursor)

	s.cursorDown()
	assert.Equal(t, 2, s.cursor)

	s.cursorDown()               // chat is skipped in navigation
	assert.Equal(t, 2, s.cursor) // clamped at planner
}

func TestAgentStep_ToggleEnabled(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Enabled: true},
		{Role: "reviewer", Harness: "claude", Enabled: true},
		{Role: "planner", Harness: "claude", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude"}, nil)
	s.cursor = 0
	s.toggleEnabled()
	assert.False(t, s.agents[0].Enabled)
	s.toggleEnabled()
	assert.True(t, s.agents[0].Enabled)
}

func TestAgentStep_DetailPanelContent(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Effort: "medium", Temperature: "0.1", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude"}, nil)
	detail := s.renderDetailPanel(60, 20)
	assert.Contains(t, detail, "coder")
	assert.NotContains(t, detail, "CODER")
	assert.Contains(t, detail, "claude-sonnet-4-6")
	assert.Contains(t, detail, "medium")
}

func TestAgentStep_EnterEditMode(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
	}
	modelCache := map[string][]string{
		"claude": {"claude-sonnet-4-6", "claude-opus-4-6", "claude-sonnet-4-5", "claude-haiku-4-5"},
	}

	s := newAgentStep(agents, []string{"claude", "opencode"}, modelCache)
	s.enterEditMode()
	assert.Equal(t, agentEditMode, s.mode)
	assert.Equal(t, 0, s.editField)
}

func TestAgentStep_EditFieldCycle(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude", "opencode"}, nil)
	s.enterEditMode()

	s.nextField()
	assert.Equal(t, 1, s.editField)

	s.nextField()
	assert.Equal(t, 2, s.editField)

	s.nextField()
	assert.Equal(t, 0, s.editField)
}

func TestAgentStep_ExitEditMode(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude"}, nil)
	s.enterEditMode()
	s.exitEditMode()
	assert.Equal(t, agentBrowseMode, s.mode)
}

func TestAgentStep_HarnessCycle(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Enabled: true},
	}
	harnesses := []string{"claude", "opencode"}

	s := newAgentStep(agents, harnesses, nil)
	s.enterEditMode()
	s.editField = 0

	s.cycleFieldValue(1)
	assert.Equal(t, "opencode", s.agents[0].Harness)

	s.cycleFieldValue(1)
	assert.Equal(t, "claude", s.agents[0].Harness)

	s.cycleFieldValue(-1)
	assert.Equal(t, "opencode", s.agents[0].Harness)
}

func TestAgentStep_EffortCycle(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Effort: "medium", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude"}, nil)
	s.effortLevels = map[string][]string{"claude": {"", "low", "medium", "high", "max"}}
	s.enterEditMode()
	s.editField = 2

	s.cycleFieldValue(1)
	assert.Equal(t, "high", s.agents[0].Effort)
}

func TestAgentStepPrePopulatesFromExisting(t *testing.T) {
	temp := 0.5
	existing := &config.TOMLConfigResult{
		Profiles: map[string]config.AgentProfile{
			"coder": {
				Program:     "opencode",
				Model:       "anthropic/claude-sonnet-4-6",
				Temperature: &temp,
				Effort:      "high",
				Enabled:     true,
			},
		},
	}

	agents := initAgentsFromExisting([]string{"claude", "opencode"}, existing)
	assert.Equal(t, "opencode", agents[0].Harness) // coder from existing
	assert.Equal(t, "high", agents[0].Effort)
	assert.Equal(t, "0.5", agents[0].Temperature)
	// reviewer gets defaults
	assert.Equal(t, "openai/gpt-5.3-codex", agents[1].Model)
}

func TestAgentStep_ViewSeparatorFillsPanelHeight(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "claude", Model: "anthropic/claude-sonnet-4-6", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "openai/gpt-5.3-codex", Enabled: true},
		{Role: "planner", Harness: "claude", Model: "anthropic/claude-opus-4-6", Enabled: true},
	}

	s := newAgentStep(agents, []string{"claude", "opencode"}, nil)
	view := s.View(100, 20)
	assert.Equal(t, 20, strings.Count(view, "┊"))
}

func TestTruncateForCell(t *testing.T) {
	assert.Equal(t, "", truncateForCell("abc", 0))
	assert.Equal(t, "abc", truncateForCell("abc", 3))
	assert.Equal(t, "ab...", truncateForCell("abcdef", 5))
}
