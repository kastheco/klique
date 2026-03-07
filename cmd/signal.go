package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/spf13/cobra"
)

// validSignalTypes lists the signal types that the gateway pipeline can consume today.
// architect_finished is intentionally excluded — its FSM consumption is follow-up work.
var validSignalTypes = map[string]struct{}{
	"planner_finished":         {},
	"implement_finished":       {},
	"review_approved":          {},
	"review_changes_requested": {},
	"implement_task_finished":  {},
	"implement_wave":           {},
	"elaborator_finished":      {},
}

// signalProcessOptions holds the dependencies for executeSignalProcess,
// enabling injection in tests without cobra plumbing.
type signalProcessOptions struct {
	repoRoot   string
	project    string
	signalsDir string
	store      taskstore.Store
}

// NewSignalCmd returns the "kas signal" cobra command with subcommands.
func NewSignalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "signal",
		Short: "inspect and process agent lifecycle signals",
	}
	cmd.AddCommand(newSignalListCmd())
	cmd.AddCommand(newSignalProcessCmd())
	cmd.AddCommand(newSignalEmitCmd())
	return cmd
}

// newSignalListCmd returns the "kas signal list" subcommand.
func newSignalListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list pending signals in the signals directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, _, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			signalsDir := defaultSignalsDir(repoRoot)
			fmt.Fprint(cmd.OutOrStdout(), executeSignalList(signalsDir))
			return nil
		},
	}
}

