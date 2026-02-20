package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTOMLConfig(t *testing.T) {
	t.Run("parses valid TOML with agents and phases", func(t *testing.T) {
		tmpDir := t.TempDir()
		tomlPath := filepath.Join(tmpDir, "config.toml")

		content := `
[phases]
implementing = "coder"
spec_review = "reviewer"
quality_review = "reviewer"
planning = "planner"

[agents.coder]
enabled = true
program = "opencode"
model = "anthropic/claude-sonnet-4-6"
temperature = 0.7
effort = "high"
flags = []

[agents.reviewer]
enabled = true
program = "claude"
model = "claude-opus-4-6"
effort = "high"
flags = ["--agent", "reviewer"]

[agents.planner]
enabled = false
program = "codex"
model = "gpt-5.3-codex"
flags = []
`
		err := os.WriteFile(tomlPath, []byte(content), 0o644)
		require.NoError(t, err)

		tc, err := LoadTOMLConfigFrom(tomlPath)
		require.NoError(t, err)

		// Verify phases
		assert.Equal(t, "coder", tc.PhaseRoles["implementing"])
		assert.Equal(t, "reviewer", tc.PhaseRoles["spec_review"])
		assert.Equal(t, "planner", tc.PhaseRoles["planning"])

		// Verify agent profiles
		coder, ok := tc.Profiles["coder"]
		require.True(t, ok)
		assert.Equal(t, "opencode", coder.Program)
		assert.Equal(t, "anthropic/claude-sonnet-4-6", coder.Model)
		assert.NotNil(t, coder.Temperature)
		assert.InDelta(t, 0.7, *coder.Temperature, 0.001)
		assert.Equal(t, "high", coder.Effort)
		assert.True(t, coder.Enabled)

		// Verify disabled agent
		planner, ok := tc.Profiles["planner"]
		require.True(t, ok)
		assert.False(t, planner.Enabled)

		// Verify flags preserved
		reviewer, ok := tc.Profiles["reviewer"]
		require.True(t, ok)
		assert.Equal(t, []string{"--agent", "reviewer"}, reviewer.Flags)
	})

	t.Run("returns error on missing file", func(t *testing.T) {
		_, err := LoadTOMLConfigFrom("/nonexistent/config.toml")
		assert.Error(t, err)
	})

	t.Run("returns error on invalid TOML", func(t *testing.T) {
		tmpDir := t.TempDir()
		tomlPath := filepath.Join(tmpDir, "config.toml")
		err := os.WriteFile(tomlPath, []byte("[invalid toml\n"), 0o644)
		require.NoError(t, err)

		_, err = LoadTOMLConfigFrom(tomlPath)
		assert.Error(t, err)
	})
}

func TestSaveTOMLConfig(t *testing.T) {
	t.Run("round-trips through save and load", func(t *testing.T) {
		tmpDir := t.TempDir()
		tomlPath := filepath.Join(tmpDir, "config.toml")

		temp := 0.5
		original := &TOMLConfig{
			Phases: map[string]string{
				"implementing": "coder",
				"planning":     "planner",
			},
			Agents: map[string]TOMLAgent{
				"coder": {
					Enabled:     true,
					Program:     "opencode",
					Model:       "anthropic/claude-sonnet-4-6",
					Temperature: &temp,
					Effort:      "high",
					Flags:       []string{},
				},
			},
		}

		err := SaveTOMLConfigTo(original, tomlPath)
		require.NoError(t, err)

		loaded, err := LoadTOMLConfigFrom(tomlPath)
		require.NoError(t, err)

		assert.Equal(t, original.Phases, loaded.PhaseRoles)
		coder := loaded.Profiles["coder"]
		assert.Equal(t, "opencode", coder.Program)
		assert.Equal(t, "anthropic/claude-sonnet-4-6", coder.Model)
		assert.InDelta(t, 0.5, *coder.Temperature, 0.001)
	})
}

func TestResolveProfileWithDisabledAgent(t *testing.T) {
	t.Run("disabled agent falls back to default", func(t *testing.T) {
		cfg := &Config{
			PhaseRoles: map[string]string{"planning": "planner"},
			Profiles: map[string]AgentProfile{
				"planner": {Program: "codex", Enabled: false},
			},
		}
		profile := cfg.ResolveProfile("planning", "claude")
		assert.Equal(t, "claude", profile.Program)
	})

	t.Run("enabled agent resolves normally", func(t *testing.T) {
		cfg := &Config{
			PhaseRoles: map[string]string{"implementing": "coder"},
			Profiles: map[string]AgentProfile{
				"coder": {Program: "opencode", Enabled: true},
			},
		}
		profile := cfg.ResolveProfile("implementing", "claude")
		assert.Equal(t, "opencode", profile.Program)
	})
}
