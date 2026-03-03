// Package planstore provides a Store interface for plan state persistence,
// with a SQLite implementation for direct DB access and an HTTP implementation
// for client-server communication.
package planstore

import "time"

// Status represents the lifecycle state of a plan.
// These constants mirror taskstate.Status to keep planstore self-contained
// and avoid circular imports.
type Status string

const (
	StatusReady        Status = "ready"
	StatusDone         Status = "done"
	StatusReviewing    Status = "reviewing"
	StatusCancelled    Status = "cancelled"
	StatusPlanning     Status = "planning"
	StatusImplementing Status = "implementing"
)

// TaskEntry holds the persisted metadata for a single plan.
type TaskEntry struct {
	Filename      string    `json:"filename"`
	Status        Status    `json:"status"`
	Description   string    `json:"description,omitempty"`
	Branch        string    `json:"branch,omitempty"`
	Topic         string    `json:"topic,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	Implemented   string    `json:"implemented,omitempty"`
	Content       string    `json:"content,omitempty"`
	ClickUpTaskID string    `json:"clickup_task_id,omitempty"`
	ReviewCycle   int       `json:"review_cycle,omitempty"`
}

// TopicEntry holds the persisted metadata for a topic grouping.
type TopicEntry struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Store is the interface for plan state persistence. Implementations include
// SQLiteStore (direct DB access, used by the server) and HTTPStore (client
// that talks to the server over HTTP).
type Store interface {
	// Plan CRUD
	Create(project string, entry TaskEntry) error
	Get(project, filename string) (TaskEntry, error)
	Update(project, filename string, entry TaskEntry) error
	Rename(project, oldFilename, newFilename string) error

	// Content access
	GetContent(project, filename string) (string, error)
	SetContent(project, filename, content string) error

	// ClickUp integration
	SetClickUpTaskID(project, filename, taskID string) error

	// Review cycle
	IncrementReviewCycle(project, filename string) error

	// Queries
	List(project string) ([]TaskEntry, error)
	ListByStatus(project string, statuses ...Status) ([]TaskEntry, error)
	ListByTopic(project, topic string) ([]TaskEntry, error)

	// Topics
	ListTopics(project string) ([]TopicEntry, error)
	CreateTopic(project string, entry TopicEntry) error

	// Health
	Ping() error

	// Close releases any resources held by the store.
	Close() error
}
