package session

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/kastheco/kasmos/session/git"
)

// Status represents the current state of an instance.
type Status int

const (
	// Running indicates the instance is active and the agent is working.
	Running Status = iota
	// Ready indicates the instance is idle and waiting for user input.
	Ready
	// Loading indicates the instance is starting up or initialising.
	Loading
	// Paused indicates the instance is paused — the worktree has been removed but the branch is preserved.
	Paused
)

// AgentType constants identify the role of an agent session within a plan lifecycle.
const (
	AgentTypePlanner    = "planner"
	AgentTypeCoder      = "coder"
	AgentTypeReviewer   = "reviewer"
	AgentTypeFixer      = "fixer"
	AgentTypeElaborator = "architect"
)

// Instance represents a managed agent session with its associated execution backend and git state.
type Instance struct {
	// Title is the display name and tmux session identifier for this instance.
	Title string
	// Path is the workspace directory for the instance.
	Path string
	// Branch is the git branch associated with this instance.
	Branch string
	// Status is the current lifecycle state of the instance.
	Status Status
	// Program is the agent command to execute within the session.
	Program string
	// ExecutionMode determines how the agent process is hosted (tmux or headless).
	ExecutionMode ExecutionMode
	// Height is the terminal height in rows.
	Height int
	// Width is the terminal width in columns.
	Width int
	// CreatedAt records when the instance was first created.
	CreatedAt time.Time
	// UpdatedAt records the most recent update timestamp.
	UpdatedAt time.Time
	// AutoYes causes the instance to auto-confirm prompts.
	AutoYes bool
	// SkipPermissions enables the --dangerously-skip-permissions flag for Claude.
	SkipPermissions bool
	// TaskFile is the plan file this instance is implementing (empty for ad-hoc sessions).
	TaskFile string
	// Topic is the plan-state group this instance belongs to.
	Topic string
	// AgentType identifies the role within a plan lifecycle: planner, coder, reviewer, fixer, or empty.
	AgentType string
	// TaskNumber is the 1-indexed task number within a plan wave (0 = not a wave task).
	TaskNumber int
	// WaveNumber is the 1-indexed wave number this task belongs to (0 = not a wave task).
	WaveNumber int
	// PeerCount is the number of concurrent sibling tasks in the same wave (0 = not a wave task).
	PeerCount int
	// IsReviewer indicates a reviewer session.
	// Deprecated: check AgentType == AgentTypeReviewer instead.
	IsReviewer bool
	// ImplementationComplete is set when the coder finishes and the plan transitions to review.
	ImplementationComplete bool
	// SoloAgent is true for instances launched as standalone agents outside the orchestration lifecycle.
	SoloAgent bool
	// Exited is true when the instance's tmux session has terminated unexpectedly.
	Exited bool
	// QueuedPrompt is delivered to the session on first transition to Ready. Cleared after delivery.
	QueuedPrompt string

	// sharedWorktree indicates the instance shares a topic worktree and should not clean it up.
	sharedWorktree bool
	// LoadingStage tracks the current startup progress step for the UI.
	LoadingStage int
	// LoadingTotal is the total count of startup stages.
	LoadingTotal int
	// LoadingMessage describes the current startup step shown in the UI.
	LoadingMessage string

	// Notified is true after the instance completes (Running→Ready) until the user selects it.
	Notified bool

	// LastActiveAt records the most recent time the instance entered Running or Loading state.
	LastActiveAt time.Time

	// PromptDetected is true when the agent program is waiting for user input.
	// Persists across status transitions to prevent UI flicker.
	PromptDetected bool

	// AwaitingWork is set when a QueuedPrompt is dispatched and cleared when the agent goes Running.
	// The wave orchestrator uses this to avoid treating early idle prompts as task completion.
	AwaitingWork bool

	// ReviewCycle is the 1-indexed count of review/fix cycles for this instance (0 = not a cycle instance).
	ReviewCycle int

	// HasWorked is true once the agent produces at least one content update after receiving its task.
	// Prevents permission prompts or early returns from prematurely completing a wave.
	HasWorked bool

	// CPUPercent is the last sampled CPU utilisation of the agent process.
	CPUPercent float64
	// MemMB is the last sampled memory usage of the agent process in megabytes.
	MemMB float64

	// LastActivity is the most recently detected agent activity event (ephemeral, not persisted).
	LastActivity *Activity

	// CachedContent is the last tmux pane capture, kept to avoid redundant subprocess calls.
	CachedContent string
	// CachedContentSet is true once CachedContent has been populated for the first time.
	CachedContentSet bool

	// started is true once Start() has been called successfully.
	started bool
	// executionSession manages the underlying process host (tmux or headless) for this instance.
	executionSession ExecutionSession
	// gitWorktree manages the git worktree associated with this instance.
	gitWorktree *git.GitWorktree
}

