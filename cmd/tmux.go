package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/spf13/cobra"
)

// tmuxSessionRow holds data about a single kas_-prefixed tmux session discovered
// by discoverKasSessions.  It mirrors session/tmux.SessionInfo without importing
// that package (which would create an import cycle: session/tmux → cmd → session/tmux).
type tmuxSessionRow struct {
	Name     string
	Title    string
	Created  time.Time
	Windows  int
	Attached bool
	Width    int
	Height   int
	Managed  bool
}

// relativeAge returns a human-readable age string relative to now, using the
// same bucket thresholds as ui/overlay/tmuxBrowserOverlay.go:relativeTime.
// A deterministic now parameter is provided so tests can assert exact output.
func relativeAge(now, created time.Time) string {
	d := now.Sub(created)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// discoverKasSessions queries `tmux ls -F ...` through the injected Executor,
// parses the output, and returns only kas_-prefixed sessions.
//
// Behavior mirrors session/tmux.DiscoverAll (tmux_session.go:591):
//   - *exec.ExitError (no server, no sessions) → empty list, no error.
//   - Non-ExitError → propagated as error.
//   - Malformed lines (fewer than 6 pipe-separated fields) → silently skipped.
//   - Non-kas_ sessions → silently ignored.
//
// The known map is keyed by raw tmux session names (e.g. "kas_foo").
// Sessions whose name appears in known are marked Managed = true.
func discoverKasSessions(ex Executor, known map[string]struct{}) ([]tmuxSessionRow, error) {
	lsCmd := exec.Command("tmux", "ls", "-F",
		"#{session_name}|#{session_created}|#{session_windows}|#{session_attached}|#{window_width}|#{window_height}")
	output, err := ex.Output(lsCmd)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil, nil
		}
		return nil, fmt.Errorf("list tmux sessions: %w", err)
	}

	var rows []tmuxSessionRow
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 6 {
			continue
		}
		name := parts[0]
		if !strings.HasPrefix(name, "kas_") {
			continue
		}

		var created time.Time
		if epoch, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			created = time.Unix(epoch, 0)
		}
		windows, _ := strconv.Atoi(parts[2])
		attached := parts[3] != "0"
		width, _ := strconv.Atoi(parts[4])
		height, _ := strconv.Atoi(parts[5])

		_, managed := known[name]
		rows = append(rows, tmuxSessionRow{
			Name:     name,
			Title:    strings.TrimPrefix(name, "kas_"),
			Created:  created,
			Windows:  windows,
			Attached: attached,
			Width:    width,
			Height:   height,
			Managed:  managed,
		})
	}
	return rows, nil
}

// buildKnownNames reads persisted instance records and returns a set of
// kas_-prefixed tmux session names for all known instances.
func buildKnownNames(state config.StateManager) (map[string]struct{}, error) {
	records, err := loadInstanceRecords(state)
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(records))
	for _, r := range records {
		known[kasTmuxName(r.Title)] = struct{}{}
	}
	return known, nil
}

// executeTmuxList discovers orphan tmux sessions (Managed == false) and formats
// them as a tab-aligned table.  Returns "no orphan tmux sessions found\n" when
// the orphan list is empty.
func executeTmuxList(state config.StateManager, ex Executor) (string, error) {
	known, err := buildKnownNames(state)
	if err != nil {
		return "", err
	}
	rows, err := discoverKasSessions(ex, known)
	if err != nil {
		return "", err
	}

	var orphans []tmuxSessionRow
	for _, r := range rows {
		if !r.Managed {
			orphans = append(orphans, r)
		}
	}
	if len(orphans) == 0 {
		return "no orphan tmux sessions found\n", nil
	}

	now := time.Now()
	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTITLE\tWINDOWS\tATTACHED\tAGE")
	for _, r := range orphans {
		fmt.Fprintf(w, "%s\t%s\t%d\t%v\t%s\n",
			r.Name,
			r.Title,
			r.Windows,
			r.Attached,
			relativeAge(now, r.Created),
		)
	}
	w.Flush()
	return sb.String(), nil
}

// executeTmuxAdopt validates and persists a new instanceRecord for the given
// orphan tmux session.
//
// Validation order (deterministic):
//  1. title must be non-empty (after TrimSpace)
//  2. title must not collide with an existing instance title
//  3. sessionName must match an orphan session (Managed == false)
//
// On success a new instanceRecord is appended to state with Status = instanceReady,
// Program = "unknown", Path = repoRoot, CreatedAt/UpdatedAt = now.
func executeTmuxAdopt(state config.StateManager, sessionName, title, repoRoot string, now time.Time, ex Executor) error {
	// 1. Validate title is non-empty.
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title must not be empty")
	}

	// 2. Load existing records and validate title uniqueness.
	records, err := loadInstanceRecords(state)
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.Title == title {
			return fmt.Errorf("instance title already exists: %s", title)
		}
	}

	// 3. Discover orphan set and validate session existence.
	known, err := buildKnownNames(state)
	if err != nil {
		return err
	}
	rows, err := discoverKasSessions(ex, known)
	if err != nil {
		return err
	}
	found := false
	for _, row := range rows {
		if row.Name == sessionName && !row.Managed {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("orphan tmux session not found: %s", sessionName)
	}

	// 4. Persist new record without mutating existing records.
	rec := instanceRecord{
		Title:     title,
		Path:      repoRoot,
		Status:    instanceReady,
		Program:   "unknown",
		CreatedAt: now,
		UpdatedAt: now,
	}
	records = append(records, rec)
	raw, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshal instances: %w", err)
	}
	return state.SaveInstances(raw)
}

// executeTmuxKill validates that the target session is an orphan, then kills it.
// Failure is wrapped as: "kill tmux session <name>: <cause>".
func executeTmuxKill(state config.StateManager, sessionName string, ex Executor) error {
	known, err := buildKnownNames(state)
	if err != nil {
		return err
	}
	rows, err := discoverKasSessions(ex, known)
	if err != nil {
		return err
	}

	found := false
	for _, row := range rows {
		if row.Name == sessionName && !row.Managed {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("orphan tmux session not found: %s", sessionName)
	}

	killCmd := exec.Command("tmux", "kill-session", "-t", sessionName)
	if err := ex.Run(killCmd); err != nil {
		return fmt.Errorf("kill tmux session %s: %w", sessionName, err)
	}
	return nil
}

// NewTmuxCmd builds the `kas tmux` cobra command tree.
func NewTmuxCmd() *cobra.Command {
	tmuxCmd := &cobra.Command{
		Use:   "tmux",
		Short: "manage orphan tmux sessions",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "list orphan tmux sessions not tracked by kasmos",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			state := config.LoadState()
			out, err := executeTmuxList(state, MakeExecutor())
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}

	adoptCmd := &cobra.Command{
		Use:   "adopt <session> <title>",
		Short: "adopt an orphan tmux session as a managed kasmos instance",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionName := args[0]
			title := args[1]
			state := config.LoadState()
			if err := executeTmuxAdopt(state, sessionName, title, ".", time.Now(), MakeExecutor()); err != nil {
				return err
			}
			fmt.Printf("adopted: %s as %q\n", sessionName, title)
			return nil
		},
	}

	killCmd := &cobra.Command{
		Use:   "kill <session>",
		Short: "kill an orphan tmux session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionName := args[0]
			state := config.LoadState()
			if err := executeTmuxKill(state, sessionName, MakeExecutor()); err != nil {
				return err
			}
			fmt.Printf("killed: %s\n", sessionName)
			return nil
		},
	}

	tmuxCmd.AddCommand(listCmd, adoptCmd, killCmd)
	return tmuxCmd
}
