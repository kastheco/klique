package taskfsm

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// TaskSignal represents a parsed completion signal for a specific wave task.
type TaskSignal struct {
	WaveNumber int
	TaskNumber int
	TaskFile   string
	filePath   string // full path for deletion
}

var taskSignalRe = regexp.MustCompile(`^implement-task-finished-w(\d+)-t(\d+)-(.+\.md)$`)

// ParseTaskSignal attempts to parse a filename as a wave-task completion signal.
func ParseTaskSignal(filename string) (TaskSignal, bool) {
	m := taskSignalRe.FindStringSubmatch(filename)
	if m == nil {
		return TaskSignal{}, false
	}
	wave, err := strconv.Atoi(m[1])
	if err != nil || wave < 1 {
		return TaskSignal{}, false
	}
	taskNumber, err := strconv.Atoi(m[2])
	if err != nil || taskNumber < 1 {
		return TaskSignal{}, false
	}
	return TaskSignal{
		WaveNumber: wave,
		TaskNumber: taskNumber,
		TaskFile:   m[3],
	}, true
}

// Key returns a dedup key for this signal (wave:task:plan).
func (s TaskSignal) Key() string {
	return strconv.Itoa(s.WaveNumber) + ":" + strconv.Itoa(s.TaskNumber) + ":" + s.TaskFile
}

// ScanTaskSignals reads the given signals directory and returns parsed task signals.
// The caller is responsible for passing the full signals directory path
// (e.g. filepath.Join(repoRoot, ".kasmos", "signals")).
func ScanTaskSignals(signalsDir string) []TaskSignal {
	entries, err := os.ReadDir(signalsDir)
	if err != nil {
		return nil
	}

	var signals []TaskSignal
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		sig, ok := ParseTaskSignal(entry.Name())
		if !ok {
			continue
		}
		sig.filePath = filepath.Join(signalsDir, entry.Name())
		sig.TaskFile = filepath.Base(sig.TaskFile)
		signals = append(signals, sig)
	}
	return signals
}

// ConsumeTaskSignal deletes the task signal file after processing.
func ConsumeTaskSignal(ts TaskSignal) {
	_ = os.Remove(ts.filePath)
}
