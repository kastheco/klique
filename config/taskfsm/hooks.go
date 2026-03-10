package taskfsm

import (
	"context"
	"log/slog"
	"time"
)

const hookTimeout = 30 * time.Second

// TransitionEvent carries information about a completed FSM state transition.
// It is passed to every matching HookRunner after the state write succeeds.
type TransitionEvent struct {
	PlanFile   string    `json:"plan_file"`
	FromStatus Status    `json:"from_status"`
	ToStatus   Status    `json:"to_status"`
	Event      Event     `json:"event"`
	Timestamp  time.Time `json:"timestamp"`
	Project    string    `json:"project"`
}

// HookRunner is implemented by any type that wants to react to FSM transitions.
type HookRunner interface {
	Name() string
	Run(context.Context, TransitionEvent) error
}

// hookEntry pairs a runner with an optional event filter.
type hookEntry struct {
	runner HookRunner
	filter map[Event]bool // nil means fire on every event
}

// HookRegistry holds the set of registered hooks. Hooks are registered once
// at startup before transitions begin; no mutex is required.
type HookRegistry struct {
	entries []hookEntry
}

// NewHookRegistry returns an empty HookRegistry.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{}
}

// Add registers a HookRunner. If events is nil or empty the runner fires on
// every transition; otherwise it fires only when the transition event is in
// the provided list.
func (r *HookRegistry) Add(runner HookRunner, events []Event) {
	var filter map[Event]bool
	if len(events) > 0 {
		filter = make(map[Event]bool, len(events))
		for _, e := range events {
			filter[e] = true
		}
	}
	r.entries = append(r.entries, hookEntry{runner: runner, filter: filter})
}

// Len returns the number of registered hooks. Safe on a nil registry.
func (r *HookRegistry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.entries)
}

// FireAll dispatches ev to all matching hooks. It returns immediately; each
// hook runs in its own goroutine with a 30-second timeout. Failures are
// logged via slog.Warn and never returned to the caller. Safe on a nil registry.
func (r *HookRegistry) FireAll(ev TransitionEvent) {
	if r == nil {
		return
	}
	for _, entry := range r.entries {
		if entry.filter != nil && !entry.filter[ev.Event] {
			continue
		}
		e := entry // capture for goroutine
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
			defer cancel()
			if err := e.runner.Run(ctx, ev); err != nil {
				slog.Warn("hook failed", "hook", e.runner.Name(), "error", err)
			}
		}()
	}
}
