package harness

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	t.Run("registers all built-in harnesses", func(t *testing.T) {
		assert.NotNil(t, r.Get("claude"))
		assert.NotNil(t, r.Get("opencode"))
		assert.NotNil(t, r.Get("codex"))
		assert.Nil(t, r.Get("nonexistent"))
	})

	t.Run("All returns stable order", func(t *testing.T) {
		assert.Equal(t, []string{"claude", "opencode", "codex"}, r.All())
	})

	t.Run("DetectAll returns results for every harness", func(t *testing.T) {
		results := r.DetectAll()
		require.Len(t, results, 3)
		assert.Equal(t, "claude", results[0].Name)
		assert.Equal(t, "opencode", results[1].Name)
		assert.Equal(t, "codex", results[2].Name)
	})
}

func TestClaudeAdapter(t *testing.T) {
	c := &Claude{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "claude", c.Name())
	})

	t.Run("ListModels returns static list", func(t *testing.T) {
		models, err := c.ListModels()
		require.NoError(t, err)
		assert.Contains(t, models, "claude-sonnet-4-6")
		assert.Contains(t, models, "claude-opus-4-6")
		assert.Len(t, models, 4)
	})

	t.Run("BuildFlags with model and effort", func(t *testing.T) {
		flags := c.BuildFlags(AgentConfig{
			Model:  "claude-opus-4-6",
			Effort: "high",
		})
		assert.Equal(t, []string{"--model", "claude-opus-4-6", "--effort", "high"}, flags)
	})

	t.Run("BuildFlags skips empty fields", func(t *testing.T) {
		flags := c.BuildFlags(AgentConfig{})
		assert.Empty(t, flags)
	})

	t.Run("SupportsTemperature is false", func(t *testing.T) {
		assert.False(t, c.SupportsTemperature())
	})

	t.Run("SupportsEffort is true", func(t *testing.T) {
		assert.True(t, c.SupportsEffort())
	})
}

func TestCodexAdapter(t *testing.T) {
	c := &Codex{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "codex", c.Name())
	})

	t.Run("ListModels returns default", func(t *testing.T) {
		models, err := c.ListModels()
		require.NoError(t, err)
		assert.Equal(t, []string{"gpt-5.3-codex"}, models)
	})

	t.Run("BuildFlags with all fields", func(t *testing.T) {
		temp := 0.3
		flags := c.BuildFlags(AgentConfig{
			Model:       "gpt-5.3-codex",
			Effort:      "high",
			Temperature: &temp,
		})
		assert.Equal(t, []string{
			"-m", "gpt-5.3-codex",
			"-c", "reasoning.effort=high",
			"-c", "temperature=0.3",
		}, flags)
	})

	t.Run("SupportsTemperature is true", func(t *testing.T) {
		assert.True(t, c.SupportsTemperature())
	})

	t.Run("SupportsEffort is true", func(t *testing.T) {
		assert.True(t, c.SupportsEffort())
	})
}

func TestOpenCodeAdapter(t *testing.T) {
	o := &OpenCode{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "opencode", o.Name())
	})

	t.Run("BuildFlags returns only extra flags", func(t *testing.T) {
		flags := o.BuildFlags(AgentConfig{
			Model:      "anthropic/claude-sonnet-4-6",
			ExtraFlags: []string{"--verbose"},
		})
		// opencode uses project config, not CLI flags for model
		assert.Equal(t, []string{"--verbose"}, flags)
	})

	t.Run("SupportsTemperature is true", func(t *testing.T) {
		assert.True(t, o.SupportsTemperature())
	})

	t.Run("SupportsEffort is true", func(t *testing.T) {
		assert.True(t, o.SupportsEffort())
	})
}
