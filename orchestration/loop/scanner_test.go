package loop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanAllSignals(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write a planner-finished signal
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-test-plan.md"),
		nil, 0o644,
	))

	result := ScanAllSignals(dir, nil)
	assert.Len(t, result.FSMSignals, 1)
	assert.Equal(t, "test-plan.md", result.FSMSignals[0].TaskFile)
}

func TestScanAllSignals_IncludesWorktrees(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	wtDir := filepath.Join(dir, "worktrees", "wt1")
	wtSignals := filepath.Join(wtDir, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(wtSignals, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(wtSignals, "implement-finished-wt-plan.md"),
		nil, 0o644,
	))

	worktreePaths := []string{wtDir}
	result := ScanAllSignals(dir, worktreePaths)
	assert.Len(t, result.FSMSignals, 1)
	assert.Equal(t, "wt-plan.md", result.FSMSignals[0].TaskFile)
}
