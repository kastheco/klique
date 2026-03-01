package planfsm

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// WaveSignal represents a parsed implement-wave signal file.
type WaveSignal struct {
	WaveNumber int
	PlanFile   string
	filePath   string // full path for deletion
}

var waveSignalRe = regexp.MustCompile(`^implement-wave-(\d+)-(.+\.md)$`)

// ParseWaveSignal attempts to parse a filename as a wave signal.
func ParseWaveSignal(filename string) (WaveSignal, bool) {
	m := waveSignalRe.FindStringSubmatch(filename)
	if m == nil {
		return WaveSignal{}, false
	}
	wave, err := strconv.Atoi(m[1])
	if err != nil {
		return WaveSignal{}, false
	}
	return WaveSignal{
		WaveNumber: wave,
		PlanFile:   m[2],
	}, true
}

// ScanWaveSignals reads the given signals directory and returns parsed wave signals.
// These are handled separately from FSM signals because they don't map to
// state transitions — they trigger wave orchestration in the TUI.
// The caller is responsible for passing the full signals directory path
// (e.g. filepath.Join(repoRoot, ".kasmos", "signals")).
func ScanWaveSignals(signalsDir string) []WaveSignal {
	entries, err := os.ReadDir(signalsDir)
	if err != nil {
		return nil
	}

	var signals []WaveSignal
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ws, ok := ParseWaveSignal(entry.Name())
		if !ok {
			continue
		}
		ws.filePath = filepath.Join(signalsDir, entry.Name())
		signals = append(signals, ws)
	}
	return signals
}

// ConsumeWaveSignal deletes the wave signal file after processing.
func ConsumeWaveSignal(ws WaveSignal) {
	_ = os.Remove(ws.filePath)
}
