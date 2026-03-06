package taskfsm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskSignal(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantOK   bool
		wantWave int
		wantTask int
		wantPlan string
	}{
		{
			name:     "valid signal",
			filename: "implement-task-finished-w2-t3-feature.md",
			wantOK:   true,
			wantWave: 2,
			wantTask: 3,
			wantPlan: "feature.md",
		},
		{
			name:     "not a task signal",
			filename: "planner-finished-feature.md",
			wantOK:   false,
		},
		{
			name:     "wave 0 invalid",
			filename: "implement-task-finished-w0-t1-feature.md",
			wantOK:   false,
		},
		{
			name:     "task 0 invalid",
			filename: "implement-task-finished-w1-t0-feature.md",
			wantOK:   false,
		},
		{
			name:     "non-numeric values invalid",
			filename: "implement-task-finished-wx-ty-feature.md",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, ok := ParseTaskSignal(tt.filename)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantWave, ts.WaveNumber)
				assert.Equal(t, tt.wantTask, ts.TaskNumber)
				assert.Equal(t, tt.wantPlan, ts.TaskFile)
			}
		})
	}
}

func TestScanTaskSignals_ParsesAndIgnoresInvalid(t *testing.T) {
	signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, "implement-task-finished-w1-t2-task.md"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, "implement-task-finished-w0-t2-task.md"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, "garbage.txt"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, ".hidden"), nil, 0o644))

	signals := ScanTaskSignals(signalsDir)
	require.Len(t, signals, 1)
	assert.Equal(t, 1, signals[0].WaveNumber)
	assert.Equal(t, 2, signals[0].TaskNumber)
	assert.Equal(t, "task.md", signals[0].TaskFile)
}

func TestTaskSignal_Dedup(t *testing.T) {
	a := TaskSignal{WaveNumber: 1, TaskNumber: 2, TaskFile: "alpha.md"}
	b := TaskSignal{WaveNumber: 1, TaskNumber: 2, TaskFile: "alpha.md"}
	c := TaskSignal{WaveNumber: 2, TaskNumber: 2, TaskFile: "alpha.md"}

	assert.Equal(t, a.Key(), b.Key())
	assert.NotEqual(t, a.Key(), c.Key())
}

func TestConsumeTaskSignal_DeletesFile(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	path := filepath.Join(signalsDir, "implement-task-finished-w1-t2-task.md")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	sig := TaskSignal{WaveNumber: 1, TaskNumber: 2, TaskFile: "task.md", filePath: path}
	ConsumeTaskSignal(sig)

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestScanTaskSignals_EmptyDirReturnsNil(t *testing.T) {
	signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	signals := ScanTaskSignals(signalsDir)
	assert.Nil(t, signals)
}
