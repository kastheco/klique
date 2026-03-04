package app

import (
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/log"

	tea "charm.land/bubbletea/v2"
)

// resolveClickUpTaskID returns the ClickUp task ID for a plan entry.
// It checks the ClickUpTaskID field first (DB-stored), then falls back to
// parsing the plan content for a "**Source:** ClickUp <ID>" line.
// Returns "" when no task ID can be found.
func resolveClickUpTaskID(entry taskstate.TaskEntry, content string) string {
	if entry.ClickUpTaskID != "" {
		return entry.ClickUpTaskID
	}
	return clickup.ParseClickUpTaskID(content)
}

// postClickUpProgress creates a fire-and-forget tea.Cmd that posts a markdown
// progress comment to the ClickUp task linked to the given taskID.
// Returns nil (no-op) when taskID is empty or commenter is nil.
// Any PostComment error is logged as a warning — ClickUp is best-effort.
func postClickUpProgress(commenter *clickup.Commenter, taskID, comment string) tea.Cmd {
	if taskID == "" || commenter == nil {
		return nil
	}
	return func() tea.Msg {
		if err := commenter.PostComment(taskID, comment); err != nil {
			log.WarningLog.Printf("postClickUpProgress: %v", err)
		}
		return nil
	}
}
