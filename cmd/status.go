package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/spf13/cobra"
)

// statusTask is the JSON-serialisable representation of a task entry for the
// status command output.
type statusTask struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Branch string `json:"branch"`
}

// statusInstance is the JSON-serialisable representation of an instance record
// for the status command output.
type statusInstance struct {
	Title   string `json:"title"`
	Status  string `json:"status"`
	Program string `json:"program"`
	Task    string `json:"task,omitempty"`
	Type    string `json:"type,omitempty"`
}

// statusOrphan is the JSON-serialisable representation of an orphan tmux session
// for the status command output.
type statusOrphan struct {
	Name string `json:"name"`
	Age  string `json:"age"`
}

// statusData is the top-level JSON structure returned by executeStatus when
// format == "json".
type statusData struct {
	Tasks          []statusTask     `json:"tasks"`
	Instances      []statusInstance `json:"instances"`
	OrphanSessions []statusOrphan   `json:"orphan_sessions"`
}

// executeStatus assembles a unified overview of active tasks, agent instances,
// and orphan tmux sessions. It is the testable core of NewStatusCmd.
//
// Parameters:
//   - state: instance state manager (required)
//   - store: task store; may be nil (tasks section shows "no active tasks")
//   - project: project name used for store queries
//   - ex: executor for tmux discovery
//   - format: "text" or "json"
func executeStatus(state config.StateManager, store taskstore.Store, project string, ex Executor, format string) string {
	// 1. Tasks section — filter to non-done, non-cancelled entries.
	tasks := make([]statusTask, 0)
	if store != nil {
		entries, err := store.List(project)
		if err == nil {
			for _, e := range entries {
				if e.Status == taskstore.StatusCancelled || e.Status == taskstore.StatusDone {
					continue
				}
				tasks = append(tasks, statusTask{
					Name:   e.Filename,
					Status: string(e.Status),
					Branch: e.Branch,
				})
			}
		}
	}

	// 2. Instances section.
	// Load records once; reuse them for orphan detection below to avoid a
	// second deserialisation inside buildKnownNames.
	instances := make([]statusInstance, 0)
	records, recordsErr := loadInstanceRecords(state)
	if recordsErr == nil {
		for _, r := range records {
			agentType := r.AgentType
			if agentType == "" && (r.SoloAgent || r.TaskFile == "") {
				agentType = "solo"
			}
			instances = append(instances, statusInstance{
				Title:   r.Title,
				Status:  statusLabel(r.Status),
				Program: r.Program,
				Task:    r.TaskFile,
				Type:    agentType,
			})
		}
	}

	// 3. Orphan sessions section.
	// Build known names from the already-loaded records instead of calling
	// buildKnownNames (which would deserialise the state a second time).
	orphans := make([]statusOrphan, 0)
	if recordsErr == nil {
		known := make(map[string]struct{}, len(records))
		for _, r := range records {
			known[kasTmuxName(r.Title)] = struct{}{}
		}
		rows, discErr := discoverKasSessions(ex, known)
		if discErr == nil {
			now := time.Now()
			for _, row := range rows {
				if !row.Managed {
					orphans = append(orphans, statusOrphan{
						Name: row.Name,
						Age:  relativeAge(now, row.Created),
					})
				}
			}
		}
	}

	data := statusData{
		Tasks:          tasks,
		Instances:      instances,
		OrphanSessions: orphans,
	}

	// 4. JSON format.
	if format == "json" {
		b, err := json.Marshal(data)
		if err != nil {
			return fmt.Sprintf(`{"error": %q}`, err.Error())
		}
		return string(b)
	}

	// 5. Text format.
	var sb strings.Builder

	// Tasks section.
	sb.WriteString("tasks:\n")
	if len(tasks) == 0 {
		sb.WriteString("  no active tasks\n")
	} else {
		w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  STATUS\tNAME\tBRANCH")
		for _, t := range tasks {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", t.Status, t.Name, t.Branch)
		}
		w.Flush()
	}

	sb.WriteString("\n")

	// Instances section.
	sb.WriteString("instances:\n")
	if len(instances) == 0 {
		sb.WriteString("  no instances\n")
	} else {
		w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  TITLE\tSTATUS\tPROGRAM\tTASK\tTYPE")
		for _, i := range instances {
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", i.Title, i.Status, i.Program, i.Task, i.Type)
		}
		w.Flush()
	}

	sb.WriteString("\n")

	// Orphan sessions section.
	sb.WriteString("orphan tmux sessions:\n")
	if len(orphans) == 0 {
		sb.WriteString("  no orphan tmux sessions\n")
	} else {
		w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NAME\tAGE")
		for _, o := range orphans {
			fmt.Fprintf(w, "  %s\t%s\n", o.Name, o.Age)
		}
		w.Flush()
	}

	// Hints section — only shown when at least one condition applies.
	var hints []string
	for _, t := range tasks {
		if t.Status == string(taskstore.StatusReady) {
			hints = append(hints, "  kas task implement <task-name>    # start implementing a ready task")
			break
		}
	}
	for _, i := range instances {
		if i.Status == "paused" {
			hints = append(hints, "  kas instance resume <title>       # resume a paused instance")
			break
		}
	}
	if len(orphans) > 0 {
		hints = append(hints, "  kas tmux adopt <session> <title>  # adopt an orphan tmux session")
		hints = append(hints, "  kas tmux kill <session>            # kill an orphan tmux session")
	}
	if len(hints) > 0 {
		sb.WriteString("\nhints:\n")
		for _, h := range hints {
			sb.WriteString(h + "\n")
		}
	}

	return sb.String()
}

// NewStatusCmd builds the `kas status` cobra command.
func NewStatusCmd() *cobra.Command {
	var jsonFlag bool
	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"st"},
		Short:   "show overview of tasks, instances, and orphan tmux sessions",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			state := config.LoadState()
			store := resolveStore(project)
			format := "text"
			if jsonFlag {
				format = "json"
			}
			fmt.Print(executeStatus(state, store, project, MakeExecutor(), format))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")
	return cmd
}
