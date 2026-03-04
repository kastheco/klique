package orchestration

import (
	"github.com/kastheco/kasmos/config/taskparser"
)

// WaveState represents the current state of wave orchestration for a plan.
type WaveState int

const (
	WaveStateIdle         WaveState = iota // Not started
	WaveStateElaborating                   // Waiting for elaborator to enrich task descriptions
	WaveStateRunning                       // Current wave's tasks are running
	WaveStateWaveComplete                  // Current wave finished, awaiting user confirmation
	WaveStateAllComplete                   // All waves finished
)

// taskStatus tracks the completion state of a single task.
type taskStatus int

const (
	taskPending taskStatus = iota
	taskRunning
	taskComplete
	taskFailed
)

// WaveOrchestrator manages wave-based parallel task execution for a single plan.
type WaveOrchestrator struct {
	taskFile          string
	plan              *taskparser.Plan
	state             WaveState
	currentWave       int                // 0-indexed into plan.Waves
	taskStates        map[int]taskStatus // task number → status
	waitingForConfirm bool               // true once we've shown the wave-complete dialog
}

// NewWaveOrchestrator creates an orchestrator for the given plan.
func NewWaveOrchestrator(planFile string, plan *taskparser.Plan) *WaveOrchestrator {
	return &WaveOrchestrator{
		taskFile:   planFile,
		plan:       plan,
		state:      WaveStateIdle,
		taskStates: make(map[int]taskStatus),
	}
}

// State returns the current orchestration state.
func (o *WaveOrchestrator) State() WaveState {
	return o.state
}

// TaskFile returns the plan filename this orchestrator manages.
func (o *WaveOrchestrator) TaskFile() string {
	return o.taskFile
}

// TotalWaves returns the number of waves in the plan.
func (o *WaveOrchestrator) TotalWaves() int {
	return len(o.plan.Waves)
}

// TotalTasks returns the total number of tasks across all waves.
func (o *WaveOrchestrator) TotalTasks() int {
	total := 0
	for _, w := range o.plan.Waves {
		total += len(w.Tasks)
	}
	return total
}

// CurrentWaveNumber returns the 1-indexed wave number currently active.
func (o *WaveOrchestrator) CurrentWaveNumber() int {
	if o.currentWave >= len(o.plan.Waves) {
		return 0
	}
	return o.plan.Waves[o.currentWave].Number
}

// CurrentWaveTasks returns the tasks in the current wave.
func (o *WaveOrchestrator) CurrentWaveTasks() []taskparser.Task {
	if o.currentWave >= len(o.plan.Waves) {
		return nil
	}
	return o.plan.Waves[o.currentWave].Tasks
}

// SetElaborating puts the orchestrator into the elaborating state.
// StartNextWave is blocked until UpdatePlan is called.
func (o *WaveOrchestrator) SetElaborating() {
	o.state = WaveStateElaborating
}

// UpdatePlan replaces the plan with an elaborated version and resets the
// orchestrator to Idle so waves can begin. Task states are cleared since
// no tasks have started yet.
func (o *WaveOrchestrator) UpdatePlan(plan *taskparser.Plan) {
	o.plan = plan
	o.state = WaveStateIdle
	o.currentWave = 0
	o.taskStates = make(map[int]taskStatus)
}

// StartNextWave advances to the next wave and returns its tasks.
// Returns nil if all waves are complete or if elaboration is in progress.
func (o *WaveOrchestrator) StartNextWave() []taskparser.Task {
	if o.state == WaveStateElaborating {
		return nil
	}
	if o.state == WaveStateAllComplete {
		return nil
	}
	if o.state == WaveStateWaveComplete {
		o.currentWave++
		o.waitingForConfirm = false // reset for next wave
	}
	if o.currentWave >= len(o.plan.Waves) {
		o.state = WaveStateAllComplete
		return nil
	}

	o.state = WaveStateRunning
	tasks := o.plan.Waves[o.currentWave].Tasks
	for _, t := range tasks {
		o.taskStates[t.Number] = taskRunning
	}
	return tasks
}

// MarkTaskComplete marks a task as successfully completed.
// If all tasks in the current wave are done, transitions state.
// Idempotent: calling again on an already-resolved task is a no-op.
func (o *WaveOrchestrator) MarkTaskComplete(taskNumber int) {
	if o.taskStates[taskNumber] != taskRunning {
		return
	}
	o.taskStates[taskNumber] = taskComplete
	o.checkWaveComplete()
}

// MarkTaskFailed marks a task as failed.
// Other tasks in the wave continue. Wave completes when all tasks resolve.
// Idempotent: calling again on an already-resolved task is a no-op.
func (o *WaveOrchestrator) MarkTaskFailed(taskNumber int) {
	if o.taskStates[taskNumber] != taskRunning {
		return
	}
	o.taskStates[taskNumber] = taskFailed
	o.checkWaveComplete()
}

// NeedsConfirm returns true if the wave just completed and the user hasn't
// been shown the confirmation dialog yet. Calling this marks the dialog as shown.
func (o *WaveOrchestrator) NeedsConfirm() bool {
	if o.state == WaveStateWaveComplete && !o.waitingForConfirm {
		o.waitingForConfirm = true
		return true
	}
	return false
}

