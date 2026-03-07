package loop

import (
	"encoding/json"
	"fmt"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstore"
)

// BridgeFilesystemSignals reads all sentinel files from the project's signals
// directory (and any active worktree signal directories) and persists each one
// as a pending row in the SignalGateway. After each successful persist the
// corresponding sentinel file is removed, maintaining the consume-after-persist
// contract: if Create or Marshal fails the file is left on disk for retry.
//
// Returns the number of signals successfully bridged and any error encountered
// during marshalling or gateway persistence.
func BridgeFilesystemSignals(gw taskstore.SignalGateway, project, repoRoot string, worktreePaths []string) (int, error) {
	if gw == nil {
		return 0, fmt.Errorf("nil signal gateway")
	}

	scan := ScanAllSignals(repoRoot, worktreePaths)
	bridged := 0

	// --- FSM signals (planner-finished, implement-finished, review-*) ---
	for _, sig := range scan.FSMSignals {
		payload, err := json.Marshal(map[string]string{"body": sig.Body})
		if err != nil {
			return bridged, fmt.Errorf("marshal fsm signal payload: %w", err)
		}
		entry := taskstore.SignalEntry{
			PlanFile:   sig.TaskFile,
			SignalType: string(sig.Event),
			Payload:    string(payload),
		}
		if err := gw.Create(project, entry); err != nil {
			return bridged, fmt.Errorf("create fsm signal: %w", err)
		}
		taskfsm.ConsumeSignal(sig)
		bridged++
	}

	// --- Task signals (implement-task-finished-wN-tN-<plan>) ---
	for _, ts := range scan.TaskSignals {
		payload, err := json.Marshal(map[string]int{
			"wave_number": ts.WaveNumber,
			"task_number": ts.TaskNumber,
		})
		if err != nil {
			return bridged, fmt.Errorf("marshal task signal payload: %w", err)
		}
		entry := taskstore.SignalEntry{
			PlanFile:   ts.TaskFile,
			SignalType: "implement_task_finished",
			Payload:    string(payload),
		}
		if err := gw.Create(project, entry); err != nil {
			return bridged, fmt.Errorf("create task signal: %w", err)
		}
		taskfsm.ConsumeTaskSignal(ts)
		bridged++
	}

	// --- Wave signals (implement-wave-N-<plan>) ---
	for _, ws := range scan.WaveSignals {
		payload, err := json.Marshal(map[string]int{
			"wave_number": ws.WaveNumber,
		})
		if err != nil {
			return bridged, fmt.Errorf("marshal wave signal payload: %w", err)
		}
		entry := taskstore.SignalEntry{
			PlanFile:   ws.TaskFile,
			SignalType: "implement_wave",
			Payload:    string(payload),
		}
		if err := gw.Create(project, entry); err != nil {
			return bridged, fmt.Errorf("create wave signal: %w", err)
		}
		taskfsm.ConsumeWaveSignal(ws)
		bridged++
	}

	// --- Elaboration signals (elaborator-finished-<plan>) ---
	for _, es := range scan.ElaborationSignals {
		payload, err := json.Marshal(map[string]string{})
		if err != nil {
			return bridged, fmt.Errorf("marshal elaboration signal payload: %w", err)
		}
		entry := taskstore.SignalEntry{
			PlanFile:   es.TaskFile,
			SignalType: "elaborator_finished",
			Payload:    string(payload),
		}
		if err := gw.Create(project, entry); err != nil {
			return bridged, fmt.Errorf("create elaboration signal: %w", err)
		}
		taskfsm.ConsumeElaborationSignal(es)
		bridged++
	}

	return bridged, nil
}
