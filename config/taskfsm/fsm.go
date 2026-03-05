package taskfsm

import (
	"fmt"
	"time"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
)

// Status represents the lifecycle state of a plan.
type Status string

const (
	StatusReady        Status = "ready"
	StatusPlanning     Status = "planning"
	StatusImplementing Status = "implementing"
	StatusReviewing    Status = "reviewing"
	StatusDone         Status = "done"
	StatusCancelled    Status = "cancelled"
)

// Event represents a lifecycle transition trigger.
type Event string

const (
	PlanStart              Event = "plan_start"
	PlannerFinished        Event = "planner_finished"
	ImplementStart         Event = "implement_start"
	ImplementFinished      Event = "implement_finished"
	ReviewApproved         Event = "review_approved"
	ReviewChangesRequested Event = "review_changes_requested"
	RequestReview          Event = "request_review"
	StartOver              Event = "start_over"
	Reimplement            Event = "reimplement"
	Cancel                 Event = "cancel"
	Reopen                 Event = "reopen"
)

// IsUserOnly returns true if this event can only be triggered from the TUI,
// never by agent sentinel files.
func (e Event) IsUserOnly() bool {
	switch e {
	case StartOver, Reimplement, RequestReview, Cancel, Reopen:
		return true
	}
	return false
}

// transitionTable defines all valid state transitions.
// Key: current status → event → new status.
var transitionTable = map[Status]map[Event]Status{
	StatusReady: {
		PlanStart:      StatusPlanning,
		ImplementStart: StatusImplementing,
		Cancel:         StatusCancelled,
	},
	StatusPlanning: {
		PlanStart:       StatusPlanning, // allow restart after crash/interrupt
		PlannerFinished: StatusReady,
		Cancel:          StatusCancelled,
	},
	StatusImplementing: {
		ImplementFinished: StatusReviewing,
		Cancel:            StatusCancelled,
	},
	StatusReviewing: {
		ReviewApproved:         StatusDone,
		ReviewChangesRequested: StatusImplementing,
		Cancel:                 StatusCancelled,
	},
	StatusDone: {
		StartOver:     StatusPlanning,
		Reimplement:   StatusImplementing, // resume implementation without resetting branch
		RequestReview: StatusReviewing,    // retrigger review for unmerged branches
		Cancel:        StatusCancelled,    // explicit user cancellation from done
	},
	StatusCancelled: {
		Reopen: StatusPlanning,
	},
}

// ApplyTransition returns the new status for the given current status and event.
// Returns an error if the transition is not valid.
func ApplyTransition(current Status, event Event) (Status, error) {
	events, ok := transitionTable[current]
	if !ok {
		return "", fmt.Errorf("no transitions defined for status %q", current)
	}
	next, ok := events[event]
	if !ok {
		return "", fmt.Errorf("invalid transition: %q + %q", current, event)
	}
	return next, nil
}

// TaskStateMachine is the sole writer of plan state. All plan status mutations
// must flow through Transition(). The store handles concurrency via SQLite.
type TaskStateMachine struct {
	dir     string          // legacy: retained for file rename operations (may be empty)
	store   taskstore.Store // always non-nil
	project string          // project name used with the store
}

// New creates a TaskStateMachine backed by the given store.
func New(store taskstore.Store, project, dir string) *TaskStateMachine {
	return &TaskStateMachine{dir: dir, store: store, project: project}
}

// Transition applies an event to a plan's current status. It reads the current
// state from the store, validates the transition, writes the new state, and returns.
// Concurrency is handled server-side via SQLite's own locking.
func (m *TaskStateMachine) Transition(planFile string, event Event) error {
	ps, err := taskstate.Load(m.store, m.project, m.dir)
	if err != nil {
		return fmt.Errorf("load plan state: %w", err)
	}
	entry, ok := ps.Entry(planFile)
	if !ok {
		return fmt.Errorf("plan not found: %s", planFile)
	}
	currentStatus := mapLegacyStatus(entry.Status)
	newStatus, err := ApplyTransition(currentStatus, event)
	if err != nil {
		return err
	}
	// ForceSetStatus writes through to the store.
	if err := ps.ForceSetStatus(planFile, taskstate.Status(newStatus)); err != nil {
		return err
	}
	if phase, ok := phaseNameForStatus(newStatus); ok {
		if err := m.store.SetPhaseTimestamp(m.project, planFile, phase, time.Now().UTC()); err != nil {
			return fmt.Errorf("set phase timestamp: %w", err)
		}
	}
	return nil
}

func phaseNameForStatus(s Status) (string, bool) {
	switch s {
	case StatusPlanning:
		return "planning", true
	case StatusImplementing:
		return "implementing", true
	case StatusReviewing:
		return "reviewing", true
	case StatusDone:
		return "done", true
	default:
		return "", false
	}
}

// mapLegacyStatus converts old planstate statuses to FSM statuses.
// Handles the consolidated aliases (in_progress → implementing, completed/finished → done).
func mapLegacyStatus(s taskstate.Status) Status {
	switch s {
	case "in_progress":
		return StatusImplementing
	case "completed", "finished":
		return StatusDone
	default:
		return Status(s)
	}
}
