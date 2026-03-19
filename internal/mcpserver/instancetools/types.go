// Package instancetools provides MCP tools for managing kasmos agent instances
// and querying the kasmos daemon. It exposes instance_list, instance_send,
// instance_pause, instance_resume, and daemon_status tools that agents can use
// to manage instances and inspect daemon state via typed MCP calls.
package instancetools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kastheco/kasmos/config"
)

// CmdRunner abstracts external command execution for testability.
// It exposes both Run (fire-and-forget) and Output (capture stdout).
type CmdRunner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner is the real CmdRunner that delegates to os/exec.
type ExecRunner struct{}

// Run executes name with args under the given context, discarding output.
func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// Output runs name with args under the given context and returns its standard output.
func (r *ExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// StateLoader is a function that returns a fresh StateManager snapshot.
// Each call should return a consistent view of the current state.
type StateLoader func() config.StateManager

// instanceStatus mirrors session.Status (int iota) without importing session.
// Values must stay in sync with session package constants:
//
//	Running = 0, Ready = 1, Loading = 2, Paused = 3
type instanceStatus int

const (
	instanceRunning instanceStatus = 0
	instanceReady   instanceStatus = 1
	instanceLoading instanceStatus = 2
	instancePaused  instanceStatus = 3
)

// instanceWorktree holds the git worktree metadata needed for lifecycle commands.
// It fully mirrors session.GitWorktreeData so round-trip serialisation is lossless.
type instanceWorktree struct {
	RepoPath      string `json:"repo_path"`
	WorktreePath  string `json:"worktree_path"`
	SessionName   string `json:"session_name"`
	BranchName    string `json:"branch_name"`
	BaseCommitSHA string `json:"base_commit_sha"`
}

// instanceRecord is a local mirror of session.InstanceData containing all fields
// required for lossless round-trip serialisation. Every field present in
// InstanceData must appear here; omitting a field causes silent data loss when
// the state file is rewritten by pause/resume.
//
// Using a local type avoids the import cycle that arises because session/tmux
// imports cmd for the Executor interface — so cmd cannot import session/tmux.
type instanceRecord struct {
	Title     string         `json:"title"`
	Path      string         `json:"path,omitempty"`
	Branch    string         `json:"branch"`
	Status    instanceStatus `json:"status"`
	Height    int            `json:"height"`
	Width     int            `json:"width"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Program   string         `json:"program"`
	AutoYes   bool           `json:"auto_yes"`

	// SkipPermissions, when true, passes --dangerously-skip-permissions to Claude.
	SkipPermissions bool `json:"skip_permissions"`

	// Optional plan/orchestration fields — must stay in sync with InstanceData.
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

	Worktree instanceWorktree `json:"worktree"`
}

// UnmarshalJSON implements a custom unmarshaler that handles the historical rename
// from the "plan_file" JSON key to "task_file", mirroring session.InstanceData.UnmarshalJSON.
func (r *instanceRecord) UnmarshalJSON(data []byte) error {
	type Alias instanceRecord
	aux := &struct {
		*Alias
		PlanFile string `json:"plan_file,omitempty"`
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if r.TaskFile == "" && aux.PlanFile != "" {
		r.TaskFile = aux.PlanFile
	}
	return nil
}

// statusLabel converts an instanceStatus to a lowercase text label.
func statusLabel(s instanceStatus) string {
	switch s {
	case instanceRunning:
		return "running"
	case instanceReady:
		return "ready"
	case instanceLoading:
		return "loading"
	case instancePaused:
		return "paused"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// whiteSpaceRe matches one or more whitespace characters for session name sanitisation.
var whiteSpaceRe = regexp.MustCompile(`\s+`)

// kasTmuxName converts a human-readable instance title to the kas_-prefixed tmux
// session name used by the session package. It replicates toKasTmuxName from
// session/tmux without importing that package (which would create a cycle).
func kasTmuxName(title string) string {
	name := whiteSpaceRe.ReplaceAllString(title, "")
	name = strings.ReplaceAll(name, ".", "_")
	return "kas_" + name
}

// loadRecords reads and parses the raw instance JSON from the state loader.
func loadRecords(loadState StateLoader) ([]instanceRecord, error) {
	state := loadState()
	raw := state.GetInstances()
	var records []instanceRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("parse instances: %w", err)
	}
	return records, nil
}

// findRecord finds an instance record by title. It first tries an exact
// match, then falls back to a substring match. Returns an error when no match
// is found or when the substring matches more than one record (ambiguous).
func findRecord(records []instanceRecord, title string) (instanceRecord, error) {
	// Exact match takes precedence.
	for _, r := range records {
		if r.Title == title {
			return r, nil
		}
	}

	// Substring fallback.
	var matches []instanceRecord
	for _, r := range records {
		if strings.Contains(r.Title, title) {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 0:
		return instanceRecord{}, fmt.Errorf("instance not found: %q", title)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Title
		}
		return instanceRecord{}, fmt.Errorf("ambiguous instance %q matches: %s", title, strings.Join(names, ", "))
	}
}

// updateRecord finds the named instance, applies updater to a copy, and
// persists the modified list back to state. All fields of every record are
// preserved verbatim.
func updateRecord(loadState StateLoader, title string, updater func(*instanceRecord) error) error {
	state := loadState()
	raw := state.GetInstances()
	var records []instanceRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return fmt.Errorf("parse instances: %w", err)
	}
	found := false
	for i := range records {
		if records[i].Title == title {
			if err := updater(&records[i]); err != nil {
				return err
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("instance not found: %q", title)
	}
	out, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshal instances: %w", err)
	}
	return state.SaveInstances(out)
}

// validateAction checks whether the instance is in a state compatible
// with the requested action and returns an error when it is not.
//
//   - kill:   allowed in any status
//   - pause:  not allowed when already paused
//   - resume: only allowed when paused
//   - send:   not allowed when paused
func validateAction(rec instanceRecord, action string) error {
	switch action {
	case "kill":
		// kill is allowed in any status
		return nil
	case "pause":
		if rec.Status == instancePaused {
			return fmt.Errorf("instance is already paused")
		}
		return nil
	case "resume":
		if rec.Status != instancePaused {
			return fmt.Errorf("can only resume paused instances (current status: %s)", statusLabel(rec.Status))
		}
		return nil
	case "send":
		if rec.Status == instancePaused {
			return fmt.Errorf("cannot send prompt to a paused instance")
		}
		return nil
	default:
		return fmt.Errorf("unknown action: %q", action)
	}
}

// daemonSocketPath returns the default Unix domain socket path for the daemon
// control API. Matches the defaultSocketPath() logic in the daemon package:
// prefers $XDG_RUNTIME_DIR/kasmos/kas.sock, then falls back to
// /tmp/kasmos-<uid>/kas.sock.
func daemonSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "kasmos", "kas.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("kasmos-%d", os.Getuid()), "kas.sock")
}
