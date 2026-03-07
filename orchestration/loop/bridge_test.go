package loop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBridgeFilesystemSignals(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, "planner-finished-test-plan"), []byte("plan body"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, "implement-task-finished-w1-t2-test-plan"), nil, 0o644))

	gw := newTestGateway(t)
	n, err := BridgeFilesystemSignals(gw, "proj", dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	entries, err := os.ReadDir(signalsDir)
	require.NoError(t, err)
	assert.Empty(t, entries)

	pending, err := gw.List("proj", taskstore.SignalPending)
	require.NoError(t, err)
	require.Len(t, pending, 2)
}

func TestBridgeFilesystemSignals_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	gw := newTestGateway(t)
	n, err := BridgeFilesystemSignals(gw, "proj", dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}
