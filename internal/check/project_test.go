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

	// Create .agents/skills/ with kasmos-coder and kasmos-planner (not all skills)
	agentsSkills := filepath.Join(dir, ".agents", "skills")
	for _, name := range []string{"kasmos-coder", "kasmos-planner"} {
		skillDir := filepath.Join(agentsSkills, name)
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("test"), 0o644))
	}

	// Create .claude/skills/ with kasmos-coder symlinked
	claudeSkills := filepath.Join(dir, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	coderTarget := filepath.Join("..", "..", ".agents", "skills", "kasmos-coder")
	require.NoError(t, os.Symlink(coderTarget, filepath.Join(claudeSkills, "kasmos-coder")))

	// .opencode/skills/ is empty (no symlinks)
	opencodeSkills := filepath.Join(dir, ".opencode", "skills")
	require.NoError(t, os.MkdirAll(opencodeSkills, 0o755))

	results := AuditProject(dir, []string{"claude", "opencode"})

	byName := make(map[string]ProjectSkillEntry)
	for _, r := range results {
		byName[r.Name] = r
	}

	// kasmos-coder: in canonical, claude synced, opencode missing
	coder := byName["kasmos-coder"]
	assert.True(t, coder.InCanonical, "kasmos-coder should be in canonical")
	assert.Equal(t, StatusSynced, coder.HarnessStatus["claude"], "kasmos-coder: claude should be synced")
	assert.Equal(t, StatusMissing, coder.HarnessStatus["opencode"], "kasmos-coder: opencode should be missing")

	// kasmos-planner: in canonical, both missing (no symlinks created)
	planner := byName["kasmos-planner"]
	assert.True(t, planner.InCanonical, "kasmos-planner should be in canonical")
	assert.Equal(t, StatusMissing, planner.HarnessStatus["claude"], "kasmos-planner: claude should be missing")
	assert.Equal(t, StatusMissing, planner.HarnessStatus["opencode"], "kasmos-planner: opencode should be missing")

	// kasmos-reviewer: not in canonical (missing from .agents/skills/)
	reviewer := byName["kasmos-reviewer"]
	assert.False(t, reviewer.InCanonical, "kasmos-reviewer should not be in canonical")
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
	skillDir := filepath.Join(agentsSkills, "kasmos-coder")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	claudeSkills := filepath.Join(dir, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	// Broken symlink
	require.NoError(t, os.Symlink("/nonexistent", filepath.Join(claudeSkills, "kasmos-coder")))

	results := AuditProject(dir, []string{"claude"})

	byName := make(map[string]ProjectSkillEntry)
	for _, r := range results {
		byName[r.Name] = r
	}

	coder := byName["kasmos-coder"]
	assert.True(t, coder.InCanonical)
	assert.Equal(t, StatusBroken, coder.HarnessStatus["claude"], "kasmos-coder: claude should be broken")
}
