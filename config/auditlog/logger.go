package auditlog

import "time"

// QueryFilter specifies criteria for querying audit events.
type QueryFilter struct {
	Project       string
	PlanFile      string
	InstanceTitle string
	Kinds         []EventKind
	Limit         int
	Before        time.Time
	After         time.Time
}

// Logger is the interface for emitting and querying audit events.
type Logger interface {
	Emit(event Event)
	Query(filter QueryFilter) ([]Event, error)
	Close() error
}

// EventOption is a functional option for configuring optional Event fields.
type EventOption func(*Event)

// WithPlan sets the PlanFile field on the event.
func WithPlan(planFile string) EventOption {
	return func(e *Event) { e.PlanFile = planFile }
}

// WithInstance sets the InstanceTitle field on the event.
func WithInstance(title string) EventOption {
	return func(e *Event) { e.InstanceTitle = title }
}

// WithAgent sets the AgentType field on the event.
func WithAgent(agentType string) EventOption {
	return func(e *Event) { e.AgentType = agentType }
}

// WithWave sets the WaveNumber and TaskNumber fields on the event.
func WithWave(wave, task int) EventOption {
	return func(e *Event) {
		e.WaveNumber = wave
		e.TaskNumber = task
	}
}

// WithDetail sets the Detail field on the event (JSON-encoded extra data).
func WithDetail(detail string) EventOption {
	return func(e *Event) { e.Detail = detail }
}

// WithLevel sets the Level field on the event (info, warn, error).
func WithLevel(level string) EventOption {
	return func(e *Event) { e.Level = level }
}

// nopLogger is a no-op Logger used when planstore is unconfigured.
type nopLogger struct{}

// NopLogger returns a Logger that discards all events.
func NopLogger() Logger {
	return &nopLogger{}
}

func (n *nopLogger) Emit(_ Event) {}

func (n *nopLogger) Query(_ QueryFilter) ([]Event, error) {
	return nil, nil
}

func (n *nopLogger) Close() error {
	return nil
}