// ToInstanceData converts an Instance to its JSON-serialisable form for persistence.
// UpdatedAt is always refreshed to the current time.
func (i *Instance) ToInstanceData() InstanceData {
	data := InstanceData{
		Title:                  i.Title,
		Path:                   i.Path,
		Branch:                 i.Branch,
		Status:                 i.Status,
		Height:                 i.Height,
		Width:                  i.Width,
		CreatedAt:              i.CreatedAt,
		UpdatedAt:              time.Now(),
		Program:                i.Program,
		ExecutionMode:          NormalizeExecutionMode(i.ExecutionMode),
		AutoYes:                i.AutoYes,
		SkipPermissions:        i.SkipPermissions,
		TaskFile:               i.TaskFile,
		AgentType:              i.AgentType,
		TaskNumber:             i.TaskNumber,
		WaveNumber:             i.WaveNumber,
		PeerCount:              i.PeerCount,
		IsReviewer:             i.IsReviewer,
		ImplementationComplete: i.ImplementationComplete,
		SoloAgent:              i.SoloAgent,
		QueuedPrompt:           i.QueuedPrompt,
		ReviewCycle:            i.ReviewCycle,
	}

	if i.gitWorktree != nil {
		data.Worktree = GitWorktreeData{
			RepoPath:      i.gitWorktree.GetRepoPath(),
			WorktreePath:  i.gitWorktree.GetWorktreePath(),
			SessionName:   i.Title,
			BranchName:    i.gitWorktree.GetBranchName(),
			BaseCommitSHA: i.gitWorktree.GetBaseCommitSHA(),
		}
	}

	return data
}

// FromInstanceData reconstructs an Instance from its serialised form.
// Empty or unknown ExecutionMode is normalised to tmux before constructing the session.
// For paused instances the execution session is prepared but not started.
// For live instances the session is reattached; dead sessions are marked Exited.
func FromInstanceData(data InstanceData) (*Instance, error) {
	// Normalise empty/unknown mode to tmux for backward compatibility.
	mode := NormalizeExecutionMode(data.ExecutionMode)

	instance := &Instance{
		Title:                  data.Title,
		Path:                   data.Path,
		Branch:                 data.Branch,
		Status:                 data.Status,
		Height:                 data.Height,
		Width:                  data.Width,
		CreatedAt:              data.CreatedAt,
		UpdatedAt:              data.UpdatedAt,
		Program:                data.Program,
		ExecutionMode:          mode,
		SkipPermissions:        data.SkipPermissions,
		TaskFile:               data.TaskFile,
		AgentType:              data.AgentType,
		TaskNumber:             data.TaskNumber,
		WaveNumber:             data.WaveNumber,
		PeerCount:              data.PeerCount,
		IsReviewer:             data.IsReviewer,
		ImplementationComplete: data.ImplementationComplete,
		SoloAgent:              data.SoloAgent,
		QueuedPrompt:           data.QueuedPrompt,
		ReviewCycle:            data.ReviewCycle,
		gitWorktree: git.NewGitWorktreeFromStorage(
			data.Worktree.RepoPath,
			data.Worktree.WorktreePath,
			data.Worktree.SessionName,
			data.Worktree.BranchName,
			data.Worktree.BaseCommitSHA,
		),
	}

	if instance.Paused() {
		// Paused instances keep the session struct ready but do not reattach.
		instance.started = true
		es := NewExecutionSession(mode, instance.Title, instance.Program, instance.SkipPermissions)
		es.SetAgentType(instance.AgentType)
		instance.executionSession = es
		return instance, nil
	}

	// Build the execution session handle and check liveness before attempting a full restore.
	es := NewExecutionSession(mode, instance.Title, instance.Program, instance.SkipPermissions)
	es.SetAgentType(instance.AgentType)
	instance.executionSession = es

	if !es.DoesSessionExist() {
		// The session is gone — mark as exited so the UI can display it as dead.
		instance.started = true
		instance.Exited = true
		instance.SetStatus(Ready)
		return instance, nil
	}

	// Session is alive — restore the full attachment via Start(false).
	if err := instance.Start(false); err != nil {
		return nil, err
	}

	return instance, nil
}

