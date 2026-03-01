package planfsm

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanSignals_ParsesValidSentinels(t *testing.T) {
	signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-2026-02-22-foo.md"),
		nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "review-changes-2026-02-22-bar.md"),
		[]byte("fix the tests"), 0o644))

	signals := ScanSignals(signalsDir)
	require.Len(t, signals, 2)

	// Sort by plan file for deterministic assertion
	if signals[0].PlanFile > signals[1].PlanFile {
		signals[0], signals[1] = signals[1], signals[0]
	}

	assert.Equal(t, PlannerFinished, signals[1].Event)
	assert.Equal(t, "2026-02-22-foo.md", signals[1].PlanFile)
	assert.Empty(t, signals[1].Body)

	assert.Equal(t, ReviewChangesRequested, signals[0].Event)
	assert.Equal(t, "2026-02-22-bar.md", signals[0].PlanFile)
	assert.Equal(t, "fix the tests", signals[0].Body)
}

func TestScanSignals_IgnoresInvalidFiles(t *testing.T) {
	signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "garbage-file.txt"),
		nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, ".hidden"),
		nil, 0o644))

	signals := ScanSignals(signalsDir)
	assert.Empty(t, signals)
}

func TestScanSignals_EmptyDirReturnsNil(t *testing.T) {
	signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	signals := ScanSignals(signalsDir)
	assert.Nil(t, signals)
}

func TestScanSignals_RejectsUserOnlyEvents(t *testing.T) {
	signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// An agent trying to drop a cancel sentinel — should be ignored
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "cancel-2026-02-22-foo.md"),
		nil, 0o644))

	signals := ScanSignals(signalsDir)
	assert.Empty(t, signals)
}

func TestSignalKey_Dedup(t *testing.T) {
	a := Signal{Event: ReviewChangesRequested, PlanFile: "foo.md"}
	b := Signal{Event: ReviewChangesRequested, PlanFile: "foo.md"}
	c := Signal{Event: ImplementFinished, PlanFile: "foo.md"}

	assert.Equal(t, a.Key(), b.Key(), "same event+planFile should produce same key")
	assert.NotEqual(t, a.Key(), c.Key(), "different events should produce different keys")
}

func TestConsumeSignal_DeletesFile(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	path := filepath.Join(signalsDir, "planner-finished-test.md")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	sig := Signal{Event: PlannerFinished, PlanFile: "test.md", filePath: path}
	ConsumeSignal(sig)

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestScanSignals_KasmosSignalsDir(t *testing.T) {
	signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-test.md"),
		[]byte(""), 0o644,
	))

	signals := ScanSignals(signalsDir)
	require.Len(t, signals, 1)
	assert.Equal(t, PlannerFinished, signals[0].Event)
}

// TestSignals_WithStoreFSM verifies that signals still trigger store-backed FSM transitions.
// The sentinel file system is decoupled from storage — it just triggers FSM events.
func TestSignals_WithStoreFSM(t *testing.T) {
	backend := planstore.NewTestSQLiteStore(t)
	srv := httptest.NewServer(planstore.NewHandler(backend))
	defer srv.Close()

	store := planstore.NewHTTPStore(srv.URL, "test-project")
	err := store.Create("test-project", planstore.PlanEntry{
		Filename: "test.md", Status: "planning",
	})
	require.NoError(t, err)

	// Write a sentinel file in a temp signals dir
	signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, "planner-finished-test.md"), nil, 0o644))

	signals := ScanSignals(signalsDir)
	require.Len(t, signals, 1)
	assert.Equal(t, PlannerFinished, signals[0].Event)

	// Apply via store-backed FSM
	fsm := New(store, "test-project", signalsDir)
	require.NoError(t, fsm.Transition("test.md", signals[0].Event))

	entry, err := store.Get("test-project", "test.md")
	require.NoError(t, err)
	assert.Equal(t, "ready", string(entry.Status))
}
