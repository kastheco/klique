package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditGlobal_SyncedAndSkipped(t *testing.T) {
	home := t.TempDir()

	// Create canonical skills: foo (real dir), bar (real dir), external-skill (symlink)
	agentsSkills := filepath.Join(home, ".agents", "skills")
	for _, name := range []string{"foo", "bar"} {
		dir := filepath.Join(agentsSkills, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("test"), 0o644))
	}
	// external-skill is a symlink (externally managed)
	extRepo := filepath.Join(home, "external-repo")
	require.NoError(t, os.MkdirAll(extRepo, 0o755))
	require.NoError(t, os.Symlink(extRepo, filepath.Join(agentsSkills, "external-skill")))

	// Create harness dir with foo symlinked correctly
	claudeSkills := filepath.Join(home, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	// foo: correct symlink
	fooTarget, err := filepath.Rel(claudeSkills, filepath.Join(agentsSkills, "foo"))
	require.NoError(t, err)
	require.NoError(t, os.Symlink(fooTarget, filepath.Join(claudeSkills, "foo")))
	// stale: orphan in harness dir with no source
	require.NoError(t, os.Symlink("/nonexistent-orphan", filepath.Join(claudeSkills, "stale")))

	result := AuditGlobal(home, "claude")

	assert.Equal(t, "claude", result.Name)

	byName := make(map[string]SkillEntry)
	for _, s := range result.Skills {
		byName[s.Name] = s
	}

	assert.Equal(t, StatusSynced, byName["foo"].Status, "foo should be synced")
	assert.Equal(t, StatusMissing, byName["bar"].Status, "bar should be missing (no link in harness)")
	assert.Equal(t, StatusSkipped, byName["external-skill"].Status, "external-skill should be skipped (symlink source)")
	assert.Equal(t, StatusOrphan, byName["stale"].Status, "stale should be orphan (no source)")
}

func TestAuditGlobal_BrokenSymlink(t *testing.T) {
	home := t.TempDir()

	agentsSkills := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(agentsSkills, "myplugin")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	claudeSkills := filepath.Join(home, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	// Symlink exists but points to nonexistent target
	require.NoError(t, os.Symlink("/nonexistent-target", filepath.Join(claudeSkills, "myplugin")))

	result := AuditGlobal(home, "claude")

	byName := make(map[string]SkillEntry)
	for _, s := range result.Skills {
		byName[s.Name] = s
	}

	assert.Equal(t, StatusBroken, byName["myplugin"].Status, "myplugin should be broken (symlink target missing)")
}

func TestAuditGlobal_NoSkillsDir(t *testing.T) {
	home := t.TempDir()
	// No ~/.agents/skills/ — should return empty result, not error
	result := AuditGlobal(home, "claude")
	assert.Equal(t, "claude", result.Name)
	assert.Empty(t, result.Skills)
}

func TestAuditGlobal_OrphanWithNoCanonical(t *testing.T) {
	home := t.TempDir()
	// No ~/.agents/skills/ but harness dir has a stale symlink
	claudeSkills := filepath.Join(home, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	require.NoError(t, os.Symlink("/nonexistent", filepath.Join(claudeSkills, "stale")))

	result := AuditGlobal(home, "claude")
	byName := make(map[string]SkillEntry)
	for _, s := range result.Skills {
		byName[s.Name] = s
	}
	assert.Equal(t, StatusOrphan, byName["stale"].Status, "stale should be orphan even with no canonical dir")
}

func TestAuditGlobal_Codex(t *testing.T) {
	home := t.TempDir()

	// Codex reads ~/.agents/skills/ natively — all skills are "synced"
	agentsSkills := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(agentsSkills, "foo")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	result := AuditGlobal(home, "codex")
	assert.Equal(t, "codex", result.Name)

	byName := make(map[string]SkillEntry)
	for _, s := range result.Skills {
		byName[s.Name] = s
	}
	assert.Equal(t, StatusSynced, byName["foo"].Status, "codex: all real skills are natively synced")
}
