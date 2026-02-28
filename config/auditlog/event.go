package auditlog

import "time"

// EventKind identifies the type of audit event.
type EventKind string

// String returns the string representation of the EventKind.
func (k EventKind) String() string {
	return string(k)
}

// Lifecycle events.
const (
	EventAgentSpawned  EventKind = "agent_spawned"
	EventAgentFinished EventKind = "agent_finished"
	EventAgentKilled   EventKind = "agent_killed"
	EventAgentPaused   EventKind = "agent_paused"
	EventAgentResumed  EventKind = "agent_resumed"
)

// Plan events.
const (
	EventPlanTransition EventKind = "plan_transition"
	EventPlanCreated    EventKind = "plan_created"
	EventPlanMerged     EventKind = "plan_merged"
	EventPlanCancelled  EventKind = "plan_cancelled"
)

// Wave events.
const (
	EventWaveStarted   EventKind = "wave_started"
	EventWaveCompleted EventKind = "wave_completed"
	EventWaveFailed    EventKind = "wave_failed"
)

// Operational events.
const (
	EventPromptSent         EventKind = "prompt_sent"
	EventGitPush            EventKind = "git_push"
	EventPRCreated          EventKind = "pr_created"
	EventPermissionDetected EventKind = "permission_detected"
	EventPermissionAnswered EventKind = "permission_answered"
	EventFSMError           EventKind = "fsm_error"
	EventError              EventKind = "error"
)

// Event is a single audit log entry.
type Event struct {
	ID            int64
	Kind          EventKind
	Timestamp     time.Time
	Project       string
	PlanFile      string
	InstanceTitle string
	AgentType     string
	WaveNumber    int
	TaskNumber    int
	Message       string
	Detail        string // JSON-encoded extra data
	Level         string // info, warn, error
}
