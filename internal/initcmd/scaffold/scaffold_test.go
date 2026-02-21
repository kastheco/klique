package scaffold

import (
	"os"
	"path/filepath"
	"strings"
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

	t.Run("cli-tools directive is always present regardless of selectedTools", func(t *testing.T) {
		dir := t.TempDir()
		agents := []harness.AgentConfig{
			{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		}

		// Even with no selected tools, the mandatory directive is always written
		_, err := WriteClaudeProject(dir, agents, []string{}, false)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "coder.md"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "CLI Tools (MANDATORY)")
		assert.Contains(t, string(content), "cli-tools")
	})
}

func TestWriteProjectSkills(t *testing.T) {
	dir := t.TempDir()

	results, err := WriteProjectSkills(dir, false)
	require.NoError(t, err)

	// All four skills written (including cli-tools)
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "golang-pro", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tui-design", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tmux-orchestration", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "SKILL.md"))

	// Reference files included
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tui-design", "references", "bubbletea-patterns.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tmux-orchestration", "references", "pane-orchestration.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "golang-pro", "references", "concurrency.md"))

	// cli-tools resource files included
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "resources", "ast-grep.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "resources", "comby.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "resources", "difftastic.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "resources", "sd.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "resources", "yq.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "resources", "typos.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "resources", "scc.md"))

	// Results track what was written
	assert.Greater(t, len(results), 0)
	for _, r := range results {
		assert.True(t, r.Created)
	}
}

func TestWriteProjectSkills_SkipsExisting(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "tui-design")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	customFile := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(customFile, []byte("custom"), 0o644))

	_, err := WriteProjectSkills(dir, false) // force=false
	require.NoError(t, err)

	content, err := os.ReadFile(customFile)
	require.NoError(t, err)
	assert.Equal(t, "custom", string(content))
}

func TestWriteProjectSkills_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "tui-design")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	customFile := filepath.Join(skillDir, "SKILL.md")
	require.NoError(t, os.WriteFile(customFile, []byte("old"), 0o644))

	_, err := WriteProjectSkills(dir, true) // force=true
	require.NoError(t, err)

	content, err := os.ReadFile(customFile)
	require.NoError(t, err)
	assert.NotEqual(t, "old", string(content))
}

func TestSymlinkHarnessSkills(t *testing.T) {
	dir := t.TempDir()

	// Create canonical skill dirs (simulating WriteProjectSkills already ran)
	for _, name := range []string{"golang-pro", "tui-design", "tmux-orchestration"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", name), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, ".agents", "skills", name, "SKILL.md"),
			[]byte("test"), 0o644))
	}

	// Symlink for claude
	err := SymlinkHarnessSkills(dir, "claude")
	require.NoError(t, err)

	for _, name := range []string{"golang-pro", "tui-design", "tmux-orchestration"} {
		link := filepath.Join(dir, ".claude", "skills", name)
		target, err := os.Readlink(link)
		require.NoError(t, err, "skill %s should be symlinked", name)
		assert.Equal(t, filepath.Join("..", "..", ".agents", "skills", name), target)

		// Symlink should resolve to actual content
		content, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
		require.NoError(t, err)
		assert.Equal(t, "test", string(content))
	}

	// Symlink for opencode
	err = SymlinkHarnessSkills(dir, "opencode")
	require.NoError(t, err)

	for _, name := range []string{"golang-pro", "tui-design", "tmux-orchestration"} {
		link := filepath.Join(dir, ".opencode", "skills", name)
		_, err := os.Readlink(link)
		require.NoError(t, err, "skill %s should be symlinked for opencode", name)
	}
}

func TestSymlinkHarnessSkills_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()

	// Create canonical
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", "tui-design"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".agents", "skills", "tui-design", "SKILL.md"),
		[]byte("new"), 0o644))

	// Create stale symlink
	skillsDir := filepath.Join(dir, ".claude", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.Symlink("/nonexistent", filepath.Join(skillsDir, "tui-design")))

	err := SymlinkHarnessSkills(dir, "claude")
	require.NoError(t, err)

	// Should have replaced the stale symlink
	content, err := os.ReadFile(filepath.Join(skillsDir, "tui-design", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
}

func TestScaffoldAll_IncludesSkills(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Enabled: true},
	}

	results, err := ScaffoldAll(dir, agents, allTools, false)
	require.NoError(t, err)

	// Skills written to canonical location
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tui-design", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "tmux-orchestration", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "golang-pro", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "SKILL.md"))

	// Symlinks created for each active harness
	for _, h := range []string{"claude", "opencode"} {
		link := filepath.Join(dir, "."+h, "skills", "tui-design")
		_, err := os.Readlink(link)
		assert.NoError(t, err, "%s should have tui-design symlink", h)
	}

	// Codex not scaffolded (no codex agent), so no codex symlinks
	assert.NoFileExists(t, filepath.Join(dir, ".codex", "skills"))

	// Results include skill files
	var skillResults int
	for _, r := range results {
		if strings.HasPrefix(filepath.ToSlash(r.Path), ".agents/skills/") {
			skillResults++
		}
	}
	assert.Greater(t, skillResults, 0)
}

