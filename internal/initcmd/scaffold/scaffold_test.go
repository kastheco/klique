package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/internal/initcmd/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertValidJSON strips JSONC-style line comments and asserts the result is valid JSON.
func assertValidJSON(t *testing.T, content string) {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		lines = append(lines, line)
	}
	var parsed interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.Join(lines, "\n")), &parsed),
		"rendered opencode.jsonc must be valid JSON:\n%s", content)
}

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
		{Role: "chat", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Enabled: true},
	}

	_, err := WriteOpenCodeProject(dir, agents, allTools, false)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".opencode", "agents", "coder.md"))
	assert.FileExists(t, filepath.Join(dir, ".opencode", "agents", "chat.md"))
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

	// Result correctly shows skipped for the coder agent
	require.GreaterOrEqual(t, len(results), 1)
	coderResult := results[0]
	assert.False(t, coderResult.Created)
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
	require.GreaterOrEqual(t, len(results), 1)
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
		{Role: "fixer", Harness: "claude", Model: "claude-sonnet-4-6", Enabled: true},
		{Role: "reviewer", Harness: "opencode", Model: "anthropic/claude-opus-4-6", Enabled: true},
	}

	results, err := WriteClaudeProject(dir, agents, allTools, false)
	require.NoError(t, err)

	// Only coder.md and fixer.md created (claude only — no opencode reviewer)
	assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "coder.md"))
	assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "fixer.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".opencode", "agents", "reviewer.md"))
	require.GreaterOrEqual(t, len(results), 1)
	// First result is the per-role coder agent
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

	// Generic project skills written (including cli-tools)
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "kasmos-fixer", "SKILL.md"))

	fixerSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "kasmos-fixer", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(fixerSkill), "Scaffolding System Protocol (always before editing skills/agent commands)")

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
	skillDir := filepath.Join(dir, ".agents", "skills", "cli-tools")
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
	skillDir := filepath.Join(dir, ".agents", "skills", "cli-tools")
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
	for _, name := range []string{"cli-tools", "writing-plans", "executing-plans"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", name), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, ".agents", "skills", name, "SKILL.md"),
			[]byte("test"), 0o644))
	}

	// Symlink for claude
	err := SymlinkHarnessSkills(dir, "claude")
	require.NoError(t, err)

	for _, name := range []string{"cli-tools", "writing-plans", "executing-plans"} {
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

	for _, name := range []string{"cli-tools", "writing-plans", "executing-plans"} {
		link := filepath.Join(dir, ".opencode", "skills", name)
		_, err := os.Readlink(link)
		require.NoError(t, err, "skill %s should be symlinked for opencode", name)
	}
}

func TestSymlinkHarnessSkills_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()

	// Create canonical
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", "cli-tools"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".agents", "skills", "cli-tools", "SKILL.md"),
		[]byte("new"), 0o644))

	// Create stale symlink
	skillsDir := filepath.Join(dir, ".claude", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.Symlink("/nonexistent", filepath.Join(skillsDir, "cli-tools")))

	err := SymlinkHarnessSkills(dir, "claude")
	require.NoError(t, err)

	// Should have replaced the stale symlink
	content, err := os.ReadFile(filepath.Join(skillsDir, "cli-tools", "SKILL.md"))
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
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "cli-tools", "SKILL.md"))

	// Symlinks created for each active harness
	for _, h := range []string{"claude", "opencode"} {
		link := filepath.Join(dir, "."+h, "skills", "cli-tools")
		_, err := os.Readlink(link)
		assert.NoError(t, err, "%s should have cli-tools symlink", h)
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

	// Output must be valid JSON
	assertValidJSON(t, s)

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

func TestWriteOpenCodeProject_ValidJSONC_OnlyCoder(t *testing.T) {
	// Regression: when planner+reviewer are removed (non-opencode harness),
	// the preceding coder block must not have a trailing comma.
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: ptrFloat(0.1), Effort: "medium", Enabled: true},
		{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
	}
	_, err := WriteOpenCodeProject(dir, agents, nil, false)
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(dir, ".opencode", "opencode.jsonc"))
	require.NoError(t, err)
	assertValidJSON(t, string(content))
}

func TestWriteOpenCodeProject_ValidJSONC_NoWizardAgents(t *testing.T) {
	// Regression: when all three wizard roles are removed (none use opencode harness),
	// only chat+build+plan remain and the output must still be valid JSON.
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "claude", Model: "claude-opus-4-6", Enabled: true},
	}
	_, err := WriteOpenCodeProject(dir, agents, nil, false)
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(dir, ".opencode", "opencode.jsonc"))
	require.NoError(t, err)
	assertValidJSON(t, string(content))
}

