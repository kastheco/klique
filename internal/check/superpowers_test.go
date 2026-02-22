package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditSuperpowers_OpenCode_Installed(t *testing.T) {
	home := t.TempDir()

	// Set up ~/.config/opencode/superpowers/.git (repo cloned)
	repoDir := filepath.Join(home, ".config", "opencode", "superpowers")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))

	// Set up ~/.config/opencode/plugins/superpowers.js as a valid symlink
	pluginDir := filepath.Join(home, ".config", "opencode", "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	// Create the actual file the symlink points to
	pluginSrc := filepath.Join(repoDir, ".opencode", "plugins", "superpowers.js")
	require.NoError(t, os.MkdirAll(filepath.Dir(pluginSrc), 0o755))
	require.NoError(t, os.WriteFile(pluginSrc, []byte("// plugin"), 0o644))
	pluginLink := filepath.Join(pluginDir, "superpowers.js")
	require.NoError(t, os.Symlink(pluginSrc, pluginLink))

	results := AuditSuperpowers(home, []string{"opencode"})

	require.Len(t, results, 1)
	assert.Equal(t, "opencode", results[0].Name)
	assert.True(t, results[0].Installed, "opencode superpowers should be installed")
}

func TestAuditSuperpowers_OpenCode_MissingRepo(t *testing.T) {
	home := t.TempDir()
	// No repo dir at all

	results := AuditSuperpowers(home, []string{"opencode"})

	require.Len(t, results, 1)
	assert.Equal(t, "opencode", results[0].Name)
	assert.False(t, results[0].Installed, "opencode superpowers should not be installed (no repo)")
	assert.Contains(t, results[0].Detail, "repo")
}

func TestAuditSuperpowers_OpenCode_MissingPlugin(t *testing.T) {
	home := t.TempDir()

	// Repo exists but plugin symlink is missing
	repoDir := filepath.Join(home, ".config", "opencode", "superpowers")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))

	results := AuditSuperpowers(home, []string{"opencode"})

	require.Len(t, results, 1)
	assert.False(t, results[0].Installed)
	assert.Contains(t, results[0].Detail, "plugin")
}

func TestAuditSuperpowers_OpenCode_BrokenPluginSymlink(t *testing.T) {
	home := t.TempDir()

	repoDir := filepath.Join(home, ".config", "opencode", "superpowers")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))

	pluginDir := filepath.Join(home, ".config", "opencode", "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	// Broken symlink
	require.NoError(t, os.Symlink("/nonexistent/superpowers.js", filepath.Join(pluginDir, "superpowers.js")))

	results := AuditSuperpowers(home, []string{"opencode"})

	require.Len(t, results, 1)
	assert.False(t, results[0].Installed)
	assert.Contains(t, results[0].Detail, "plugin")
}

func TestAuditSuperpowers_Codex_Skipped(t *testing.T) {
	home := t.TempDir()

	results := AuditSuperpowers(home, []string{"codex"})

	// codex has no superpowers concept — should be omitted from results
	assert.Empty(t, results)
}

func TestAuditSuperpowers_MultipleHarnesses(t *testing.T) {
	home := t.TempDir()

	// opencode fully installed
	repoDir := filepath.Join(home, ".config", "opencode", "superpowers")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))
	pluginSrc := filepath.Join(repoDir, ".opencode", "plugins", "superpowers.js")
	require.NoError(t, os.MkdirAll(filepath.Dir(pluginSrc), 0o755))
	require.NoError(t, os.WriteFile(pluginSrc, []byte("// plugin"), 0o644))
	pluginDir := filepath.Join(home, ".config", "opencode", "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.Symlink(pluginSrc, filepath.Join(pluginDir, "superpowers.js")))

	// claude: not installed (no claude binary in test env, will be detected as not found)
	results := AuditSuperpowers(home, []string{"claude", "opencode", "codex"})

	// codex should be skipped, so we get claude + opencode
	assert.Len(t, results, 2)

	byName := make(map[string]SuperpowersResult)
	for _, r := range results {
		byName[r.Name] = r
	}

	assert.True(t, byName["opencode"].Installed)
	// claude result exists (either installed or not, depending on env)
	_, hasClaude := byName["claude"]
	assert.True(t, hasClaude)
}
