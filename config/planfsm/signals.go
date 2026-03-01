package planfsm

import (
	"os"
	"path/filepath"
	"strings"
)

// Signal represents a parsed sentinel file from an agent.
type Signal struct {
	Event    Event
	PlanFile string
	Body     string // file contents (e.g. review feedback)
	filePath string // full path for deletion
}

// Key returns a dedup key for this signal (event + plan file).
func (s Signal) Key() string {
	return string(s.Event) + ":" + s.PlanFile
}

// sentinelPrefixes maps filename prefixes to FSM events.
var sentinelPrefixes = []struct {
	prefix string
	event  Event
}{
	{"planner-finished-", PlannerFinished},
	{"implement-finished-", ImplementFinished},
	{"review-approved-", ReviewApproved},
	{"review-changes-", ReviewChangesRequested},
}

// ScanSignals reads the given signals directory and returns parsed signals.
// Ignores invalid files and user-only events. Returns nil if directory missing.
// The caller is responsible for passing the full signals directory path
// (e.g. filepath.Join(repoRoot, ".kasmos", "signals")).
func ScanSignals(signalsDir string) []Signal {
	entries, err := os.ReadDir(signalsDir)
	if err != nil {
		return nil
	}

	var signals []Signal
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		sig, ok := parseSignal(signalsDir, entry.Name())
		if !ok {
			continue
		}
		if sig.Event.IsUserOnly() {
			continue
		}
		signals = append(signals, sig)
	}
	return signals
}

// ConsumeSignal deletes the sentinel file after processing.
func ConsumeSignal(sig Signal) {
	_ = os.Remove(sig.filePath)
}

func parseSignal(dir, filename string) (Signal, bool) {
	for _, sp := range sentinelPrefixes {
		if strings.HasPrefix(filename, sp.prefix) {
			planFile := strings.TrimPrefix(filename, sp.prefix)
			if planFile == "" {
				return Signal{}, false
			}
			// Defensive: strip any path prefix (e.g. "docs/plans/") so the
			// planFile matches plan-state.json keys which are bare filenames.
			planFile = filepath.Base(planFile)
			filePath := filepath.Join(dir, filename)
			body := ""
			if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
				body = strings.TrimSpace(string(data))
			}
			return Signal{
				Event:    sp.event,
				PlanFile: planFile,
				Body:     body,
				filePath: filePath,
			}, true
		}
	}
	return Signal{}, false
}
