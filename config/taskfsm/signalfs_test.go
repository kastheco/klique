package taskfsm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureSignalDirs_CreatesSubdirectories(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".kasmos", "signals")

	require.NoError(t, EnsureSignalDirs(baseDir))

	for _, sub := range []string{"", StagingDir, ProcessingDir, FailedDir} {
		dir := filepath.Join(baseDir, sub)
		info, err := os.Stat(dir)
		require.NoError(t, err, "expected dir to exist: %s", dir)
		assert.True(t, info.IsDir(), "expected %s to be a directory", dir)
	}
}

func TestAtomicWrite_WritesViaStagingAndLeavesNoResidue(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".kasmos", "signals")

	filename := "implement-task-finished-w1-t1-test"
	require.NoError(t, AtomicWrite(baseDir, filename, "hello world"))

	// Final file exists
	finalPath := filepath.Join(baseDir, filename)
	data, err := os.ReadFile(finalPath)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))

	// Staging file is gone
	stagingPath := filepath.Join(baseDir, StagingDir, filename)
	_, err = os.Stat(stagingPath)
	assert.True(t, os.IsNotExist(err), "staging file should not exist after atomic write")
}

func TestAtomicWrite_AllowsEmptyBody(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".kasmos", "signals")

	filename := "planner-finished-myplan"
	require.NoError(t, AtomicWrite(baseDir, filename, ""))

	finalPath := filepath.Join(baseDir, filename)
	data, err := os.ReadFile(finalPath)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestBeginProcessing_MovesFileIntoProcessingDir(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, EnsureSignalDirs(baseDir))

	filename := "implement-task-finished-w1-t2-test"
	src := filepath.Join(baseDir, filename)
	require.NoError(t, os.WriteFile(src, []byte("body"), 0o644))

	processingPath, err := BeginProcessing(baseDir, filename)
	require.NoError(t, err)

	// Source should be gone
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source file should be gone after BeginProcessing")

	// File should be in processing dir
	expectedDst := filepath.Join(baseDir, ProcessingDir, filename)
	assert.Equal(t, expectedDst, processingPath)
	_, err = os.Stat(processingPath)
	assert.NoError(t, err, "processing file should exist")
}

func TestCompleteProcessing_RemovesProcessingFile(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, EnsureSignalDirs(baseDir))

	filename := "implement-task-finished-w1-t3-test"
	processingPath := filepath.Join(baseDir, ProcessingDir, filename)
	require.NoError(t, os.WriteFile(processingPath, []byte("body"), 0o644))

	CompleteProcessing(processingPath)

	_, err := os.Stat(processingPath)
	assert.True(t, os.IsNotExist(err), "processing file should be removed after CompleteProcessing")
}

func TestFailProcessing_MovesSignalToFailedAndWritesReason(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, EnsureSignalDirs(baseDir))

	filename := "implement-task-finished-w1-t4-test"
	processingPath := filepath.Join(baseDir, ProcessingDir, filename)
	require.NoError(t, os.WriteFile(processingPath, []byte("body"), 0o644))

	FailProcessing(baseDir, filename, "something went wrong")

	// Processing file gone
	_, err := os.Stat(processingPath)
	assert.True(t, os.IsNotExist(err), "processing file should be gone after FailProcessing")

	// Failed file exists
	failedPath := filepath.Join(baseDir, FailedDir, filename)
	_, err = os.Stat(failedPath)
	assert.NoError(t, err, "failed file should exist")

	// Reason file exists and contains the reason
	reasonPath := failedPath + ".reason"
	data, err := os.ReadFile(reasonPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "something went wrong")
}

func TestRecoverInFlight_MovesSignalsBackToBaseDir(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, EnsureSignalDirs(baseDir))

	// Place two files in processing dir simulating a crash
	filenames := []string{
		"implement-task-finished-w1-t1-alpha",
		"implement-task-finished-w1-t2-beta",
	}
	for _, fn := range filenames {
		p := filepath.Join(baseDir, ProcessingDir, fn)
		require.NoError(t, os.WriteFile(p, nil, 0o644))
	}

	count := RecoverInFlight(baseDir)
	assert.Equal(t, 2, count)

	// Both files back in base dir
	for _, fn := range filenames {
		recovered := filepath.Join(baseDir, fn)
		_, err := os.Stat(recovered)
		assert.NoError(t, err, "recovered file should be in baseDir: %s", fn)

		// Processing entry gone
		inProcessing := filepath.Join(baseDir, ProcessingDir, fn)
		_, err = os.Stat(inProcessing)
		assert.True(t, os.IsNotExist(err), "file should be gone from processing: %s", fn)
	}
}

func TestRecoverInFlight_SkipsWhenNewerSignalAlreadyInBaseDir(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, EnsureSignalDirs(baseDir))

	filename := "implement-task-finished-w1-t1-alpha"

	// Place stale copy in processing dir (simulating interrupted previous run).
	stalePath := filepath.Join(baseDir, ProcessingDir, filename)
	require.NoError(t, os.WriteFile(stalePath, []byte("stale"), 0o644))

	// Place newer signal with the same name directly in the base dir.
	newerPath := filepath.Join(baseDir, filename)
	require.NoError(t, os.WriteFile(newerPath, []byte("newer"), 0o644))

	count := RecoverInFlight(baseDir)
	// Stale in-flight file should be discarded, not counted as recovered.
	assert.Equal(t, 0, count)

	// Newer signal must be untouched.
	content, err := os.ReadFile(newerPath)
	require.NoError(t, err)
	assert.Equal(t, "newer", string(content))

	// Stale processing file must be removed.
	_, err = os.Stat(stalePath)
	assert.True(t, os.IsNotExist(err), "stale processing file should be removed")
}

func TestSignalPathAccessors(t *testing.T) {
	t.Run("Signal", func(t *testing.T) {
		s := Signal{filePath: "/repo/.kasmos/signals/planner-finished-foo"}
		assert.Equal(t, "planner-finished-foo", s.Filename())
		assert.Equal(t, "/repo/.kasmos/signals", s.Dir())
	})

	t.Run("TaskSignal", func(t *testing.T) {
		s := TaskSignal{filePath: "/repo/.kasmos/signals/implement-task-finished-w1-t1-bar"}
		assert.Equal(t, "implement-task-finished-w1-t1-bar", s.Filename())
		assert.Equal(t, "/repo/.kasmos/signals", s.Dir())
	})

	t.Run("WaveSignal", func(t *testing.T) {
		s := WaveSignal{filePath: "/repo/.kasmos/signals/implement-wave-2-baz"}
		assert.Equal(t, "implement-wave-2-baz", s.Filename())
		assert.Equal(t, "/repo/.kasmos/signals", s.Dir())
	})

	t.Run("ElaborationSignal", func(t *testing.T) {
		s := ElaborationSignal{filePath: "/repo/.kasmos/signals/elaborator-finished-qux"}
		assert.Equal(t, "elaborator-finished-qux", s.Filename())
		assert.Equal(t, "/repo/.kasmos/signals", s.Dir())
	})
}
