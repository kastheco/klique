package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/spf13/cobra"
)

// NewAuditCmd builds the `kas audit` cobra command tree.
func NewAuditCmd() *cobra.Command {
	auditCmd := &cobra.Command{Use: "audit", Short: "query audit events"}
	var limit int
	var event string
	listCmd := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit <= 0 {
				return fmt.Errorf("limit must be > 0")
			}
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			logger, err := openAuditLogger()
			if err != nil {
				return err
			}
			defer logger.Close()
			out, err := executeAuditList(logger, project, limit, event)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
	listCmd.Flags().IntVar(&limit, "limit", 50, "max rows")
	listCmd.Flags().StringVar(&event, "event", "", "event kind filter")
	auditCmd.AddCommand(listCmd)
	return auditCmd
}

// openAuditLogger opens the shared SQLite database for audit log queries.
func openAuditLogger() (*auditlog.SQLiteLogger, error) {
	return auditlog.NewSQLiteLogger(taskstore.ResolvedDBPath())
}

// executeAuditList queries audit events from the logger and returns a
// formatted table string. It is deterministic and pure (no stdout writes).
func executeAuditList(logger auditlog.Logger, project string, limit int, event string) (string, error) {
	filter := auditlog.QueryFilter{Project: project, Limit: limit}
	if event != "" {
		filter.Kinds = []auditlog.EventKind{auditlog.EventKind(event)}
	}

	events, err := logger.Query(filter)
	if err != nil {
		return "", fmt.Errorf("query audit events: %w", err)
	}

	if len(events) == 0 {
		return "no audit entries found\n", nil
	}

	return renderAuditRows(events), nil
}

// renderAuditRows formats a slice of audit events as a tabwriter table string.
func renderAuditRows(events []auditlog.Event) string {
	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tEVENT\tDETAILS")
	for _, e := range events {
		ts := e.Timestamp.Local().Format("2006-01-02 15:04:05")
		details := formatAuditDetails(e.Message, e.Detail)
		fmt.Fprintf(w, "%s\t%s\t%s\n", ts, string(e.Kind), details)
	}
	w.Flush()
	return sb.String()
}

// formatAuditDetails combines Message and Detail into a single display string.
func formatAuditDetails(message, detail string) string {
	switch {
	case message != "" && detail != "":
		return message + " | " + detail
	case message != "":
		return message
	default:
		return detail
	}
}
