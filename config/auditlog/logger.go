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