// InstanceOptions holds the configuration values for creating a new Instance.
type InstanceOptions struct {
	// Title is the display name and session identifier.
	Title string
	// Path is the workspace directory.
	Path string
	// Program is the agent command to run (e.g. "claude", "opencode").
	Program string
	// ExecutionMode selects the process host backend (tmux or headless).
	// Empty string defaults to ExecutionModeTmux.
	ExecutionMode ExecutionMode
	// AutoYes enables automatic confirmation of agent prompts.
	AutoYes bool
	// SkipPermissions enables --dangerously-skip-permissions for Claude.
	SkipPermissions bool
	// TaskFile binds this instance to a plan from plan-state.
	TaskFile string
	// AgentType is the role of this instance within a plan: planner, coder, reviewer, fixer, or empty.
	AgentType string
	// TaskNumber is the 1-indexed task number within a plan wave (0 = not a wave task).
	TaskNumber int
	// WaveNumber is the 1-indexed wave this task belongs to (0 = not a wave task).
	WaveNumber int
	// PeerCount is the number of concurrent sibling tasks in the same wave.
	PeerCount int
	// ReviewCycle is the 1-indexed review/fix cycle number (0 = not a cycle instance).
	ReviewCycle int
}

// NewInstance constructs a new unstarted Instance from the given options.
// The workspace path is resolved to an absolute path before storage.
func NewInstance(opts InstanceOptions) (*Instance, error) {
	now := time.Now()

	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	return &Instance{
		Title:           opts.Title,
		Status:          Ready,
		Path:            absPath,
		Program:         opts.Program,
		ExecutionMode:   NormalizeExecutionMode(opts.ExecutionMode),
		Height:          0,
		Width:           0,
		CreatedAt:       now,
		UpdatedAt:       now,
		AutoYes:         opts.AutoYes,
		SkipPermissions: opts.SkipPermissions,
		TaskFile:        opts.TaskFile,
		AgentType:       opts.AgentType,
		TaskNumber:      opts.TaskNumber,
		WaveNumber:      opts.WaveNumber,
		PeerCount:       opts.PeerCount,
		ReviewCycle:     opts.ReviewCycle,
	}, nil
}

// RepoName returns the repository name for this instance.
// For instances without a git worktree (e.g. planner sessions on the main branch),
// the repo name is derived from the workspace path.
func (i *Instance) RepoName() (string, error) {
	if !i.started {
		return "", fmt.Errorf("cannot get repo name for instance that has not been started")
	}
	if i.gitWorktree == nil {
		return filepath.Base(i.Path), nil
	}
	return i.gitWorktree.GetRepoName(), nil
}

// GetRepoPath returns the repository root path, or empty string if the instance is not started
// or has no git worktree.
func (i *Instance) GetRepoPath() string {
	if !i.started || i.gitWorktree == nil {
		return ""
	}
	return i.gitWorktree.GetRepoPath()
}

// GetWorktreePath returns the worktree directory path, or empty string if unavailable.
func (i *Instance) GetWorktreePath() string {
	if i.gitWorktree == nil {
		return ""
	}
	return i.gitWorktree.GetWorktreePath()
}

// SetStatus transitions the instance to the given status and triggers associated side-effects:
// desktop notification on Running→Ready, timestamp refresh on Running/Loading, and
// AwaitingWork clear on Running.
func (i *Instance) SetStatus(status Status) {
	if i.Status == Running && status == Ready {
		i.Notified = true
		// Wave task instances are managed collectively by the orchestrator.
		// Only send per-instance notifications for standalone (non-wave) sessions.
		if i.TaskNumber == 0 {
			SendNotification("kas", fmt.Sprintf("'%s' has finished", i.Title))
		}
	}

	if status == Running || status == Loading {
		i.LastActiveAt = time.Now()
		i.PromptDetected = false
		i.Notified = false
	}

	if status == Running {
		i.AwaitingWork = false
	}

	i.Status = status
}

// setLoadingProgress updates the loading stage and message shown during startup.
func (i *Instance) setLoadingProgress(stage int, message string) {
	i.LoadingStage = stage
	i.LoadingMessage = message
}

// Started reports whether the instance has been started via Start().
func (i *Instance) Started() bool {
	return i.started
}

// SetTitle updates the instance title. Returns an error if the instance has already started,
// since the title is used as the tmux session name and cannot be changed after creation.
func (i *Instance) SetTitle(title string) error {
	if i.started {
		return fmt.Errorf("cannot change title of a started instance")
	}
	i.Title = title
	return nil
}

// Paused reports whether the instance is in the Paused state.
func (i *Instance) Paused() bool {
	return i.Status == Paused
}

// TmuxAlive reports whether the underlying execution session is still running.
// The method name is preserved for backward compatibility with callers that
// use it to check session liveness (the semantics are unchanged).
func (i *Instance) TmuxAlive() bool {
	if i.executionSession == nil {
		return false
	}
	return i.executionSession.DoesSessionExist()
}
