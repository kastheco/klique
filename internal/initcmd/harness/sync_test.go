package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncGlobalSkills_Claude(t *testing.T) {
	home := t.TempDir()

	// Create canonical skills
	agentsSkills := filepath.Join(home, ".agents", "skills")
	for _, name := range []string{"cli-tools", "my-custom-skill"} {
		dir := filepath.Join(agentsSkills, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("test"), 0o644))
	}

	// Create a symlink skill (simulating an externally-managed skill — should be skipped)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "external-repo", "skills"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(home, "external-repo", "skills"),
		filepath.Join(agentsSkills, "external-skill"),
	))

	err := SyncGlobalSkills(home, "claude")
	require.NoError(t, err)

	// Real skills should be symlinked
	for _, name := range []string{"cli-tools", "my-custom-skill"} {
		link := filepath.Join(home, ".claude", "skills", name)
		target, err := os.Readlink(link)
		require.NoError(t, err, "skill %s should be symlinked", name)
		assert.Equal(t, filepath.Join("..", "..", ".agents", "skills", name), target)
	}

	// externally-managed symlink-source should NOT be propagated
	_, err = os.Readlink(filepath.Join(home, ".claude", "skills", "external-skill"))
	assert.Error(t, err, "symlink-source skills should not be synced")
}

func TestSyncGlobalSkills_OpenCode(t *testing.T) {
	home := t.TempDir()

	agentsSkills := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(agentsSkills, "cli-tools")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("test"), 0o644))

	err := SyncGlobalSkills(home, "opencode")
	require.NoError(t, err)

	link := filepath.Join(home, ".config", "opencode", "skills", "cli-tools")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	// OpenCode uses a different base path, so relative symlink differs
	assert.Contains(t, target, "cli-tools")

	// Symlink should resolve to actual content
	content, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))
}

func TestSyncGlobalSkills_ReplacesStaleSymlinks(t *testing.T) {
	home := t.TempDir()

	agentsSkills := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(agentsSkills, "cli-tools")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("new"), 0o644))

	// Create stale symlink
	claudeSkills := filepath.Join(home, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	require.NoError(t, os.Symlink("/nonexistent", filepath.Join(claudeSkills, "cli-tools")))

	err := SyncGlobalSkills(home, "claude")
	require.NoError(t, err)

	// Should resolve to new content
	content, err := os.ReadFile(filepath.Join(claudeSkills, "cli-tools", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
}

func TestSyncGlobalSkills_SkipsNonSymlinkEntries(t *testing.T) {
	home := t.TempDir()

	agentsSkills := filepath.Join(home, ".agents", "skills")
	dir := filepath.Join(agentsSkills, "cli-tools")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("test"), 0o644))

	// Create a real directory (user-managed) in destination — should not be overwritten
	claudeSkills := filepath.Join(home, ".claude", "skills")
	userManaged := filepath.Join(claudeSkills, "cli-tools")
	require.NoError(t, os.MkdirAll(userManaged, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userManaged, "SKILL.md"), []byte("custom"), 0o644))

	err := SyncGlobalSkills(home, "claude")
	require.NoError(t, err)

	// User-managed dir should be untouched
	content, err := os.ReadFile(filepath.Join(userManaged, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "custom", string(content))
}

func TestSyncGlobalSkills_NoSkillsDir(t *testing.T) {
	home := t.TempDir()

	// No ~/.agents/skills/ — should be a no-op, not an error
	err := SyncGlobalSkills(home, "claude")
	assert.NoError(t, err)
}