// TestWriteOpenCodeProject_IncludesNonOpencodeAgents verifies that agent roles
// configured for a different harness (e.g. claude) are still written to
// opencode.jsonc. Kasmos controls which harness is used at orchestration time;
// opencode.jsonc just needs the block present so the agent is defined.
func TestWriteOpenCodeProject_IncludesNonOpencodeAgents(t *testing.T) {
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: ptrFloat(0.1), Effort: "medium", Enabled: true},
		{Role: "reviewer", Harness: "claude", Model: "claude-opus-4-6", Temperature: ptrFloat(0.2), Effort: "medium", Enabled: true},
	}

	_, err := WriteOpenCodeProject(dir, agents, nil, false)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".opencode", "opencode.jsonc"))
	require.NoError(t, err)
	s := string(content)

	// Coder block present (opencode harness)
	assert.Contains(t, s, `"coder"`)
	assert.Contains(t, s, `"anthropic/claude-sonnet-4-6"`)

	// Reviewer block also present even though harness is claude
	assert.Contains(t, s, `"reviewer"`)
	assert.Contains(t, s, `"anthropic/claude-opus-4-6"`)
}

func TestRun_OpencodeConfigGenerated(t *testing.T) {
	// This is a scaffold-level check — the integration test in initcmd_test.go
	// already tests the TOML write path. Just verify the config file shows up
	// in ScaffoldAll results when opencode agents are present.
	dir := t.TempDir()
	agents := []harness.AgentConfig{
		{Role: "coder", Harness: "opencode", Model: "anthropic/claude-sonnet-4-6",
			Temperature: ptrFloat(0.1), Effort: "medium", Enabled: true},
	}

	results, err := ScaffoldAll(dir, agents, nil, false)
	require.NoError(t, err)

	var hasConfig bool
	for _, r := range results {
		if r.Path == ".opencode/opencode.jsonc" {
			hasConfig = true
		}
	}
	assert.True(t, hasConfig, "ScaffoldAll should produce opencode.jsonc for opencode agents")
}

func TestEnsureRuntimeDirs_CreatesAll(t *testing.T) {
	dir := t.TempDir()
	results, err := EnsureRuntimeDirs(dir)
	require.NoError(t, err)

	// All dirs should be created on first run.
	assert.Len(t, results, len(runtimeDirs), "should create all runtime dirs")

	for _, rel := range runtimeDirs {
		info, err := os.Stat(filepath.Join(dir, rel))
		require.NoError(t, err, "dir %s must exist", rel)
		assert.True(t, info.IsDir(), "%s must be a directory", rel)
	}
}

func TestEnsureRuntimeDirs_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// First run creates dirs.
	_, err := EnsureRuntimeDirs(dir)
	require.NoError(t, err)

	// Second run should create nothing new.
	results, err := EnsureRuntimeDirs(dir)
	require.NoError(t, err)
	assert.Empty(t, results, "second run should not create any dirs")
}

func TestScaffold_IncludesFixerAgent(t *testing.T) {
	dir := t.TempDir()
	temp := 0.1
	agents := []harness.AgentConfig{
		{Harness: "opencode", Role: "coder", Model: "anthropic/claude-sonnet-4-6", Temperature: &temp, Effort: "medium", Enabled: true},
		{Harness: "opencode", Role: "fixer", Model: "anthropic/claude-sonnet-4-6", Temperature: &temp, Effort: "low", Enabled: true},
	}
	results, err := WriteOpenCodeProject(dir, agents, nil, true)
	require.NoError(t, err)

	// Fixer agent is now wizard-managed: scaffolded when included in agents list
	fixerPath := filepath.Join(dir, ".opencode", "agents", "fixer.md")
	assert.FileExists(t, fixerPath)

	content, err := os.ReadFile(fixerPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "fixer")
	assert.Contains(t, string(content), "Scaffolding System (first step for skills/agent commands)")

	// Check opencode.jsonc includes fixer block
	var foundConfig bool
	for _, r := range results {
		if strings.Contains(r.Path, "opencode.jsonc") {
			foundConfig = true
		}
	}
	assert.True(t, foundConfig)
}

func ptrFloat(f float64) *float64 { return &f }
