package taskstore

import "time"

// SignalStatus represents the lifecycle state of a signal.
type SignalStatus string

const (
	SignalPending    SignalStatus = "pending"
	SignalProcessing SignalStatus = "processing"
	SignalDone       SignalStatus = "done"
	SignalFailed     SignalStatus = "failed"
)

// SignalEntry is a single row in the signals table.
type SignalEntry struct {
	ID          int64
	Project     string
	PlanFile    string
	SignalType  string
	Payload     string
	Status      SignalStatus
	CreatedAt   time.Time
	ClaimedBy   string
	ClaimedAt   time.Time
	ProcessedAt time.Time
	Result      string
}

// SignalGateway is the persistence abstraction for signals.
type SignalGateway interface {
	// Create inserts a new pending signal for the given project.
	Create(project string, entry SignalEntry) error
	// List returns all signals for the given project matching any of the provided statuses.
	// If no statuses are given, it returns nil, nil.
	List(project string, statuses ...SignalStatus) ([]SignalEntry, error)
	// Claim atomically claims the oldest pending signal for the given project and marks
	// it as processing. Returns nil, nil when no pending signal is available.
	Claim(project, claimedBy string) (*SignalEntry, error)
	// MarkProcessed sets the final status, result, and processed_at timestamp on a signal.
	MarkProcessed(id int64, status SignalStatus, result string) error
	// ResetStuck resets signals that have been in "processing" state for longer than
	// olderThan, returning them to "pending" so they can be reclaimed.
	ResetStuck(olderThan time.Duration) (int, error)
	// Close releases the underlying database connection.
	Close() error
}
