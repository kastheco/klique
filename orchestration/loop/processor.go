package loop

import (
	"fmt"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/orchestration"
)

// ProcessorConfig holds the dependencies needed to construct a Processor.
type ProcessorConfig struct {
	// Store is the plan state persistence backend. Must be non-nil.
	Store taskstore.Store
	// Project is the project name used with the store.
	Project string
	// Dir is the legacy directory path used for file rename operations (may be empty).
	Dir string
	// AutoReviewFix enables automatic fixer spawning after review changes.
	AutoReviewFix bool
	// MaxReviewFixCycles is the maximum number of review-fix cycles allowed
	// before emitting ReviewCycleLimitAction instead of SpawnCoderAction.
	// Zero or negative means unlimited.
	MaxReviewFixCycles int
}

// Processor converts signal scan results into typed Action values without
// performing side effects. The caller is responsible for executing the returned
// actions (spawning agents, creating PRs, etc.).
type Processor struct {
	config            ProcessorConfig
	fsm               *taskfsm.TaskStateMachine
	waveOrchestrators map[string]*orchestration.WaveOrchestrator
	// activeWaveOrchs tracks plans whose wave orchestrator is active.
	// ImplementFinished signals are suppressed for plans in this set so that
	// individual wave-task agents don't prematurely trigger the reviewing state.
	activeWaveOrchs map[string]bool
}

// NewProcessor creates a Processor backed by the given store and project.
func NewProcessor(cfg ProcessorConfig) *Processor {
	return &Processor{
		config:            cfg,
		fsm:               taskfsm.New(cfg.Store, cfg.Project, cfg.Dir),
		waveOrchestrators: make(map[string]*orchestration.WaveOrchestrator),
		activeWaveOrchs:   make(map[string]bool),
	}
}

// SetReviewFixConfig updates the runtime review-fix loop settings.
func (p *Processor) SetReviewFixConfig(enabled bool, maxCycles int) {
	p.config.AutoReviewFix = enabled
	p.config.MaxReviewFixCycles = maxCycles
}

// SetWaveOrchestratorActive marks or unmarks a plan as having an active wave
// orchestrator. When active, ImplementFinished signals for that plan are
// suppressed in ProcessFSMSignals.
func (p *Processor) SetWaveOrchestratorActive(planFile string, active bool) {
	if active {
		p.activeWaveOrchs[planFile] = true
	} else {
		delete(p.activeWaveOrchs, planFile)
	}
}

// RegisterOrchestrator creates a wave orchestrator for the given plan with the
// specified wave number and task numbers in the running state. Intended for
// tests and daemon restore operations.
func (p *Processor) RegisterOrchestrator(planFile string, waveNumber int, taskNumbers []int) {
	tasks := make([]taskparser.Task, len(taskNumbers))
	for i, n := range taskNumbers {
		tasks[i] = taskparser.Task{Number: n, Title: fmt.Sprintf("Task %d", n)}
	}
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{{Number: waveNumber, Tasks: tasks}},
	}
	orch := orchestration.NewWaveOrchestrator(planFile, plan)
	if p.config.Store != nil {
		orch.SetStore(p.config.Store, p.config.Project)
	}
	orch.StartNextWave() // puts tasks into running state
	p.waveOrchestrators[planFile] = orch
}

// WaveOrchestrator returns the active WaveOrchestrator for the given plan file,
// or nil if none is registered.
func (p *Processor) WaveOrchestrator(planFile string) *orchestration.WaveOrchestrator {
	return p.waveOrchestrators[planFile]
}

// ProcessFSMSignals converts FSM sentinel signals into Action values.
// It validates each signal against the plan state machine, suppresses
// ImplementFinished when a wave orchestrator is active, and emits typed
// actions for the caller to execute.
//
// Extracted from app.go metadataResultMsg handler (lines 921-1077).
func (p *Processor) ProcessFSMSignals(signals []taskfsm.Signal) []Action {
	var actions []Action
	for _, sig := range signals {
		// Guard: suppress ImplementFinished when a wave orchestrator is active.
		// Wave task agents write this sentinel after each task, but the wave
		// orchestrator owns the implementing→reviewing transition.
		if sig.Event == taskfsm.ImplementFinished {
			if p.activeWaveOrchs[sig.TaskFile] {
				continue
			}
			if _, hasOrch := p.waveOrchestrators[sig.TaskFile]; hasOrch {
				continue
			}
		}

		if err := p.fsm.Transition(sig.TaskFile, sig.Event); err != nil {
			// Invalid or already-consumed transition — skip silently.
			continue
		}

		switch sig.Event {
		case taskfsm.ImplementFinished:
			actions = append(actions, SpawnReviewerAction{PlanFile: sig.TaskFile})

		case taskfsm.ReviewApproved:
			// Always emit ReviewApprovedAction so callers can perform side effects
			// (audit log, toast, ClickUp progress, reviewer pause) regardless of
			// whether a PR will be created.
			actions = append(actions, ReviewApprovedAction{
				PlanFile:   sig.TaskFile,
				ReviewBody: sig.Body,
			})
			// Additionally emit CreatePRAction only when the plan is eligible
			// (has a branch and no existing PR URL).
			if p.config.Store != nil {
				if entry, err := p.config.Store.Get(p.config.Project, sig.TaskFile); err == nil {
					if shouldCreatePR(entry) {
						actions = append(actions, CreatePRAction{
							PlanFile:   sig.TaskFile,
							ReviewBody: sig.Body,
						})
					}
				}
			}

		case taskfsm.ReviewChangesRequested:
			actions = append(actions, ReviewChangesAction{
				PlanFile: sig.TaskFile,
				Feedback: sig.Body,
			})
			if !p.config.AutoReviewFix {
				break
			}
			if p.config.MaxReviewFixCycles > 0 && p.config.Store != nil {
				if entry, err := p.config.Store.Get(p.config.Project, sig.TaskFile); err == nil {
					if entry.ReviewCycle+1 > p.config.MaxReviewFixCycles {
						actions = append(actions, ReviewCycleLimitAction{
							PlanFile: sig.TaskFile,
							Cycle:    entry.ReviewCycle + 1,
							Limit:    p.config.MaxReviewFixCycles,
						})
						break // don't spawn coder
					}
				}
			}
			actions = append(actions, IncrementReviewCycleAction{PlanFile: sig.TaskFile})
			actions = append(actions, SpawnCoderAction{
				PlanFile: sig.TaskFile,
				Feedback: sig.Body,
			})

		case taskfsm.PlannerFinished:
			actions = append(actions, PlannerCompleteAction{PlanFile: sig.TaskFile})
		}
	}
	return actions
}