// ResetConfirm resets the one-shot confirm latch so NeedsConfirm() can return true
// again on the next check. Call this when the user cancels a wave-advance confirmation
// so the prompt re-appears on the subsequent metadata tick (fixes deadlock).
func (o *WaveOrchestrator) ResetConfirm() {
	o.waitingForConfirm = false
}

// RetryFailedTasks transitions all failed tasks in the current wave back to running
// and sets the orchestrator state to WaveStateRunning. Returns the tasks that were
// retried (previously failed). Returns nil if there are no failed tasks to retry.
func (o *WaveOrchestrator) RetryFailedTasks() []taskparser.Task {
	if o.currentWave >= len(o.plan.Waves) {
		return nil
	}
	var tasks []taskparser.Task
	for _, t := range o.plan.Waves[o.currentWave].Tasks {
		if o.taskStates[t.Number] == taskFailed {
			o.taskStates[t.Number] = taskRunning
			tasks = append(tasks, t)
		}
	}
	if len(tasks) > 0 {
		o.state = WaveStateRunning
		o.waitingForConfirm = false
	}
	return tasks
}

// IsCurrentWaveComplete returns true if all tasks in the current wave have resolved.
func (o *WaveOrchestrator) IsCurrentWaveComplete() bool {
	return o.state == WaveStateWaveComplete || o.state == WaveStateAllComplete
}

// CompletedTaskCount returns the number of completed tasks in the current wave.
func (o *WaveOrchestrator) CompletedTaskCount() int {
	return o.countCurrentWaveByStatus(taskComplete)
}

// FailedTaskCount returns the number of failed tasks in the current wave.
func (o *WaveOrchestrator) FailedTaskCount() int {
	return o.countCurrentWaveByStatus(taskFailed)
}

// IsTaskRunning returns true if the given task number is currently in the running state.
// Used to gate the "Mark complete" context menu action.
func (o *WaveOrchestrator) IsTaskRunning(taskNumber int) bool {
	return o.taskStates[taskNumber] == taskRunning
}

// IsTaskComplete returns true if the given task number has completed successfully.
func (o *WaveOrchestrator) IsTaskComplete(taskNumber int) bool {
	return o.taskStates[taskNumber] == taskComplete
}

// IsTaskFailed returns true if the given task number has failed.
func (o *WaveOrchestrator) IsTaskFailed(taskNumber int) bool {
	return o.taskStates[taskNumber] == taskFailed
}

// HeaderContext returns the plan header for inclusion in task prompts.
func (o *WaveOrchestrator) HeaderContext() string {
	return o.plan.HeaderContext()
}

// ShouldPostWaveCompleteComment returns true when an intermediate wave_complete
// comment should be posted. Single-wave plans return false — they use the
// "all waves complete" event instead. Nil-receiver safe.
func (o *WaveOrchestrator) ShouldPostWaveCompleteComment() bool {
	return o != nil && o.TotalWaves() > 1
}

// RestoreToWave fast-forwards the orchestrator wave-by-wave to targetWave,
// auto-completing all tasks in earlier waves, then marks the specified task
// numbers as complete in the target wave. Remaining tasks in the target wave
// are left in the running state.
func (o *WaveOrchestrator) RestoreToWave(targetWave int, completedTasks []int) {
	completedSet := make(map[int]bool, len(completedTasks))
	for _, n := range completedTasks {
		completedSet[n] = true
	}

	for o.currentWave < len(o.plan.Waves) {
		o.StartNextWave()
		if o.CurrentWaveNumber() == targetWave {
			break
		}
		// Mark all tasks in this earlier wave as complete to advance.
		for _, t := range o.plan.Waves[o.currentWave].Tasks {
			o.MarkTaskComplete(t.Number)
		}
	}

	// Apply the actual task states for the target wave.
	for _, t := range o.plan.Waves[o.currentWave].Tasks {
		if completedSet[t.Number] {
			o.MarkTaskComplete(t.Number)
		}
		// Running tasks stay in taskRunning.
	}
}

// BuildTaskPrompt is a convenience wrapper that builds the task prompt for a
// task in the current wave.
func (o *WaveOrchestrator) BuildTaskPrompt(task taskparser.Task, peerCount int) string {
	return BuildTaskPrompt(o.plan, task, o.CurrentWaveNumber(), o.TotalWaves(), peerCount)
}

func (o *WaveOrchestrator) checkWaveComplete() {
	if o.currentWave >= len(o.plan.Waves) {
		return
	}
	tasks := o.plan.Waves[o.currentWave].Tasks
	for _, t := range tasks {
		s := o.taskStates[t.Number]
		if s == taskRunning || s == taskPending {
			return // still in progress
		}
	}
	// All tasks resolved — check if more waves remain
	if o.currentWave+1 >= len(o.plan.Waves) {
		o.state = WaveStateAllComplete
	} else {
		o.state = WaveStateWaveComplete
	}
}

func (o *WaveOrchestrator) countCurrentWaveByStatus(s taskStatus) int {
	if o.currentWave >= len(o.plan.Waves) {
		return 0
	}
	count := 0
	for _, t := range o.plan.Waves[o.currentWave].Tasks {
		if o.taskStates[t.Number] == s {
			count++
		}
	}
	return count
}
