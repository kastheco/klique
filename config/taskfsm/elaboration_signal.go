package taskfsm

import (
	"os"
	"path/filepath"
	"strings"
)

// ElaborationSignal represents a parsed elaborator-finished signal file.
type ElaborationSignal struct {
	TaskFile string
	filePath string // full path for deletion
}

const elaborationPrefix = "elaborator-finished-"

// ParseElaborationSignal attempts to parse a filename as an elaboration signal.
func ParseElaborationSignal(filename string) (ElaborationSignal, bool) {
	if !strings.HasPrefix(filename, elaborationPrefix) {
		return ElaborationSignal{}, false
	}
	planFile := strings.TrimPrefix(filename, elaborationPrefix)
	if planFile == "" {
		return ElaborationSignal{}, false
	}
	planFile = filepath.Base(planFile)
	return ElaborationSignal{TaskFile: planFile}, true
}

// ScanElaborationSignals reads the given signals directory and returns parsed
// elaboration signals. Like wave signals, these are handled separately from FSM
// signals — they don't map to state transitions but trigger orchestration actions.
func ScanElaborationSignals(signalsDir string) []ElaborationSignal {
	entries, err := os.ReadDir(signalsDir)
	if err != nil {
		return nil
	}
	var signals []ElaborationSignal
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		es, ok := ParseElaborationSignal(entry.Name())
		if !ok {
			continue
		}
		es.filePath = filepath.Join(signalsDir, entry.Name())
		signals = append(signals, es)
	}
	return signals
}

// ConsumeElaborationSignal deletes the signal file after processing.
func ConsumeElaborationSignal(es ElaborationSignal) {
	_ = os.Remove(es.filePath)
}

// ClearElaborationSignal removes any stale elaborator-finished-<planFile> sentinel
// from signalsDir before a new elaborator run begins. This prevents a leftover file
// from a prior run (e.g. after a TUI restart) from being picked up immediately and
// skipping the new elaborator's output.
func ClearElaborationSignal(signalsDir, planFile string) {
	name := elaborationPrefix + planFile
	_ = os.Remove(filepath.Join(signalsDir, name))
}
