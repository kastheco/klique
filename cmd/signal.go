package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/spf13/cobra"
)

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
// each valid signal using atomic processing (move → processing → complete/failed).
// Returns the number of signals that resulted in a successful FSM transition.
//
// Signals for unknown plans or invalid transitions are dead-lettered into the
// failed/ subdirectory with a companion .reason file. Wave and elaboration signals
// are consumed atomically but do not trigger FSM transitions (handled by TUI).
func executeSignalProcess(opts signalProcessOptions) (int, error) {
	if err := taskfsm.EnsureSignalDirs(opts.signalsDir); err != nil {
		return 0, fmt.Errorf("ensure signal dirs: %w", err)
	}

	// Recover any signals that were moved to processing/ by a previous run
	// that was interrupted (crash or CTRL-C) before it could complete or
	// dead-letter them. Without this, those files would be invisible to every
	// future scan because ScanSignals only reads the base signals directory.
	if n := taskfsm.RecoverInFlight(opts.signalsDir); n > 0 {
		log.Printf("signal: recovered %d in-flight signals from processing dir", n)
	}

	fsm := newFSMByProject(opts.project, opts.store)
	processed := 0

	// Process FSM signals (implement-finished, review-approved, etc.).
	for _, sig := range taskfsm.ScanSignals(opts.signalsDir) {
		fn := sig.Filename()

		procPath, err := taskfsm.BeginProcessing(opts.signalsDir, fn)
		if err != nil {
			log.Printf("signal: begin processing failed file=%s event=%s: %v", fn, sig.Event, err)
			continue
		}

		ps, err := taskstate.Load(opts.store, opts.project, "")
		if err != nil {
			taskfsm.FailProcessing(opts.signalsDir, fn, fmt.Sprintf("load task state: %v", err))
			return processed, fmt.Errorf("load task state: %w", err)
		}

		_, ok := ps.Entry(sig.TaskFile)
		if !ok {
			log.Printf("signal: unknown plan file=%s event=%s plan=%s outcome=dead-letter", fn, sig.Event, sig.TaskFile)
			taskfsm.FailProcessing(opts.signalsDir, fn, "unknown plan: "+sig.TaskFile)
			continue
		}

		if err := fsm.Transition(sig.TaskFile, sig.Event); err != nil {
			log.Printf("signal: transition failed file=%s event=%s plan=%s outcome=dead-letter: %v", fn, sig.Event, sig.TaskFile, err)
			taskfsm.FailProcessing(opts.signalsDir, fn, fmt.Sprintf("transition %s: %v", sig.Event, err))
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

		log.Printf("signal: completed file=%s event=%s plan=%s outcome=success", fn, sig.Event, sig.TaskFile)
		taskfsm.CompleteProcessing(procPath)
		processed++
	}

	// Consume wave signals atomically (no FSM transition — handled by TUI orchestrator).
	for _, ws := range taskfsm.ScanWaveSignals(opts.signalsDir) {
		fn := ws.Filename()
		procPath, err := taskfsm.BeginProcessing(opts.signalsDir, fn)
		if err != nil {
			log.Printf("signal: begin processing wave signal file=%s wave=%d plan=%s: %v", fn, ws.WaveNumber, ws.TaskFile, err)
			continue
		}
		log.Printf("signal: completed file=%s event=implement_wave plan=%s wave=%d outcome=consumed", fn, ws.TaskFile, ws.WaveNumber)
		taskfsm.CompleteProcessing(procPath)
	}

	// Consume elaboration signals atomically (no FSM transition).
	for _, es := range taskfsm.ScanElaborationSignals(opts.signalsDir) {
		fn := es.Filename()
		procPath, err := taskfsm.BeginProcessing(opts.signalsDir, fn)
		if err != nil {
			log.Printf("signal: begin processing elaboration signal file=%s plan=%s: %v", fn, es.TaskFile, err)
			continue
		}
		log.Printf("signal: completed file=%s event=elaborator_finished plan=%s outcome=consumed", fn, es.TaskFile)
		taskfsm.CompleteProcessing(procPath)
	}

	return processed, nil
}

// defaultSignalsDir returns the canonical signals directory path for a repo root.
func defaultSignalsDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".kasmos", "signals")
}
