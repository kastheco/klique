package wizard

import (
	"fmt"
	"io"
	"strings"
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

func TestRoleDefaults(t *testing.T) {
	defaults := RoleDefaults()

	t.Run("has all three roles", func(t *testing.T) {
		assert.Contains(t, defaults, "coder")
		assert.Contains(t, defaults, "reviewer")
		assert.Contains(t, defaults, "planner")
	})

	t.Run("coder defaults", func(t *testing.T) {
		c := defaults["coder"]
		assert.Equal(t, "anthropic/claude-sonnet-4-6", c.Model)
		assert.Equal(t, "medium", c.Effort)
		assert.Equal(t, "0.1", c.Temperature)
		assert.True(t, c.Enabled)
	})

	t.Run("planner defaults", func(t *testing.T) {
		p := defaults["planner"]
		assert.Equal(t, "anthropic/claude-opus-4-6", p.Model)
		assert.Equal(t, "max", p.Effort)
		assert.Equal(t, "0.5", p.Temperature)
		assert.True(t, p.Enabled)
	})

	t.Run("reviewer defaults", func(t *testing.T) {
		r := defaults["reviewer"]
		assert.Equal(t, "openai/gpt-5.3-codex", r.Model)
		assert.Equal(t, "xhigh", r.Effort)
		assert.Equal(t, "0.2", r.Temperature)
		assert.True(t, r.Enabled)
	})
}

func TestFormatAgentSummary(t *testing.T) {
	t.Run("full settings", func(t *testing.T) {
		a := AgentState{
			Role: "coder", Harness: "opencode",
			Model:  "anthropic/claude-sonnet-4-6",
			Effort: "medium", Temperature: "0.1", Enabled: true,
		}
		s := FormatAgentSummary(a)
		assert.Contains(t, s, "opencode")
		assert.Contains(t, s, "anthropic/claude-sonnet-4-6")
		assert.Contains(t, s, "medium")
		assert.Contains(t, s, "temp=0.1")
	})

	t.Run("no temperature", func(t *testing.T) {
		a := AgentState{
			Role: "coder", Harness: "claude",
			Model:  "claude-sonnet-4-6",
			Effort: "high", Temperature: "", Enabled: true,
		}
		s := FormatAgentSummary(a)
		assert.Contains(t, s, "claude")
		assert.Contains(t, s, "claude-sonnet-4-6")
		assert.Contains(t, s, "high")
		assert.NotContains(t, s, "temp=")
	})

	t.Run("disabled", func(t *testing.T) {
		a := AgentState{
			Role: "planner", Harness: "codex", Enabled: false,
		}
		s := FormatAgentSummary(a)
		assert.Contains(t, s, "disabled")
	})
}

func TestPromptCustomize(t *testing.T) {
	t.Run("empty input (Enter) returns false", func(t *testing.T) {
		r := strings.NewReader("\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.False(t, result)
	})

	t.Run("n returns false", func(t *testing.T) {
		r := strings.NewReader("n\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.False(t, result)
	})

	t.Run("N returns false", func(t *testing.T) {
		r := strings.NewReader("N\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.False(t, result)
	})

	t.Run("y returns true", func(t *testing.T) {
		r := strings.NewReader("y\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.True(t, result)
	})

	t.Run("Y returns true", func(t *testing.T) {
		r := strings.NewReader("Y\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.True(t, result)
	})

	t.Run("junk defaults to false", func(t *testing.T) {
		r := strings.NewReader("hello\n")
		result := PromptCustomize(r, io.Discard, "coder", "opencode / claude-sonnet-4-6")
		assert.False(t, result)
	})
}

func TestPrePopulateFromExisting(t *testing.T) {
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

	// Simulate what runAgentStage now does for pre-population
	roles := DefaultAgentRoles()
	defaults := RoleDefaults()
	var agents []AgentState
	for _, role := range roles {
		as := defaults[role]
		if as.Harness == "" {
			as.Harness = "claude"
		}
		if profile, ok := existing.Profiles[role]; ok {
			as.Harness = profile.Program
			as.Model = profile.Model
			as.Effort = profile.Effort
			as.Enabled = profile.Enabled
			as.Temperature = "" // clear role default when existing config found
			if profile.Temperature != nil {
				as.Temperature = fmt.Sprintf("%g", *profile.Temperature)
			}
		}
		agents = append(agents, as)
	}

	assert.Equal(t, "opencode", agents[0].Harness) // coder got pre-populated
	assert.Equal(t, "claude", agents[1].Harness)   // reviewer got harness default
	// reviewer still has role defaults for model/effort
	assert.Equal(t, "openai/gpt-5.3-codex", agents[1].Model)
	assert.Equal(t, "xhigh", agents[1].Effort)
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