func TestWriteOpenCodeProject_GeneratesConfig(t *testing.T) {
	dir := t.TempDir()
	temp := 0.1
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: &temp, Effort: "medium", Enabled: true},
		{Role: "planner", Harness: "opencode", Model: "anthropic/claude-opus-4-6", Temperature: ptrFloat(0.5), Effort: "max", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "openai/gpt-5.3-codex", Temperature: ptrFloat(0.2), Effort: "xhigh", Enabled: true},
	}

	results, err := WriteOpenCodeProject(dir, agents, allTools, false)
	require.NoError(t, err)

	// Config file created
	configPath := filepath.Join(dir, ".opencode", "opencode.jsonc")
	assert.FileExists(t, configPath)

	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	s := string(content)

	// Schema present
	assert.Contains(t, s, `"$schema": "https://opencode.ai/config.json"`)

	// Disabled built-in agents
	assert.Contains(t, s, `"build"`)
	assert.Contains(t, s, `"plan"`)
	assert.Contains(t, s, `"disable": true`)

	// Chat agent with fixed defaults
	assert.Contains(t, s, `"chat"`)
	assert.Contains(t, s, `"anthropic/claude-sonnet-4-6"`)

	// Wizard-configured agents have correct models
	assert.Contains(t, s, `"anthropic/claude-opus-4-6"`)
	assert.Contains(t, s, `"openai/gpt-5.3-codex"`)

	// Temperature rendered as bare numbers (no quotes)
	assert.Contains(t, s, "0.1")
	assert.Contains(t, s, "0.5")
	assert.Contains(t, s, "0.2")

	// Effort values present
	assert.Contains(t, s, `"reasoningEffort": "medium"`)
	assert.Contains(t, s, `"reasoningEffort": "max"`)
	assert.Contains(t, s, `"reasoningEffort": "xhigh"`)

	// No raw placeholders left
	assert.NotContains(t, s, "{{")
	assert.NotContains(t, s, "}}")

	// Dynamic paths resolved (home dir and project dir)
	homeDir, _ := os.UserHomeDir()
	assert.Contains(t, s, homeDir)
	assert.Contains(t, s, dir)

	// Config is in the results list
	var found bool
	for _, r := range results {
		if r.Path == ".opencode/opencode.jsonc" {
			found = true
			assert.True(t, r.Created)
		}
	}
	assert.True(t, found, "opencode.jsonc should be in results")
}

func TestWriteOpenCodeProject_NoEffort(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: ptrFloat(0.1), Effort: "", Enabled: true},
	}

	_, err := WriteOpenCodeProject(dir, agents, nil, false)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".opencode", "opencode.jsonc"))
	require.NoError(t, err)
	s := string(content)

	// Coder block should NOT have reasoningEffort line
	coderIdx := strings.Index(s, `"coder"`)
	require.Greater(t, coderIdx, 0)
	// Look at the next ~500 chars after "coder" for the effort line
	coderSection := s[coderIdx:min(coderIdx+500, len(s))]
	assert.NotContains(t, coderSection, "reasoningEffort")
}

func TestWriteOpenCodeProject_NoTemp(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: nil, Effort: "medium", Enabled: true},
	}

	_, err := WriteOpenCodeProject(dir, agents, nil, false)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".opencode", "opencode.jsonc"))
	require.NoError(t, err)
	s := string(content)

	// Coder block should NOT have temperature line
	coderIdx := strings.Index(s, `"coder"`)
	require.Greater(t, coderIdx, 0)
	coderSection := s[coderIdx:min(coderIdx+500, len(s))]
	assert.NotContains(t, coderSection, "temperature")
}

func TestWriteOpenCodeProject_SkipsNonOpencodeAgents(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: ptrFloat(0.1), Effort: "medium", Enabled: true},
		{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
	}

	_, err := WriteOpenCodeProject(dir, agents, nil, false)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".opencode", "opencode.jsonc"))
	require.NoError(t, err)
	s := string(content)

	// Coder block present (opencode harness)
	assert.Contains(t, s, `"coder"`)
	assert.Contains(t, s, `"anthropic/claude-sonnet-4-6"`)

	// Reviewer block removed (claude harness, not opencode)
	assert.NotContains(t, s, `"reviewer"`)
	assert.NotContains(t, s, `"claude-opus-4-6"`)
}

func ptrFloat(f float64) *float64 { return &f }