// ProcessTaskSignals converts wave-task completion sentinel signals into
// TaskCompleteAction values.
//
// Extracted from app.go metadataResultMsg handler (lines 1080-1107).
func (p *Processor) ProcessTaskSignals(signals []taskfsm.TaskSignal) []Action {
	var actions []Action
	for _, ts := range signals {
		orch, exists := p.waveOrchestrators[ts.TaskFile]
		if !exists {
			continue
		}
		if ts.WaveNumber != orch.CurrentWaveNumber() {
			continue
		}
		if !orch.IsTaskRunning(ts.TaskNumber) {
			continue
		}
		orch.MarkTaskComplete(ts.TaskNumber)
		actions = append(actions, TaskCompleteAction{
			PlanFile:   ts.TaskFile,
			TaskNumber: ts.TaskNumber,
			WaveNumber: ts.WaveNumber,
		})
	}
	return actions
}

// ProcessWaveSignals converts implement-wave sentinel signals into
// AdvanceWaveAction values. It reads the plan from the store, creates a
// WaveOrchestrator, fast-forwards to the requested wave, and emits the action.
//
// Extracted from app.go metadataResultMsg handler (lines 1142-1191).
func (p *Processor) ProcessWaveSignals(signals []taskfsm.WaveSignal) []Action {
	var actions []Action
	for _, ws := range signals {
		// Reject if an orchestrator is already running for this plan.
		if _, exists := p.waveOrchestrators[ws.TaskFile]; exists {
			continue
		}

		// Read and parse the plan from the store.
		content, err := p.config.Store.GetContent(p.config.Project, ws.TaskFile)
		if err != nil {
			continue
		}
		plan, err := taskparser.Parse(content)
		if err != nil {
			continue
		}
		if ws.WaveNumber > len(plan.Waves) {
			continue
		}

		orch := orchestration.NewWaveOrchestrator(ws.TaskFile, plan)
		if p.config.Store != nil {
			orch.SetStore(p.config.Store, p.config.Project)
		}
		p.waveOrchestrators[ws.TaskFile] = orch
		p.activeWaveOrchs[ws.TaskFile] = true

		// Fast-forward through earlier waves (all tasks auto-completed).
		for i := 1; i < ws.WaveNumber; i++ {
			tasks := orch.StartNextWave()
			for _, t := range tasks {
				orch.MarkTaskComplete(t.Number)
			}
		}

		// Start the requested wave.
		orch.StartNextWave()

		actions = append(actions, AdvanceWaveAction{
			PlanFile: ws.TaskFile,
			Wave:     ws.WaveNumber,
		})
	}
	return actions
}

// ProcessElaborationSignals converts elaborator-finished sentinel signals into
// AdvanceWaveAction values. It re-reads the enriched plan from the store,
// updates the orchestrator, and emits the action to start wave 1.
//
// Extracted from app.go metadataResultMsg handler (lines 1198-1241).
func (p *Processor) ProcessElaborationSignals(signals []taskfsm.ElaborationSignal) []Action {
	var actions []Action
	for _, es := range signals {
		orch, exists := p.waveOrchestrators[es.TaskFile]
		if !exists || orch.State() != orchestration.WaveStateElaborating {
			continue
		}

		// Re-read the enriched plan from the store.
		content, err := p.config.Store.GetContent(p.config.Project, es.TaskFile)
		if err != nil {
			continue
		}
		plan, err := taskparser.Parse(content)
		if err != nil {
			continue
		}

		// Replace the plan with the elaborated version and reset orchestrator state.
		orch.UpdatePlan(plan)

		// Start wave 1.
		orch.StartNextWave()

		actions = append(actions, AdvanceWaveAction{
			PlanFile: es.TaskFile,
			Wave:     1,
		})
	}
	return actions
}

// shouldCreatePR returns true when a plan entry is eligible for automatic PR
// creation: the review has been approved, the plan is on a branch, and no PR
// has been opened yet.
func shouldCreatePR(entry taskstore.TaskEntry) bool {
	return entry.Status == taskstore.StatusDone && entry.Branch != "" && entry.PRURL == ""
}
