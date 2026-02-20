package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/klique/internal/initcmd/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScaffoldClaudeProject(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
	}

	err := WriteClaudeProject(dir, agents, false)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "coder.md"))
	assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "agents", "planner.md"))
}

func TestScaffoldOpenCodeProject(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Enabled: true},
	}

	err := WriteOpenCodeProject(dir, agents, false)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".opencode", "agents", "coder.md"))
}

func TestScaffoldCodexProject(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "codex", Model: "gpt-5.3-codex", Enabled: true},
	}

	err := WriteCodexProject(dir, agents, false)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".codex", "AGENTS.md"))
}

func TestScaffoldSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	existing := filepath.Join(agentDir, "coder.md")
	require.NoError(t, os.WriteFile(existing, []byte("custom content"), 0o644))

	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Enabled: true},
	}

	err := WriteClaudeProject(dir, agents, false) // force=false
	require.NoError(t, err)

	content, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Equal(t, "custom content", string(content))
}

func TestScaffoldForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	existing := filepath.Join(agentDir, "coder.md")
	require.NoError(t, os.WriteFile(existing, []byte("old content"), 0o644))

	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Enabled: true},
	}

	err := WriteClaudeProject(dir, agents, true) // force=true
	require.NoError(t, err)

	content, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.NotEqual(t, "old content", string(content))
}

func TestToolsReferenceInjected(t *testing.T) {
	t.Run("claude agents include tools reference", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		}

		err := WriteClaudeProject(dir, agents, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "{{TOOLS_REFERENCE}}")
		assert.Contains(t, string(content), "ast-grep")
		assert.Contains(t, string(content), "difft")
		assert.Contains(t, string(content), "comby")
		assert.Contains(t, string(content), "typos")
		assert.Contains(t, string(content), "scc")
		assert.Contains(t, string(content), "yq")
		assert.Contains(t, string(content), "sd")
	})

	t.Run("opencode agents include tools reference", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Enabled: true},
		}

		err := WriteOpenCodeProject(dir, agents, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".opencode", "agents", "coder.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "{{TOOLS_REFERENCE}}")
		assert.Contains(t, string(content), "ast-grep")
	})

	t.Run("codex AGENTS.md includes tools reference", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "codex", Model: "gpt-5.3-codex", Enabled: true},
		}

		err := WriteCodexProject(dir, agents, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".codex", "AGENTS.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "{{TOOLS_REFERENCE}}")
		assert.Contains(t, string(content), "ast-grep")
	})

	t.Run("model placeholder is substituted", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
		}

		err := WriteClaudeProject(dir, agents, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "{{MODEL}}")
		assert.Contains(t, string(content), "claude-opus-4-6")
	})
}
