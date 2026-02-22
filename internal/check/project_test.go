package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditProject_SyncedAndMissing(t *testing.T) {
	dir := t.TempDir()

	// Create .agents/skills/ with tui-design and golang-pro
	agentsSkills := filepath.Join(dir, ".agents", "skills")
	for _, name := range []string{"tui-design", "golang-pro"} {
		skillDir := filepath.Join(agentsSkills, name)
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("test"), 0o644))
	}

	// Create .claude/skills/ with tui-design symlinked
	claudeSkills := filepath.Join(dir, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	tuiTarget := filepath.Join("..", "..", ".agents", "skills", "tui-design")
	require.NoError(t, os.Symlink(tuiTarget, filepath.Join(claudeSkills, "tui-design")))

	// .opencode/skills/ is empty (no symlinks)
	opencodeSkills := filepath.Join(dir, ".opencode", "skills")
	require.NoError(t, os.MkdirAll(opencodeSkills, 0o755))

	results := AuditProject(dir, []string{"claude", "opencode"})

	byName := make(map[string]ProjectSkillEntry)
	for _, r := range results {
		byName[r.Name] = r
	}

	// tui-design: in canonical, claude synced, opencode missing
	tui := byName["tui-design"]
	assert.True(t, tui.InCanonical, "tui-design should be in canonical")
	assert.Equal(t, StatusSynced, tui.HarnessStatus["claude"], "tui-design: claude should be synced")
	assert.Equal(t, StatusMissing, tui.HarnessStatus["opencode"], "tui-design: opencode should be missing")

	// golang-pro: in canonical, both missing
	gp := byName["golang-pro"]
	assert.True(t, gp.InCanonical, "golang-pro should be in canonical")
	assert.Equal(t, StatusMissing, gp.HarnessStatus["claude"], "golang-pro: claude should be missing")
	assert.Equal(t, StatusMissing, gp.HarnessStatus["opencode"], "golang-pro: opencode should be missing")

	// cli-tools: not in canonical (missing from .agents/skills/)
	cliTools := byName["cli-tools"]
	assert.False(t, cliTools.InCanonical, "cli-tools should not be in canonical")

	// tmux-orchestration: not in canonical
	tmux := byName["tmux-orchestration"]
	assert.False(t, tmux.InCanonical, "tmux-orchestration should not be in canonical")
}

func TestAuditProject_AllSynced(t *testing.T) {
	dir := t.TempDir()

	agentsSkills := filepath.Join(dir, ".agents", "skills")
	for _, name := range EmbeddedSkillNames {
		skillDir := filepath.Join(agentsSkills, name)
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
	}

	// Symlink all skills for claude
	claudeSkills := filepath.Join(dir, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	for _, name := range EmbeddedSkillNames {
		target := filepath.Join("..", "..", ".agents", "skills", name)
		require.NoError(t, os.Symlink(target, filepath.Join(claudeSkills, name)))
	}

	results := AuditProject(dir, []string{"claude"})

	for _, r := range results {
		assert.True(t, r.InCanonical, "%s should be in canonical", r.Name)
		assert.Equal(t, StatusSynced, r.HarnessStatus["claude"], "%s: claude should be synced", r.Name)
	}
}

func TestAuditProject_BrokenSymlink(t *testing.T) {
	dir := t.TempDir()

	agentsSkills := filepath.Join(dir, ".agents", "skills")
	skillDir := filepath.Join(agentsSkills, "golang-pro")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	claudeSkills := filepath.Join(dir, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	// Broken symlink
	require.NoError(t, os.Symlink("/nonexistent", filepath.Join(claudeSkills, "golang-pro")))

	results := AuditProject(dir, []string{"claude"})

	byName := make(map[string]ProjectSkillEntry)
	for _, r := range results {
		byName[r.Name] = r
	}

	gp := byName["golang-pro"]
	assert.True(t, gp.InCanonical)
	assert.Equal(t, StatusBroken, gp.HarnessStatus["claude"], "golang-pro: claude should be broken")
}
