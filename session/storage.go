package session

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session/tmux"
)

// InstanceData is the JSON-serializable mirror of Instance fields used for persistence.
type InstanceData struct {
	Title           string    `json:"title"`
	Path            string    `json:"path"`
	Branch          string    `json:"branch"`
	Status          Status    `json:"status"`
	Height          int       `json:"height"`
	Width           int       `json:"width"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	AutoYes         bool      `json:"auto_yes"`
	SkipPermissions bool      `json:"skip_permissions"`
	Program         string    `json:"program"`

	// Optional plan/orchestration fields.
	TaskFile               string `json:"task_file,omitempty"`
	AgentType              string `json:"agent_type,omitempty"`
	TaskNumber             int    `json:"task_number,omitempty"`
	WaveNumber             int    `json:"wave_number,omitempty"`
	PeerCount              int    `json:"peer_count,omitempty"`
	IsReviewer             bool   `json:"is_reviewer,omitempty"`
	ImplementationComplete bool   `json:"implementation_complete,omitempty"`
	SoloAgent              bool   `json:"solo_agent,omitempty"`
	QueuedPrompt           string `json:"queued_prompt,omitempty"`
	ReviewCycle            int    `json:"review_cycle,omitempty"`

	Worktree  GitWorktreeData `json:"worktree"`
	DiffStats DiffStatsData   `json:"diff_stats"`
}

// UnmarshalJSON implements a custom unmarshaler that handles the historical rename from
// the "plan_file" JSON key to "task_file". State files written before the rename used
// "plan_file"; if that field is present and "task_file" is empty, the value is migrated.
func (d *InstanceData) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion when calling json.Unmarshal.
	type Alias InstanceData
	aux := &struct {
		*Alias
		PlanFile string `json:"plan_file,omitempty"`
	}{
		Alias: (*Alias)(d),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	// Migrate legacy field.
	if d.TaskFile == "" && aux.PlanFile != "" {
		d.TaskFile = aux.PlanFile
	}
	return nil
}

// GitWorktreeData is the serializable form of a git.GitWorktree.
type GitWorktreeData struct {
	RepoPath      string `json:"repo_path"`
	WorktreePath  string `json:"worktree_path"`
	SessionName   string `json:"session_name"`
	BranchName    string `json:"branch_name"`
	BaseCommitSHA string `json:"base_commit_sha"`
}

// DiffStatsData is the serializable form of a git.DiffStats.
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"content"`
}

// Storage persists instances via a config.StateManager.
type Storage struct {
	state config.StateManager
}

// NewStorage constructs a Storage backed by the given state manager.
func NewStorage(state config.StateManager) (*Storage, error) {
	return &Storage{state: state}, nil
}

// SaveInstances serialises all started instances and writes them to the state store.
func (s *Storage) SaveInstances(instances []*Instance) error {
	records := make([]InstanceData, 0, len(instances))
	for _, inst := range instances {
		if inst.Started() {
			records = append(records, inst.ToInstanceData())
		}
	}

	raw, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	return s.state.SaveInstances(raw)
}

// LoadInstances reads instances from the state store and restores them.
// Stale entries are dropped with a warning instead of causing a hard failure:
//   - non-paused instances whose worktree directory no longer exists
//   - wave-task instances whose tmux session no longer exists
func (s *Storage) LoadInstances() ([]*Instance, error) {
	raw := s.state.GetInstances()

	var records []InstanceData
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	instances := make([]*Instance, 0, len(records))
	for _, rec := range records {
		// Drop non-paused instances whose worktree path has disappeared from disk.
		if rec.Status != Paused && rec.Worktree.WorktreePath != "" {
			if _, err := os.Stat(rec.Worktree.WorktreePath); err != nil {
				log.WarningLog.Printf(
					"skipping stale instance %q: worktree path gone: %s",
					rec.Title, rec.Worktree.WorktreePath,
				)
				continue
			}
		}

		// Wave-task instances are not resumable without their tmux session.
		// Drop stale records so restart recovery does not resurrect ghost tasks.
		if rec.TaskNumber > 0 {
			ts := tmux.NewTmuxSession(rec.Title, rec.Program, rec.SkipPermissions)
			if !ts.DoesSessionExist() {
				log.WarningLog.Printf(
					"skipping stale wave instance %q: tmux session not found",
					rec.Title,
				)
				continue
			}
		}

		inst, err := FromInstanceData(rec)
		if err != nil {
			// Log and skip rather than failing the entire load.
			log.WarningLog.Printf("skipping unrestorable instance %q: %v", rec.Title, err)
			continue
		}
		instances = append(instances, inst)
	}

	return instances, nil
}

// DeleteInstance removes the instance with the given title from persistent storage.
// Returns an error when no matching instance is found.
func (s *Storage) DeleteInstance(title string) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	remaining := instances[:0]
	found := false
	for _, inst := range instances {
		if inst.ToInstanceData().Title == title {
			found = true
		} else {
			remaining = append(remaining, inst)
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	return s.SaveInstances(remaining)
}

// UpdateInstance replaces the stored record for the instance (matched by title).
// Returns an error when no matching instance is found.
func (s *Storage) UpdateInstance(instance *Instance) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	target := instance.ToInstanceData().Title
	found := false
	for i, inst := range instances {
		if inst.ToInstanceData().Title == target {
			instances[i] = instance
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", target)
	}

	return s.SaveInstances(instances)
}

// DeleteAllInstances removes every stored instance from the state store.
func (s *Storage) DeleteAllInstances() error {
	return s.state.DeleteAllInstances()
}