// newSignalProcessCmd returns the "kas signal process" subcommand.
func newSignalProcessCmd() *cobra.Command {
	var once bool
	cmd := &cobra.Command{
		Use:   "process",
		Short: "process pending agent lifecycle signals",
		Long: `process scans the signals directory and applies FSM transitions for each
pending signal. By default it runs in a continuous loop, polling every 5 seconds.
Use --once to process a single batch and exit.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			store := resolveStore(project)
			signalsDir := defaultSignalsDir(repoRoot)
			if err := os.MkdirAll(signalsDir, 0o755); err != nil {
				return fmt.Errorf("create signals dir: %w", err)
			}
			opts := signalProcessOptions{
				repoRoot:   repoRoot,
				project:    project,
				signalsDir: signalsDir,
				store:      store,
			}
			if once {
				n, err := executeSignalProcess(opts)
				if err != nil {
					return err
				}
				if n > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "processed %d signal(s)\n", n)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "no signals to process")
				}
				return nil
			}
			// Continuous loop mode.
			fmt.Fprintln(cmd.OutOrStdout(), "watching for signals (ctrl-c to stop)...")
			for {
				n, err := executeSignalProcess(opts)
				if err != nil {
					log.Printf("signal process error: %v", err)
				} else if n > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "processed %d signal(s)\n", n)
				}
				time.Sleep(5 * time.Second)
			}
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "process one batch of signals and exit")
	return cmd
}

// executeSignalList returns a formatted string listing all pending signals in
// the given directory. Returns "no pending signals\n" when the directory is
// empty or missing.
func executeSignalList(signalsDir string) string {
	var lines []string

	// FSM signals.
	for _, sig := range taskfsm.ScanSignals(signalsDir) {
		lines = append(lines, fmt.Sprintf("%-30s  %s", string(sig.Event), sig.TaskFile))
	}

	// Wave signals.
	for _, ws := range taskfsm.ScanWaveSignals(signalsDir) {
		lines = append(lines, fmt.Sprintf("%-30s  %s  (wave %d)", "implement_wave", ws.TaskFile, ws.WaveNumber))
	}

	// Elaboration signals.
	for _, es := range taskfsm.ScanElaborationSignals(signalsDir) {
		lines = append(lines, fmt.Sprintf("%-30s  %s", "elaborator_finished", es.TaskFile))
	}

	if len(lines) == 0 {
		return "no pending signals\n"
	}
	return strings.Join(lines, "\n") + "\n"
}

// executeSignalProcess scans the signals directory, applies FSM transitions for
// each valid signal, and consumes (deletes) the signal files. Returns the number
// of signals that resulted in a successful FSM transition.
//
// Signals for unknown plans are consumed without error and do not count toward
// the returned total. Wave and elaboration signals are consumed but do not
// trigger FSM transitions (they are handled by the TUI wave orchestrator).
func executeSignalProcess(opts signalProcessOptions) (int, error) {
	fsm := newFSMByProject(opts.project, opts.store)
	processed := 0

	// Process FSM signals (implement-finished, review-approved, etc.).
	for _, sig := range taskfsm.ScanSignals(opts.signalsDir) {
		ps, err := taskstate.Load(opts.store, opts.project, "")
		if err != nil {
			return processed, fmt.Errorf("load task state: %w", err)
		}
		_, ok := ps.Entry(sig.TaskFile)
		if !ok {
			// Unknown plan — consume the signal but don't count it.
			log.Printf("signal: unknown plan %q for event %s — consuming", sig.TaskFile, sig.Event)
			taskfsm.ConsumeSignal(sig)
			continue
		}

		if err := fsm.Transition(sig.TaskFile, sig.Event); err != nil {
			// Invalid transition (e.g. wrong state) — log and consume.
			log.Printf("signal: transition failed for %q event %s: %v — consuming", sig.TaskFile, sig.Event, err)
			taskfsm.ConsumeSignal(sig)
			continue
		}

		// For review-changes, increment the review cycle counter.
		if sig.Event == taskfsm.ReviewChangesRequested {
			ps2, err := taskstate.Load(opts.store, opts.project, "")
			if err == nil {
				if incErr := ps2.IncrementReviewCycle(sig.TaskFile); incErr != nil {
					log.Printf("signal: increment review cycle for %q: %v", sig.TaskFile, incErr)
				}
			} else {
				log.Printf("signal: reload task state for review cycle increment %q: %v", sig.TaskFile, err)
			}
		}

		taskfsm.ConsumeSignal(sig)
		processed++
	}

	// Consume wave signals (no FSM transition — handled by TUI orchestrator).
	for _, ws := range taskfsm.ScanWaveSignals(opts.signalsDir) {
		log.Printf("signal: wave signal wave=%d plan=%s — consuming (TUI handles orchestration)", ws.WaveNumber, ws.TaskFile)
		taskfsm.ConsumeWaveSignal(ws)
	}

	// Consume elaboration signals (no FSM transition).
	for _, es := range taskfsm.ScanElaborationSignals(opts.signalsDir) {
		log.Printf("signal: elaboration signal plan=%s — consuming", es.TaskFile)
		taskfsm.ConsumeElaborationSignal(es)
	}

	return processed, nil
}

// defaultSignalsDir returns the canonical signals directory path for a repo root.
func defaultSignalsDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".kasmos", "signals")
}

// normalizeSignalPayload validates and normalises the raw payload string for a
// given signal type, returning the value that should be stored in the gateway.
//
//   - FSM signals (planner_finished, implement_finished, review_approved,
//     review_changes_requested): empty → ""; JSON → kept; plain text → {"body":"..."}
//   - implement_task_finished: must be JSON with numeric wave_number and task_number
//   - implement_wave: must be JSON with numeric wave_number
//   - elaborator_finished: payload must be empty
func normalizeSignalPayload(signalType, payload string) (string, error) {
	switch signalType {
	case "planner_finished", "implement_finished", "review_approved", "review_changes_requested":
		if payload == "" {
			return "", nil
		}
		if json.Valid([]byte(payload)) {
			return payload, nil
		}
		b, _ := json.Marshal(map[string]string{"body": payload})
		return string(b), nil

	case "implement_task_finished":
		if payload == "" {
			return "", fmt.Errorf("implement_task_finished requires JSON with wave_number and task_number")
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			return "", fmt.Errorf("implement_task_finished: payload must be valid JSON: %w", err)
		}
		wn, ok := m["wave_number"].(float64)
		if !ok {
			return "", fmt.Errorf("implement_task_finished: wave_number must be a number")
		}
		if wn != math.Trunc(wn) {
			return "", fmt.Errorf("implement_task_finished: wave_number must be a whole number")
		}
		tn, ok := m["task_number"].(float64)
		if !ok {
			return "", fmt.Errorf("implement_task_finished: task_number must be a number")
		}
		if tn != math.Trunc(tn) {
			return "", fmt.Errorf("implement_task_finished: task_number must be a whole number")
		}
		return payload, nil

	case "implement_wave":
		if payload == "" {
			return "", fmt.Errorf("implement_wave requires JSON with wave_number")
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			return "", fmt.Errorf("implement_wave: payload must be valid JSON: %w", err)
		}
		wn, ok := m["wave_number"].(float64)
		if !ok {
			return "", fmt.Errorf("implement_wave: wave_number must be a number")
		}
		if wn != math.Trunc(wn) {
			return "", fmt.Errorf("implement_wave: wave_number must be a whole number")
		}
		return payload, nil

	case "elaborator_finished":
		if payload != "" {
			return "", fmt.Errorf("elaborator_finished does not accept a payload")
		}
		return "", nil

	default:
		return "", fmt.Errorf("unknown signal type %q", signalType)
	}
}

// executeSignalEmit validates the signal type, normalises the payload, and
// inserts a pending signal row via the provided gateway.
func executeSignalEmit(gw taskstore.SignalGateway, project, signalType, planFile, payload string) error {
	if _, ok := validSignalTypes[signalType]; !ok {
		return fmt.Errorf("unknown signal type %q; valid types: planner_finished, implement_finished, review_approved, review_changes_requested, implement_task_finished, implement_wave, elaborator_finished", signalType)
	}

	normalized, err := normalizeSignalPayload(signalType, payload)
	if err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	return gw.Create(project, taskstore.SignalEntry{
		PlanFile:   planFile,
		SignalType: signalType,
		Payload:    normalized,
	})
}

// newSignalEmitCmd returns the "kas signal emit" subcommand.
func newSignalEmitCmd() *cobra.Command {
	var payload string
	cmd := &cobra.Command{
		Use:   "emit <signal-type> <plan-file>",
		Short: "emit a signal into the gateway database",
		Long: `emit inserts a pending signal row into the gateway database for the given
signal type and plan file. This is the primary mechanism for agents to signal
completion of a lifecycle phase.

Valid signal types: planner_finished, implement_finished, review_approved,
review_changes_requested, implement_task_finished, implement_wave, elaborator_finished`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			signalType := args[0]
			planFile := args[1]

			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}

			dbPath := taskstore.ResolvedDBPath()
			if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
				return fmt.Errorf("create kasmos config dir: %w", err)
			}

			gw, err := taskstore.NewSQLiteSignalGateway(dbPath)
			if err != nil {
				return fmt.Errorf("open signal gateway: %w", err)
			}
			defer gw.Close() //nolint:errcheck

			if err := executeSignalEmit(gw, project, signalType, planFile, payload); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "signal emitted: type=%s plan=%s\n", signalType, planFile)
			return nil
		},
	}
	cmd.Flags().StringVar(&payload, "payload", "", "optional JSON payload for the signal")
	return cmd
}
