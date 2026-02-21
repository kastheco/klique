package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/klique/internal/initcmd/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allTools = []string{"sg", "comby", "difft", "sd", "yq", "mlr", "glow", "typos", "scc", "tokei", "watchexec", "hyperfine", "procs", "mprocs"}

func TestValidateRole(t *testing.T) {
	t.Run("valid roles pass", func(t *testing.T) {
		for _, role := range []string{"coder", "reviewer", "planner", "my-agent", "agent_1"} {
			assert.NoError(t, validateRole(role), "role: %q", role)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		for _, role := range []string{"../etc/passwd", "../../.bashrc", "a/b", "a\\b"} {
			assert.Error(t, validateRole(role), "role: %q", role)
		}
	})

	t.Run("empty role rejected", func(t *testing.T) {
		assert.Error(t, validateRole(""))
	})
}

func TestScaffoldClaudeProject(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
	}

	_, err := WriteClaudeProject(dir, agents, allTools, false)
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

	_, err := WriteOpenCodeProject(dir, agents, allTools, false)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".opencode", "agents", "coder.md"))
}

func TestScaffoldCodexProject(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "codex", Model: "gpt-5.3-codex", Enabled: true},
	}

	_, err := WriteCodexProject(dir, agents, allTools, false)
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

	results, err := WriteClaudeProject(dir, agents, allTools, false) // force=false
	require.NoError(t, err)

	// File preserved
	content, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Equal(t, "custom content", string(content))

	// Result correctly shows skipped
	require.Len(t, results, 1)
	assert.False(t, results[0].Created)
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

	results, err := WriteClaudeProject(dir, agents, allTools, true) // force=true
	require.NoError(t, err)

	// Content overwritten
	content, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.NotEqual(t, "old content", string(content))

	// Result correctly shows created
	require.Len(t, results, 1)
	assert.True(t, results[0].Created)
}

func TestScaffoldAll_MixedHarnesses(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "anthropic/claude-opus-4-6", Enabled: true},
		{Role: "planner", Harness: "codex", Model: "gpt-5.3-codex", Enabled: true},
	}

	results, err := ScaffoldAll(dir, agents, allTools, false)
	require.NoError(t, err)

	// All three harness directories created
	assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "coder.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "agents", "reviewer.md"))
	assert.FileExists(t, filepath.Join(dir, ".codex", "AGENTS.md"))

	// Results only include actually-created files
	assert.GreaterOrEqual(t, len(results), 3)
	for _, r := range results {
		assert.True(t, r.Created, "expected all results to be created in fresh dir")
	}
}

func TestScaffoldRejectsPathTraversalRole(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "../../../.bashrc", Harness: "claude", Enabled: true},
	}

	_, err := WriteClaudeProject(dir, agents, allTools, false)
	assert.Error(t, err)
}

func TestScaffoldFiltersByHarness(t *testing.T) {
	dir := t.TempDir()
	// Pass a mix: only the claude agent should be written
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "anthropic/claude-opus-4-6", Enabled: true},
	}

	results, err := WriteClaudeProject(dir, agents, allTools, false)
	require.NoError(t, err)

	// Only coder.md created (claude only)
	assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "coder.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".opencode", "agents", "reviewer.md"))
	require.Len(t, results, 1)
	assert.Equal(t, ".claude/agents/coder.md", results[0].Path)
}

func TestToolsReferenceInjected(t *testing.T) {
	t.Run("claude agents include tools reference", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		}

		_, err := WriteClaudeProject(dir, agents, allTools, false)
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

		_, err := WriteOpenCodeProject(dir, agents, allTools, false)
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

		_, err := WriteCodexProject(dir, agents, allTools, false)
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

		_, err := WriteClaudeProject(dir, agents, allTools, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "{{MODEL}}")
		assert.Contains(t, string(content), "claude-opus-4-6")
	})

	t.Run("filtered tools reference omits unselected tools", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		}

		_, err := WriteClaudeProject(dir, agents, []string{"sg", "difft"}, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "ast-grep")
		assert.Contains(t, string(content), "difft")
		assert.NotContains(t, string(content), "comby")
		assert.NotContains(t, string(content), "typos")
		assert.NotContains(t, string(content), "watchexec")
	})

	t.Run("empty tools selection produces no tools reference", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		}

		_, err := WriteClaudeProject(dir, agents, []string{}, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "ast-grep")
		assert.NotContains(t, string(content), "Available CLI Tools")
	})
}
