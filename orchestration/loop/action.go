// Package loop defines the action types emitted by the signal processor and
// the AgentSpawner interface used by the daemon and TUI to perform side effects.
package loop

import (
	"context"

	"github.com/kastheco/kasmos/config/taskfsm"
)

// Action is a sealed interface representing a side-effect the caller must
// perform. Implementations carry just enough data for the caller to act.
type Action interface {
	// Kind returns a stable snake_case string identifier for the action type.
	Kind() string
	sealedAction()
}

// SpawnReviewerAction instructs the caller to launch a reviewer agent for the
// given plan file.
type SpawnReviewerAction struct {
	PlanFile   string
	ReviewBody string
}

func (SpawnReviewerAction) Kind() string  { return "spawn_reviewer" }
func (SpawnReviewerAction) sealedAction() {}

// SpawnCoderAction instructs the caller to launch a coder agent.
type SpawnCoderAction struct {
	PlanFile string
	Feedback string
}

func (SpawnCoderAction) Kind() string  { return "spawn_coder" }
func (SpawnCoderAction) sealedAction() {}

// SpawnElaboratorAction instructs the caller to launch an elaborator agent.
type SpawnElaboratorAction struct {
	PlanFile string
}

func (SpawnElaboratorAction) Kind() string  { return "spawn_elaborator" }
func (SpawnElaboratorAction) sealedAction() {}

// AdvanceWaveAction instructs the caller to advance the plan to the next wave.
type AdvanceWaveAction struct {
	PlanFile string
	Wave     int
}

func (AdvanceWaveAction) Kind() string  { return "advance_wave" }
func (AdvanceWaveAction) sealedAction() {}

// CreatePRAction instructs the caller to open a pull request for the plan.
type CreatePRAction struct {
	PlanFile   string
	ReviewBody string
}

func (CreatePRAction) Kind() string  { return "create_pr" }
func (CreatePRAction) sealedAction() {}

// PlannerCompleteAction signals that the planner agent has finished and the
// plan is ready for implementation.
type PlannerCompleteAction struct {
	PlanFile string
}

func (PlannerCompleteAction) Kind() string  { return "planner_complete" }
func (PlannerCompleteAction) sealedAction() {}

// TaskCompleteAction signals that an individual task within a wave is done.
type TaskCompleteAction struct {
	PlanFile   string
	TaskNumber int
	WaveNumber int
}

func (TaskCompleteAction) Kind() string  { return "task_complete" }
func (TaskCompleteAction) sealedAction() {}

// IncrementReviewCycleAction instructs the caller to bump the review cycle
// counter for the plan (used to gate re-review after changes requested).
type IncrementReviewCycleAction struct {
	PlanFile string
}

func (IncrementReviewCycleAction) Kind() string  { return "increment_review_cycle" }
func (IncrementReviewCycleAction) sealedAction() {}

// PausePlanAgentAction instructs the caller to pause (kill) the running agent
// of the given type for the plan.
type PausePlanAgentAction struct {
	PlanFile  string
	AgentType string
}

func (PausePlanAgentAction) Kind() string  { return "pause_plan_agent" }
func (PausePlanAgentAction) sealedAction() {}

// TransitionAction is emitted for audit/logging hooks whenever a state
// transition should be recorded. It does not imply a spawning side effect.
type TransitionAction struct {
	PlanFile string
	Event    taskfsm.Event
}

func (TransitionAction) Kind() string  { return "transition" }
func (TransitionAction) sealedAction() {}

// ---------------------------------------------------------------------------
// AgentSpawner
// ---------------------------------------------------------------------------

// SpawnOpts carries the parameters needed to launch any agent type.
type SpawnOpts struct {
	// PlanFile is the path to the plan markdown file relative to the repo root.
	PlanFile string
	// AgentType identifies the agent role: coder, reviewer, elaborator, etc.
	AgentType string
	// RepoPath is the absolute filesystem path to the repository root.
	RepoPath string
	// Branch is the git branch for the plan's shared worktree.
	Branch string
	// Prompt is the initial prompt delivered to the agent on startup.
	Prompt string
	// Program is the agent executable command (e.g. "opencode", "claude").
	Program string
	// Feedback is forwarded to coder agents as review feedback (may be empty).
	Feedback string
	// Wave is the current wave number (used to set KASMOS_WAVE env var).
	Wave int
}

// AgentSpawner abstracts tmux session management so the daemon and TUI can
// share the same SignalProcessor logic while using different backends.
type AgentSpawner interface {
	// SpawnReviewer launches a reviewer agent for the given plan.
	SpawnReviewer(ctx context.Context, opts SpawnOpts) error
	// SpawnCoder launches a coder agent for the given plan.
	SpawnCoder(ctx context.Context, opts SpawnOpts) error
	// SpawnElaborator launches an elaborator agent for the given plan.
	SpawnElaborator(ctx context.Context, opts SpawnOpts) error
	// KillAgent stops the running agent of the given type for the plan.
	KillAgent(planFile, agentType string) error
}
