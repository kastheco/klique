package loop

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstore"
)

// payload helper types for DB-backed signal rows.
type bodyPayload struct {
	Body string `json:"body"`
}

type taskPayload struct {
	WaveNumber int `json:"wave_number"`
	TaskNumber int `json:"task_number"`
}

type wavePayload struct {
	WaveNumber int `json:"wave_number"`
}

// ScanGateway claims all pending signals for the given project from gw,
// converts them into a ScanResult that Processor.Tick can consume, and returns
// the claimed row IDs so the caller can mark them done after processing.
//
// Signals are claimed one at a time using the atomic Claim method to prevent
// double-processing. The function does NOT call MarkProcessed — ownership of
// the post-processing lifecycle belongs to the caller.
//
// Error handling:
//   - An unknown signal type or a malformed JSON payload returns an error.
//     Any IDs already claimed before the bad row are included in the return value.
//   - An empty payload is valid for FSM and elaboration signals and produces
//     an empty Body field.
func ScanGateway(gw taskstore.SignalGateway, project, claimedBy string) (ScanResult, []int64, error) {
	var result ScanResult
	var ids []int64

	for {
		entry, err := gw.Claim(project, claimedBy)
		if err != nil {
			return result, ids, fmt.Errorf("claim signal: %w", err)
		}
		if entry == nil {
			break
		}

		if convertErr := ConvertSignalEntry(entry, &result); convertErr != nil {
			// Mark the bad row as failed immediately so it does not cycle through
			// the reaper and block older valid signals from progressing.
			if markErr := gw.MarkProcessed(entry.ID, taskstore.SignalFailed, convertErr.Error()); markErr != nil {
				slog.Default().Error("gateway_scanner: mark signal as failed", "id", entry.ID, "err", markErr)
			}
			return result, ids, fmt.Errorf("signal %d (%s): %w", entry.ID, entry.SignalType, convertErr)
		}

		ids = append(ids, entry.ID)
	}

	return result, ids, nil
}

// ConvertSignalEntry decodes a single SignalEntry and appends it to result.
func ConvertSignalEntry(entry *taskstore.SignalEntry, result *ScanResult) error {
	switch entry.SignalType {
	case "planner_finished":
		body, err := decodeBody(entry.Payload)
		if err != nil {
			return err
		}
		result.FSMSignals = append(result.FSMSignals, taskfsm.Signal{
			Event:    taskfsm.PlannerFinished,
			TaskFile: entry.PlanFile,
			Body:     body,
		})

	case "implement_finished":
		body, err := decodeBody(entry.Payload)
		if err != nil {
			return err
		}
		result.FSMSignals = append(result.FSMSignals, taskfsm.Signal{
			Event:    taskfsm.ImplementFinished,
			TaskFile: entry.PlanFile,
			Body:     body,
		})

	case "review_approved":
		body, err := decodeBody(entry.Payload)
		if err != nil {
			return err
		}
		result.FSMSignals = append(result.FSMSignals, taskfsm.Signal{
			Event:    taskfsm.ReviewApproved,
			TaskFile: entry.PlanFile,
			Body:     body,
		})

	case "review_changes_requested":
		body, err := decodeBody(entry.Payload)
		if err != nil {
			return err
		}
		result.FSMSignals = append(result.FSMSignals, taskfsm.Signal{
			Event:    taskfsm.ReviewChangesRequested,
			TaskFile: entry.PlanFile,
			Body:     body,
		})

	case "implement_task_finished":
		var p taskPayload
		if err := json.Unmarshal([]byte(entry.Payload), &p); err != nil {
			return fmt.Errorf("decode task payload: %w", err)
		}
		result.TaskSignals = append(result.TaskSignals, taskfsm.TaskSignal{
			WaveNumber: p.WaveNumber,
			TaskNumber: p.TaskNumber,
			TaskFile:   entry.PlanFile,
		})

	case "implement_wave":
		var p wavePayload
		if err := json.Unmarshal([]byte(entry.Payload), &p); err != nil {
			return fmt.Errorf("decode wave payload: %w", err)
		}
		result.WaveSignals = append(result.WaveSignals, taskfsm.WaveSignal{
			WaveNumber: p.WaveNumber,
			TaskFile:   entry.PlanFile,
		})

	case "elaborator_finished":
		result.ElaborationSignals = append(result.ElaborationSignals, taskfsm.ElaborationSignal{
			TaskFile: entry.PlanFile,
		})

	default:
		return fmt.Errorf("unknown signal type %q", entry.SignalType)
	}

	return nil
}

// decodeBody extracts the optional "body" field from a JSON payload string.
// An empty payload string is treated as valid and returns an empty body.
func decodeBody(payload string) (string, error) {
	if payload == "" {
		return "", nil
	}
	var p bodyPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return "", fmt.Errorf("decode body payload: %w", err)
	}
	return p.Body, nil
}
