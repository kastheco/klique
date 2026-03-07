package loop

import (
	"path/filepath"

	"github.com/kastheco/kasmos/config/taskfsm"
)

// ScanResult aggregates all signal types found across the project and its
// active worktrees. Signals are deduplicated by key so that the same sentinel
// file written in both the main repo and a worktree is only processed once.
type ScanResult struct {
	FSMSignals         []taskfsm.Signal
	TaskSignals        []taskfsm.TaskSignal
	WaveSignals        []taskfsm.WaveSignal
	ElaborationSignals []taskfsm.ElaborationSignal
}

// ScanAllSignals reads signal files from the project's own signals directory
// (.kasmos/signals/ under repoRoot) and from each path in worktreePaths,
// returning a deduplicated ScanResult.
//
// Extracts and generalises the scanning logic from app.go lines 826-886.
func ScanAllSignals(repoRoot string, worktreePaths []string) ScanResult {
	signalsDir := filepath.Join(repoRoot, ".kasmos", "signals")

	// --- FSM signals ---
	fsmSignals := taskfsm.ScanSignals(signalsDir)
	seenFSM := make(map[string]bool, len(fsmSignals))
	for _, s := range fsmSignals {
		seenFSM[s.Key()] = true
	}

	// --- Task signals ---
	taskSignals := taskfsm.ScanTaskSignals(signalsDir)
	seenTask := make(map[string]bool, len(taskSignals))
	for _, s := range taskSignals {
		seenTask[s.Key()] = true
	}

	// --- Wave signals ---
	waveSignals := taskfsm.ScanWaveSignals(signalsDir)
	seenWave := make(map[string]bool, len(waveSignals))
	for _, s := range waveSignals {
		seenWave[s.TaskFile] = true
	}

	// --- Elaboration signals ---
	elabSignals := taskfsm.ScanElaborationSignals(signalsDir)
	seenElab := make(map[string]bool, len(elabSignals))
	for _, s := range elabSignals {
		seenElab[s.TaskFile] = true
	}

	// Merge signals from each worktree, deduplicating by key.
	for _, wt := range worktreePaths {
		if wt == "" {
			continue
		}
		wtDir := filepath.Join(wt, ".kasmos", "signals")

		for _, s := range taskfsm.ScanSignals(wtDir) {
			if !seenFSM[s.Key()] {
				seenFSM[s.Key()] = true
				fsmSignals = append(fsmSignals, s)
			}
		}
		for _, s := range taskfsm.ScanTaskSignals(wtDir) {
			if !seenTask[s.Key()] {
				seenTask[s.Key()] = true
				taskSignals = append(taskSignals, s)
			}
		}
		for _, s := range taskfsm.ScanWaveSignals(wtDir) {
			if !seenWave[s.TaskFile] {
				seenWave[s.TaskFile] = true
				waveSignals = append(waveSignals, s)
			}
		}
		for _, s := range taskfsm.ScanElaborationSignals(wtDir) {
			if !seenElab[s.TaskFile] {
				seenElab[s.TaskFile] = true
				elabSignals = append(elabSignals, s)
			}
		}
	}

	return ScanResult{
		FSMSignals:         fsmSignals,
		TaskSignals:        taskSignals,
		WaveSignals:        waveSignals,
		ElaborationSignals: elabSignals,
	}
}

// Tick is a convenience method that runs all four signal-processing passes on a
// pre-scanned ScanResult and returns the concatenated list of actions for the
// caller to execute. It is the primary entry point for the daemon's event loop.
func (p *Processor) Tick(scan ScanResult) []Action {
	var actions []Action
	actions = append(actions, p.ProcessFSMSignals(scan.FSMSignals)...)
	actions = append(actions, p.ProcessTaskSignals(scan.TaskSignals)...)
	actions = append(actions, p.ProcessWaveSignals(scan.WaveSignals)...)
	actions = append(actions, p.ProcessElaborationSignals(scan.ElaborationSignals)...)
	return actions
}
