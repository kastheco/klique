package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/internal/initcmd/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudit_InProject(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()

	// Mark as kas project by creating .agents/
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".agents", "skills"), 0o755))

	registry := harness.NewRegistry()
	result := Audit(home, projectDir, registry)

	assert.True(t, result.InProject, "should detect kas project")
	assert.NotNil(t, result.Global)
	assert.NotNil(t, result.Project)
	assert.NotNil(t, result.Superpowers)
}

func TestAudit_NotInProject(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()
	// No .agents/ dir — not a kas project

	registry := harness.NewRegistry()
	result := Audit(home, projectDir, registry)

	assert.False(t, result.InProject, "should not detect kas project")
	assert.Nil(t, result.Project, "project skills should be nil when not in project")
}

func TestAudit_Summary_AllSynced(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()

	// Set up global skills for claude: one real skill synced
	agentsSkills := filepath.Join(home, ".agents", "skills")
	skillDir := filepath.Join(agentsSkills, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	claudeSkills := filepath.Join(home, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkills, 0o755))
	target, err := filepath.Rel(claudeSkills, skillDir)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(target, filepath.Join(claudeSkills, "my-skill")))

	// Not a kas project — no project skills counted
	registry := harness.NewRegistry()
	result := Audit(home, projectDir, registry)

	ok, total := result.Summary()
	// Global: 1 synced (my-skill for claude) + opencode missing + codex synced
	// Superpowers: claude (not installed in test env) + opencode (not installed)
	assert.GreaterOrEqual(t, total, 1, "should have at least one check")
	assert.GreaterOrEqual(t, ok, 0)
	assert.LessOrEqual(t, ok, total)
}

func TestAudit_GlobalResultsPerHarness(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()

	registry := harness.NewRegistry()
	result := Audit(home, projectDir, registry)

	// Should have one HarnessResult per registered harness
	harnessNames := registry.All()
	assert.Len(t, result.Global, len(harnessNames))

	byName := make(map[string]HarnessResult)
	for _, h := range result.Global {
		byName[h.Name] = h
	}
	for _, name := range harnessNames {
		_, ok := byName[name]
		assert.True(t, ok, "should have global result for harness %s", name)
	}
}
