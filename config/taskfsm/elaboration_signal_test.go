package taskfsm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseElaborationSignal(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantOk   bool
		wantFile string
	}{
		{
			name:     "valid elaboration signal",
			filename: "elaborator-finished-my-feature.md",
			wantOk:   true,
			wantFile: "my-feature.md",
		},
		{
			name:     "not an elaboration signal",
			filename: "planner-finished-test.md",
			wantOk:   false,
		},
		{
			name:     "empty plan file",
			filename: "elaborator-finished-",
			wantOk:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig, ok := ParseElaborationSignal(tt.filename)
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.wantFile, sig.TaskFile)
			}
		})
	}
}

func TestScanElaborationSignals(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write an elaboration signal and a non-matching file
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "elaborator-finished-test.md"), nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-test.md"), nil, 0o644))

	signals := ScanElaborationSignals(signalsDir)
	require.Len(t, signals, 1)
	assert.Equal(t, "test.md", signals[0].TaskFile)
}

func TestConsumeElaborationSignal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "elaborator-finished-test.md")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	sig := ElaborationSignal{TaskFile: "test.md", filePath: path}
	ConsumeElaborationSignal(sig)

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}
