package cmd

import (
	"encoding/json"
	"fmt"
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

// instanceRecord is a local mirror of session.InstanceData containing only
// the fields needed for the list command. Using a local type avoids the import
// cycle that arises because session/tmux imports cmd for the Executor interface.
type instanceRecord struct {
	Title     string         `json:"title"`
	Status    instanceStatus `json:"status"`
	Branch    string         `json:"branch"`
	Program   string         `json:"program"`
	TaskFile  string         `json:"task_file,omitempty"`
	AgentType string         `json:"agent_type,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
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

	return instanceCmd
}
