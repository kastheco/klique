package planfsm

import "fmt"

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
	StartOver              Event = "start_over"
	Cancel                 Event = "cancel"
	Reopen                 Event = "reopen"
)

// IsUserOnly returns true if this event can only be triggered from the TUI,
// never by agent sentinel files.
func (e Event) IsUserOnly() bool {
	switch e {
	case StartOver, Cancel, Reopen:
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
		StartOver: StatusPlanning,
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
