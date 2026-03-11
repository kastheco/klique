// Package planstore provides a Store interface for plan state persistence,
// with a SQLite implementation for direct DB access and an HTTP implementation
// for client-server communication.
package taskstore

import "time"

// PRReviewEntry holds a persisted PR review record for a single plan.
type PRReviewEntry struct {
	ReviewID        int       `json:"review_id"`
	ReviewState     string    `json:"review_state"`
	ReviewBody      string    `json:"review_body"`
	ReviewerLogin   string    `json:"reviewer_login"`
	ReactionPosted  bool      `json:"reaction_posted"`
	FixerDispatched bool      `json:"fixer_dispatched"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
}

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
	Filename         string    `json:"filename"`
	Status           Status    `json:"status"`
	Description      string    `json:"description,omitempty"`
	Branch           string    `json:"branch,omitempty"`
	Topic            string    `json:"topic,omitempty"`
	CreatedAt        time.Time `json:"created_at,omitempty"`
	Implemented      string    `json:"implemented,omitempty"`
	PlanningAt       time.Time `json:"planning_at,omitempty"`
	ImplementingAt   time.Time `json:"implementing_at,omitempty"`
	ReviewingAt      time.Time `json:"reviewing_at,omitempty"`
	DoneAt           time.Time `json:"done_at,omitempty"`
	Goal             string    `json:"goal,omitempty"`
	Content          string    `json:"content,omitempty"`
	ClickUpTaskID    string    `json:"clickup_task_id,omitempty"`
	ReviewCycle      int       `json:"review_cycle,omitempty"`
	PRURL            string    `json:"pr_url,omitempty"`
	PRReviewDecision string    `json:"pr_review_decision,omitempty"`
	PRCheckStatus    string    `json:"pr_check_status,omitempty"`
}

// SubtaskStatus represents the lifecycle state of a subtask.
type SubtaskStatus string

const (
	SubtaskStatusPending  SubtaskStatus = "pending"
	SubtaskStatusRunning  SubtaskStatus = "running"
	SubtaskStatusComplete SubtaskStatus = "complete"
	SubtaskStatusFailed   SubtaskStatus = "failed"
	SubtaskStatusClosed   SubtaskStatus = "closed"
	SubtaskStatusDone     SubtaskStatus = "done"
	SubtaskStatusBlocked  SubtaskStatus = "blocked"
	SubtaskStatusInReview SubtaskStatus = "in_review"
)

// SubtaskEntry holds a persisted subtask for a single plan.
type SubtaskEntry struct {
	TaskNumber int           `json:"task_number"`
	Title      string        `json:"title"`
	Status     SubtaskStatus `json:"status"`
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

	// Subtasks
	SetSubtasks(project, filename string, subtasks []SubtaskEntry) error
	GetSubtasks(project, filename string) ([]SubtaskEntry, error)
	UpdateSubtaskStatus(project, filename string, taskNumber int, status SubtaskStatus) error

	// Phase timestamps
	SetPhaseTimestamp(project, filename, phase string, ts time.Time) error

	// ClickUp integration
	SetClickUpTaskID(project, filename, taskID string) error

	// Review cycle
	IncrementReviewCycle(project, filename string) error

	// Plan goals
	SetPlanGoal(project, filename, goal string) error

	// PR metadata
	SetPRURL(project, filename, url string) error
	SetPRState(project, filename, reviewDecision, checkStatus string) error

	// PR reviews
	RecordPRReview(project, filename string, reviewID int, state, body, reviewer string) error
	IsReviewProcessed(project, filename string, reviewID int) bool
	MarkReviewReacted(project, filename string, reviewID int) error
	MarkReviewFixerDispatched(project, filename string, reviewID int) error
	ListPendingReviews(project, filename string) ([]PRReviewEntry, error)

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
