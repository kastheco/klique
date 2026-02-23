package planfsm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanSignals_ParsesValidSentinels(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-2026-02-22-foo.md"),
		nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "review-changes-2026-02-22-bar.md"),
		[]byte("fix the tests"), 0o644))

	signals := ScanSignals(dir)
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
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "garbage-file.txt"),
		nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, ".hidden"),
		nil, 0o644))

	signals := ScanSignals(dir)
	assert.Empty(t, signals)
}

func TestScanSignals_EmptyDirReturnsNil(t *testing.T) {
	dir := t.TempDir()
	signals := ScanSignals(dir)
	assert.Nil(t, signals)
}

func TestScanSignals_RejectsUserOnlyEvents(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// An agent trying to drop a cancel sentinel — should be ignored
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "cancel-2026-02-22-foo.md"),
		nil, 0o644))

	signals := ScanSignals(dir)
	assert.Empty(t, signals)
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
