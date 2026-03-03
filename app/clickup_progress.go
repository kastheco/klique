package app

import (
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/log"

	tea "github.com/charmbracelet/bubbletea"
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

// shouldPostWaveCompleteComment returns true when an intermediate wave_complete
// comment should be posted to ClickUp. Single-wave plans always return false —
// they use the "all waves complete" event instead so they don't get a redundant
// notification for every wave.
func shouldPostWaveCompleteComment(orch *WaveOrchestrator) bool {
	return orch != nil && orch.TotalWaves() > 1
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
