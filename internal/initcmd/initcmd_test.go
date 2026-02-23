package initcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionsDefaults(t *testing.T) {
	opts := Options{}
	assert.False(t, opts.Force)
	assert.False(t, opts.Clean)
}

// TestWritePhase verifies the post-wizard write path: TOML config is written
// and can be loaded back correctly. Does not run the interactive wizard.
func TestWritePhase(t *testing.T) {
	// Set up temp HOME to avoid touching real config
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create config dir
	configDir := filepath.Join(tmpHome, ".config", "kasmos")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	// Simulate wizard output
	temp := 0.7
	tc := &config.TOMLConfig{
		Phases: map[string]string{
			"implementing": "coder",
			"planning":     "planner",
		},
		Agents: map[string]config.TOMLAgent{
			"coder": {
				Enabled:     true,
				Program:     "opencode",
				Model:       "anthropic/claude-sonnet-4-6",
				Temperature: &temp,
				Effort:      "high",
				Flags:       []string{},
			},
			"planner": {
				Enabled: true,
				Program: "claude",
				Model:   "claude-opus-4-6",
				Flags:   []string{},
			},
		},
	}

	// Write TOML config
	err := config.SaveTOMLConfig(tc)
	require.NoError(t, err)

	// Verify TOML file exists
	tomlPath := filepath.Join(configDir, "config.toml")
	assert.FileExists(t, tomlPath)

	// Verify it can be loaded back
	result, err := config.LoadTOMLConfigFrom(tomlPath)
	require.NoError(t, err)
	assert.Equal(t, "coder", result.PhaseRoles["implementing"])
	assert.Equal(t, "opencode", result.Profiles["coder"].Program)
}
