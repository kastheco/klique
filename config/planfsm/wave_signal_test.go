package planfsm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWaveSignal(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantOK   bool
		wantWave int
		wantPlan string
	}{
		{
			name:     "valid wave 1 signal",
			filename: "implement-wave-1-2026-02-20-test-plan.md",
			wantOK:   true,
			wantWave: 1,
			wantPlan: "2026-02-20-test-plan.md",
		},
		{
			name:     "valid wave 3 signal",
			filename: "implement-wave-3-2026-02-20-multi-wave.md",
			wantOK:   true,
			wantWave: 3,
			wantPlan: "2026-02-20-multi-wave.md",
		},
		{
			name:     "not a wave signal",
			filename: "planner-finished-2026-02-20-test-plan.md",
			wantOK:   false,
		},
		{
			name:     "malformed wave number",
			filename: "implement-wave-abc-2026-02-20-test-plan.md",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws, ok := ParseWaveSignal(tt.filename)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantWave, ws.WaveNumber)
				assert.Equal(t, tt.wantPlan, ws.PlanFile)
			}
		})
	}
}

func TestScanSignals_IncludesWaveSignals(t *testing.T) {
	signalsDir := filepath.Join(t.TempDir(), ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))

	// Write a regular signal and a wave signal
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "planner-finished-2026-02-20-test.md"), nil, 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(signalsDir, "implement-wave-2-2026-02-20-test.md"), nil, 0o644))

	signals := ScanSignals(signalsDir)
	// Regular signal should be present
	assert.Len(t, signals, 1, "only regular signals returned by ScanSignals")

	// Wave signals have their own scanner
	waveSignals := ScanWaveSignals(signalsDir)
	require.Len(t, waveSignals, 1)
	assert.Equal(t, 2, waveSignals[0].WaveNumber)
	assert.Equal(t, "2026-02-20-test.md", waveSignals[0].PlanFile)
}
