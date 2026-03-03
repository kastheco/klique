package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/spf13/cobra"
)

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
// It mirrors the relevant fields of session.GitWorktreeData.
type instanceWorktree struct {
	RepoPath     string `json:"repo_path"`
	WorktreePath string `json:"worktree_path"`
	BranchName   string `json:"branch_name"`
}

// instanceRecord is a local mirror of session.InstanceData containing only
// the fields needed for the list and lifecycle commands. Using a local type avoids
// the import cycle that arises because session/tmux imports cmd for the Executor
// interface.
type instanceRecord struct {
	Title     string           `json:"title"`
	Status    instanceStatus   `json:"status"`
	Branch    string           `json:"branch"`
	Program   string           `json:"program"`
	TaskFile  string           `json:"task_file,omitempty"`
	AgentType string           `json:"agent_type,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	Path      string           `json:"path,omitempty"`
	Worktree  instanceWorktree `json:"worktree"`
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

// executeInstanceList reads raw InstancesData from state, optionally filters by
// status, and formats the result as a text table or JSON array.
//
// statusFilters is optional; when provided only instances whose status label
// matches any of the given values are included.
func executeInstanceList(state config.StateManager, format string, statusFilters ...string) string {
	raw := state.GetInstances()

	var records []instanceRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return fmt.Sprintf("error: %v\n", err)
	}

	// Apply optional status filter.
	if len(statusFilters) > 0 {
		filterSet := make(map[string]struct{}, len(statusFilters))
		for _, f := range statusFilters {
			filterSet[strings.ToLower(f)] = struct{}{}
		}
		filtered := records[:0]
		for _, r := range records {
			if _, ok := filterSet[statusLabel(r.Status)]; ok {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}

	if len(records) == 0 {
		if format == "json" {
			return "[]\n"
		}
		return "no instances\n"
	}

	switch format {
	case "json":
		type jsonRecord struct {
			Title     string `json:"title"`
			Status    string `json:"status"`
			Branch    string `json:"branch"`
			Program   string `json:"program"`
			TaskFile  string `json:"task_file,omitempty"`
			AgentType string `json:"agent_type,omitempty"`
			CreatedAt string `json:"created_at,omitempty"`
		}
		out := make([]jsonRecord, 0, len(records))
		for _, r := range records {
			var createdAt string
			if !r.CreatedAt.IsZero() {
				createdAt = r.CreatedAt.Format(time.RFC3339)
			}
			out = append(out, jsonRecord{
				Title:     r.Title,
				Status:    statusLabel(r.Status),
				Branch:    r.Branch,
				Program:   r.Program,
				TaskFile:  r.TaskFile,
				AgentType: r.AgentType,
				CreatedAt: createdAt,
			})
		}
		data, err := json.Marshal(out)
		if err != nil {
			return fmt.Sprintf("error: %v\n", err)
		}
		return string(data) + "\n"

	default: // "text"
		var sb strings.Builder
		w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TITLE\tSTATUS\tBRANCH\tPROGRAM\tTASK")
		for _, r := range records {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				r.Title,
				statusLabel(r.Status),
				r.Branch,
				r.Program,
				r.TaskFile,
			)
		}
		w.Flush()
		return sb.String()
	}
}

// loadInstanceRecords reads and parses the raw instance JSON from state.
func loadInstanceRecords(state config.StateManager) ([]instanceRecord, error) {
	raw := state.GetInstances()
	var records []instanceRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("parse instances: %w", err)
	}
	return records, nil
}

// findInstanceData finds an instance record by title. It first tries an exact
// match, then falls back to a substring match. Returns an error when no match
// is found or when the substring matches more than one record (ambiguous).
func findInstanceData(records []instanceRecord, title string) (instanceRecord, error) {
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

// validateStatusForAction checks whether the instance is in a state compatible
// with the requested action and returns an error when it is not.
//
//   - kill:   allowed in any status
//   - pause:  not allowed when already paused
//   - resume: only allowed when paused
//   - send:   not allowed when paused
func validateStatusForAction(data instanceRecord, action string) error {
	switch action {
	case "kill":
		// kill is allowed in any status
		return nil
	case "pause":
		if data.Status == instancePaused {
			return fmt.Errorf("instance is already paused")
		}
		return nil
	case "resume":
		if data.Status != instancePaused {
			return fmt.Errorf("can only resume paused instances (current status: %s)", statusLabel(data.Status))
		}
		return nil
	case "send":
		if data.Status == instancePaused {
			return fmt.Errorf("cannot send prompt to a paused instance")
		}
		return nil
	default:
		return fmt.Errorf("unknown action: %q", action)
	}
}

// removeInstanceFromState removes the named instance from persisted state.
func removeInstanceFromState(state config.StateManager, title string) error {
	records, err := loadInstanceRecords(state)
	if err != nil {
		return err
	}
	remaining := records[:0]
	for _, r := range records {
		if r.Title != title {
			remaining = append(remaining, r)
		}
	}
	raw, err := json.Marshal(remaining)
	if err != nil {
		return fmt.Errorf("marshal instances: %w", err)
	}
	return state.SaveInstances(raw)
}

// updateInstanceInState finds the named instance, applies updater to a copy,
// and persists the modified list back to state.
func updateInstanceInState(state config.StateManager, title string, updater func(*instanceRecord) error) error {
	records, err := loadInstanceRecords(state)
	if err != nil {
		return err
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
	raw, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshal instances: %w", err)
	}
	return state.SaveInstances(raw)
}

// NewInstanceCmd builds the `kas instance` cobra command tree.
func NewInstanceCmd() *cobra.Command {
	instanceCmd := &cobra.Command{
		Use:   "instance",
		Short: "manage agent instances (list, kill, pause, resume, send)",
	}

	// kas instance list
	var format string
	var statusFilter string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "list all agent instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			state := config.LoadState()
			var filters []string
			if statusFilter != "" {
				filters = append(filters, statusFilter)
			}
			fmt.Print(executeInstanceList(state, format, filters...))
			return nil
		},
	}
	listCmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	listCmd.Flags().StringVar(&statusFilter, "status", "", "filter by status (running, ready, loading, paused)")
	instanceCmd.AddCommand(listCmd)

	// kas instance kill <title>
	killCmd := &cobra.Command{
		Use:   "kill <title>",
		Short: "kill an agent instance and remove it from state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			state := config.LoadState()
			records, err := loadInstanceRecords(state)
			if err != nil {
				return err
			}
			rec, err := findInstanceData(records, title)
			if err != nil {
				return err
			}
			if err := validateStatusForAction(rec, "kill"); err != nil {
				return err
			}
			// Kill tmux session (best-effort — may already be dead).
			_ = exec.Command("tmux", "kill-session", "-t", rec.Title).Run()
			// Remove git worktree if present.
			if rec.Worktree.WorktreePath != "" && rec.Worktree.RepoPath != "" {
				_ = exec.Command("git", "-C", rec.Worktree.RepoPath, "worktree", "remove", "--force", rec.Worktree.WorktreePath).Run()
				_ = exec.Command("git", "-C", rec.Worktree.RepoPath, "worktree", "prune").Run()
			}
			if err := removeInstanceFromState(state, rec.Title); err != nil {
				return err
			}
			fmt.Printf("killed: %s\n", rec.Title)
			return nil
		},
	}
	instanceCmd.AddCommand(killCmd)

	// kas instance pause <title>
	pauseCmd := &cobra.Command{
		Use:   "pause <title>",
		Short: "pause an agent instance (saves branch, removes worktree)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			state := config.LoadState()
			records, err := loadInstanceRecords(state)
			if err != nil {
				return err
			}
			rec, err := findInstanceData(records, title)
			if err != nil {
				return err
			}
			if err := validateStatusForAction(rec, "pause"); err != nil {
				return err
			}
			// Commit any dirty changes in the worktree before pausing.
			if rec.Worktree.WorktreePath != "" {
				commitMsg := fmt.Sprintf("[kas] update from '%s' on %s (paused)", rec.Title, time.Now().Format(time.RFC822))
				_ = exec.Command("git", "-C", rec.Worktree.WorktreePath, "add", "-A").Run()
				_ = exec.Command("git", "-C", rec.Worktree.WorktreePath, "commit", "-m", commitMsg, "--allow-empty").Run()
			}
			// Kill the tmux session.
			_ = exec.Command("tmux", "kill-session", "-t", rec.Title).Run()
			// Remove the worktree (preserve the branch).
			if rec.Worktree.WorktreePath != "" && rec.Worktree.RepoPath != "" {
				if err := exec.Command("git", "-C", rec.Worktree.RepoPath, "worktree", "remove", "--force", rec.Worktree.WorktreePath).Run(); err != nil {
					return fmt.Errorf("remove worktree: %w", err)
				}
				_ = exec.Command("git", "-C", rec.Worktree.RepoPath, "worktree", "prune").Run()
			}
			// Update state: mark as paused.
			if err := updateInstanceInState(state, rec.Title, func(r *instanceRecord) error {
				r.Status = instancePaused
				r.Worktree.WorktreePath = ""
				return nil
			}); err != nil {
				return err
			}
			fmt.Printf("paused: %s\n", rec.Title)
			return nil
		},
	}
	instanceCmd.AddCommand(pauseCmd)

	// kas instance resume <title>
	resumeCmd := &cobra.Command{
		Use:   "resume <title>",
		Short: "resume a paused agent instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			state := config.LoadState()
			records, err := loadInstanceRecords(state)
			if err != nil {
				return err
			}
			rec, err := findInstanceData(records, title)
			if err != nil {
				return err
			}
			if err := validateStatusForAction(rec, "resume"); err != nil {
				return err
			}
			if rec.Worktree.RepoPath == "" || rec.Worktree.BranchName == "" {
				return fmt.Errorf("instance %q has no stored worktree metadata; cannot resume", rec.Title)
			}
			// Derive the worktree path from the repo path and session name.
			worktreePath := rec.Worktree.WorktreePath
			if worktreePath == "" {
				// Reconstruct using the conventional path: <worktree-root>/<title>
				worktreePath = rec.Path
			}
			// Re-add the git worktree on the preserved branch.
			if err := exec.Command("git", "-C", rec.Worktree.RepoPath, "worktree", "add", worktreePath, rec.Worktree.BranchName).Run(); err != nil {
				return fmt.Errorf("recreate worktree: %w", err)
			}
			// Start a new tmux session in the worktree directory.
			if err := exec.Command("tmux", "new-session", "-d", "-s", rec.Title, "-c", worktreePath, rec.Program).Run(); err != nil {
				return fmt.Errorf("start tmux session: %w", err)
			}
			// Update state: mark as running.
			if err := updateInstanceInState(state, rec.Title, func(r *instanceRecord) error {
				r.Status = instanceRunning
				r.Worktree.WorktreePath = worktreePath
				return nil
			}); err != nil {
				return err
			}
			fmt.Printf("resumed: %s\n", rec.Title)
			return nil
		},
	}
	instanceCmd.AddCommand(resumeCmd)

	// kas instance send <title> <prompt>
	sendCmd := &cobra.Command{
		Use:   "send <title> <prompt>",
		Short: "send a prompt to an agent instance",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			prompt := args[1]
			state := config.LoadState()
			records, err := loadInstanceRecords(state)
			if err != nil {
				return err
			}
			rec, err := findInstanceData(records, title)
			if err != nil {
				return err
			}
			if err := validateStatusForAction(rec, "send"); err != nil {
				return err
			}
			if err := exec.Command("tmux", "send-keys", "-t", rec.Title, prompt, "Enter").Run(); err != nil {
				return fmt.Errorf("send keys: %w", err)
			}
			fmt.Printf("sent to %s\n", rec.Title)
			return nil
		},
	}
	instanceCmd.AddCommand(sendCmd)

	return instanceCmd
}
