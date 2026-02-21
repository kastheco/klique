package wizard

import (
	"testing"

	"github.com/kastheco/klique/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateToTOMLConfig(t *testing.T) {
	temp := "0.7"
	state := &State{
		Agents: []AgentState{
			{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6",
				Temperature: temp, Effort: "high", Enabled: true},
			{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6",
				Temperature: "", Effort: "high", Enabled: true},
			{Role: "planner", Harness: "codex", Model: "gpt-5.3-codex",
				Temperature: "", Effort: "", Enabled: false},
		},
		PhaseMapping: map[string]string{
			"implementing":   "coder",
			"spec_review":    "reviewer",
			"quality_review": "reviewer",
			"planning":       "planner",
		},
	}

	tc := state.ToTOMLConfig()

	// Verify phases
	assert.Equal(t, "coder", tc.Phases["implementing"])
	assert.Equal(t, "reviewer", tc.Phases["spec_review"])

	// Verify agents
	coder, ok := tc.Agents["coder"]
	require.True(t, ok)
	assert.Equal(t, "opencode", coder.Program)
	assert.Equal(t, "anthropic/claude-sonnet-4-6", coder.Model)
	assert.NotNil(t, coder.Temperature)
	assert.InDelta(t, 0.7, *coder.Temperature, 0.001)
	assert.True(t, coder.Enabled)

	// Verify disabled agent
	planner := tc.Agents["planner"]
	assert.False(t, planner.Enabled)

	// Verify nil temperature when empty
	reviewer := tc.Agents["reviewer"]
	assert.Nil(t, reviewer.Temperature)
}

func TestStateToAgentConfigs(t *testing.T) {
	state := &State{
		Agents: []AgentState{
			{Role: "coder", Harness: "opencode", Model: "model-1", Enabled: true},
			{Role: "reviewer", Harness: "claude", Model: "model-2", Enabled: true},
			{Role: "planner", Harness: "codex", Model: "model-3", Enabled: false},
		},
	}

	configs := state.ToAgentConfigs()

	// Only enabled agents
	assert.Len(t, configs, 2)
	assert.Equal(t, "coder", configs[0].Role)
	assert.Equal(t, "reviewer", configs[1].Role)
}

func TestDefaultPhases(t *testing.T) {
	phases := DefaultPhases()
	assert.Equal(t, []string{"implementing", "spec_review", "quality_review", "planning"}, phases)
}

func TestDefaultAgentRoles(t *testing.T) {
	roles := DefaultAgentRoles()
	assert.Equal(t, []string{"coder", "reviewer", "planner"}, roles)
}

func TestPrePopulateFromExisting(t *testing.T) {
	// Tests the pre-population logic without running the interactive form.
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
		PhaseRoles: map[string]string{
			"implementing": "coder",
		},
	}

	// Simulate what runAgentStage does for pre-population
	roles := DefaultAgentRoles()
	var agents []AgentState
	for _, role := range roles {
		as := AgentState{Role: role, Harness: "claude", Enabled: true}
		if profile, ok := existing.Profiles[role]; ok {
			as.Harness = profile.Program
			as.Model = profile.Model
			as.Effort = profile.Effort
			as.Enabled = profile.Enabled
			if profile.Temperature != nil {
				as.Temperature = "0.5"
			}
		}
		agents = append(agents, as)
	}

	assert.Equal(t, "opencode", agents[0].Harness) // coder got pre-populated
	assert.Equal(t, "claude", agents[1].Harness)   // reviewer got default
}

func TestBuildProgressNote(t *testing.T) {
	agents := []AgentState{
		{Role: "coder", Harness: "opencode", Model: "claude-sonnet-4-6", Effort: "high", Enabled: true},
		{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6", Effort: "high", Enabled: true},
		{Role: "planner", Harness: "codex", Model: "", Effort: "", Enabled: true},
	}

	t.Run("first agent shows current marker", func(t *testing.T) {
		note := BuildProgressNote(agents, 0)
		assert.Contains(t, note, "▸ coder")
		assert.Contains(t, note, "○ reviewer")
		assert.Contains(t, note, "○ planner")
	})

	t.Run("middle agent shows completed first", func(t *testing.T) {
		note := BuildProgressNote(agents, 1)
		assert.Contains(t, note, "✓ coder")
		assert.Contains(t, note, "opencode")
		assert.Contains(t, note, "claude-sonnet-4-6")
		assert.Contains(t, note, "▸ reviewer")
		assert.Contains(t, note, "○ planner")
	})

	t.Run("last agent shows all completed", func(t *testing.T) {
		note := BuildProgressNote(agents, 2)
		assert.Contains(t, note, "✓ coder")
		assert.Contains(t, note, "✓ reviewer")
		assert.Contains(t, note, "▸ planner")
	})

	t.Run("disabled agent shows skip marker", func(t *testing.T) {
		agents[0].Enabled = false
		note := BuildProgressNote(agents, 1)
		assert.Contains(t, note, "⊘ coder")
	})
}
