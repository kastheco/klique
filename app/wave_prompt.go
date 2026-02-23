package app

import (
	"fmt"
	"strings"

	"github.com/kastheco/klique/config/planparser"
)

// buildTaskPrompt constructs the prompt for a single task instance.
func buildTaskPrompt(plan *planparser.Plan, task planparser.Task, waveNumber, totalWaves int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Implement Task %d: %s\n\n", task.Number, task.Title))
	sb.WriteString("Load the `cli-tools` skill before starting.\n\n")

	// Plan context
	header := plan.HeaderContext()
	if header != "" {
		sb.WriteString("## Plan Context\n\n")
		sb.WriteString(header)
		sb.WriteString("\n")
	}

	// Wave context
	sb.WriteString(fmt.Sprintf("## Wave %d of %d\n\n", waveNumber, totalWaves))
	if totalWaves > 1 {
		sb.WriteString("You are implementing one task of a multi-task plan. Other tasks in this wave may be running in parallel on the same worktree. Only modify the files listed in your task.\n\n")
	}

	// Task body
	sb.WriteString("## Task Instructions\n\n")
	sb.WriteString(task.Body)
	sb.WriteString("\n")

	return sb.String()
}
