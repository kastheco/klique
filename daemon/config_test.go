package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultDaemonConfig_PRMonitor(t *testing.T) {
	cfg := defaultDaemonConfig()

	assert.False(t, cfg.PRMonitor.Enabled, "PRMonitor should be disabled by default")
	assert.Equal(t, 60*time.Second, cfg.PRMonitor.PollInterval, "default PollInterval should be 60s")
	assert.Equal(t, []string{"eyes"}, cfg.PRMonitor.Reactions, "default Reactions should be [eyes]")
}

func TestLoadDaemonConfig_MissingFile(t *testing.T) {
	cfg, err := LoadDaemonConfig("/nonexistent/path/daemon.toml")
	require.NoError(t, err, "missing file should return defaults without error")
	require.NotNil(t, cfg)

	// Verify PR monitor defaults are present
	assert.False(t, cfg.PRMonitor.Enabled)
	assert.Equal(t, 60*time.Second, cfg.PRMonitor.PollInterval)
	assert.Equal(t, []string{"eyes"}, cfg.PRMonitor.Reactions)
}

func TestLoadDaemonConfig_PRMonitorSection(t *testing.T) {
	toml := `
[pr_monitor]
enabled = true
poll_interval_sec = 120.0
reactions = ["eyes", "+1"]
`
	cfg := loadFromString(t, toml)

	assert.True(t, cfg.PRMonitor.Enabled)
	assert.Equal(t, 120*time.Second, cfg.PRMonitor.PollInterval)
	assert.Equal(t, []string{"eyes", "+1"}, cfg.PRMonitor.Reactions)
}

func TestLoadDaemonConfig_PRMonitorZeroInterval(t *testing.T) {
	toml := `
[pr_monitor]
enabled = true
poll_interval_sec = 0
`
	cfg := loadFromString(t, toml)

	// Zero interval should fall back to the 60s default.
	assert.Equal(t, 60*time.Second, cfg.PRMonitor.PollInterval)
}

func TestLoadDaemonConfig_PRMonitorNegativeInterval(t *testing.T) {
	toml := `
[pr_monitor]
enabled = true
poll_interval_sec = -5
`
	cfg := loadFromString(t, toml)

	// Negative interval should fall back to the 60s default.
	assert.Equal(t, 60*time.Second, cfg.PRMonitor.PollInterval)
}

func TestLoadDaemonConfig_PRMonitorEmptyReactions(t *testing.T) {
	toml := `
[pr_monitor]
enabled = true
reactions = []
`
	cfg := loadFromString(t, toml)

	// Empty reactions list should fall back to default.
	assert.Equal(t, []string{"eyes"}, cfg.PRMonitor.Reactions)
}

func TestLoadDaemonConfig_PRMonitorReactionsWithEmptyStrings(t *testing.T) {
	toml := `
[pr_monitor]
reactions = ["", "rocket", ""]
`
	cfg := loadFromString(t, toml)

	// Empty strings should be trimmed; remaining entries kept.
	assert.Equal(t, []string{"rocket"}, cfg.PRMonitor.Reactions)
}

func TestLoadDaemonConfig_PRMonitorReactionsAllEmpty(t *testing.T) {
	toml := `
[pr_monitor]
reactions = ["", ""]
`
	cfg := loadFromString(t, toml)

	// All-empty slice should fall back to default.
	assert.Equal(t, []string{"eyes"}, cfg.PRMonitor.Reactions)
}

func TestLoadDaemonConfig_PRMonitorReactionsWhitespaceOnly(t *testing.T) {
	toml := `
[pr_monitor]
reactions = ["", " "]
`
	cfg := loadFromString(t, toml)

	// Whitespace-only strings should be trimmed; empty result falls back to default.
	assert.Equal(t, []string{"eyes"}, cfg.PRMonitor.Reactions)
}

func TestLoadDaemonConfig_PRMonitorReactionsTrimmed(t *testing.T) {
	toml := `
[pr_monitor]
reactions = [" rocket ", " eyes"]
`
	cfg := loadFromString(t, toml)

	// Whitespace should be trimmed from reaction names, preserving order.
	assert.Equal(t, []string{"rocket", "eyes"}, cfg.PRMonitor.Reactions)
}

func TestLoadDaemonConfig_PRMonitorAbsent(t *testing.T) {
	// No [pr_monitor] section — all defaults should be preserved.
	toml := `poll_interval_sec = 5`
	cfg := loadFromString(t, toml)

	assert.False(t, cfg.PRMonitor.Enabled)
	assert.Equal(t, 60*time.Second, cfg.PRMonitor.PollInterval)
	assert.Equal(t, []string{"eyes"}, cfg.PRMonitor.Reactions)
}

func TestLoadDaemonConfig_ExistingFieldsUnchanged(t *testing.T) {
	toml := `
poll_interval_sec = 3
auto_advance = true
auto_advance_waves = true
auto_review_fix = true
max_review_fix_cycles = 5

[pr_monitor]
enabled = true
poll_interval_sec = 90
`
	cfg := loadFromString(t, toml)

	// Existing top-level fields should still work correctly.
	assert.Equal(t, 3*time.Second, cfg.PollInterval)
	assert.True(t, cfg.AutoAdvance)
	assert.True(t, cfg.AutoAdvanceWaves)
	assert.True(t, cfg.AutoReviewFix)
	assert.Equal(t, 5, cfg.MaxReviewFixCycles)

	// PRMonitor fields should also be set correctly.
	assert.True(t, cfg.PRMonitor.Enabled)
	assert.Equal(t, 90*time.Second, cfg.PRMonitor.PollInterval)
}

// loadFromString writes toml content to a temp file and calls LoadDaemonConfig.
func loadFromString(t *testing.T, content string) *DaemonConfig {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.toml")
	err := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, err)

	cfg, err := LoadDaemonConfig(path)
	require.NoError(t, err)
	return cfg
}
