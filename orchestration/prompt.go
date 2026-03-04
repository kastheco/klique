package orchestration

import (
	"fmt"
	"strings"

	"github.com/kastheco/kasmos/config/taskparser"
)

// BuildTaskPrompt constructs the prompt for a single task instance.
func BuildTaskPrompt(plan *taskparser.Plan, task taskparser.Task, waveNumber, totalWaves, peerCount int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Implement Task %d: %s\n\n", task.Number, task.Title))
	sb.WriteString("Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.\n\n")

	// Plan context
	header := plan.HeaderContext()
	if header != "" {
		sb.WriteString("## Plan Context\n\n")
		sb.WriteString(header)
		sb.WriteString("\n")
	}

	// Wave context
	sb.WriteString(fmt.Sprintf("## Wave %d of %d\n\n", waveNumber, totalWaves))

	// Parallel awareness — only for multi-task waves
	if peerCount > 1 {
		sb.WriteString(fmt.Sprintf("## Parallel Execution\n\n"))
		sb.WriteString(fmt.Sprintf("You are Task %d of %d in Wave %d. %d other agents are working in parallel on this same worktree.\n\n",
			task.Number, peerCount, waveNumber, peerCount-1))

		sb.WriteString("Your assigned files are listed in the Task Instructions below. Prioritize those files. ")
		sb.WriteString("If you must touch a shared file (go.mod, go.sum, imports), make minimal surgical changes - ")
		sb.WriteString("do not reorganize, reformat, or refactor anything outside your task scope.\n\n")

		sb.WriteString("CRITICAL - shared worktree rules:\n")
		sb.WriteString("- NEVER run `git add .` or `git add -A` - you will commit other agents' in-progress work\n")
		sb.WriteString("- NEVER run `git stash` or `git reset` - you will destroy sibling agents' changes\n")
		sb.WriteString("- NEVER run `git checkout -- <file>` on files you didn't modify - you will revert a sibling's edits\n")
		sb.WriteString("- NEVER run formatters/linters across the whole project (e.g. `go fmt ./...`) - scope them to your files only\n")
		sb.WriteString("- NEVER try to fix test failures in files outside your task - they may be caused by incomplete parallel work\n")
		sb.WriteString("- DO `git add` only the specific files you changed\n")
		sb.WriteString("- DO commit frequently with your task number in the message\n")
		sb.WriteString("- DO expect untracked files and uncommitted changes that are not yours - ignore them\n\n")
	}

	// Task body
	sb.WriteString("## Task Instructions\n\n")
	sb.WriteString(task.Body)
	sb.WriteString("\n")

	return sb.String()
}

// BuildWaveAnnotationPrompt returns the prompt used when a planner is respawned
// to add ## Wave headers to an existing plan that is missing them.
// It instructs the planner to annotate the plan, commit the change, and write
// the sentinel signal so kasmos can resume the implementation flow.
func BuildWaveAnnotationPrompt(planFile string) string {
	return fmt.Sprintf(
		"The plan %[1]s is missing ## Wave N headers required for kasmos wave orchestration. "+
			"Retrieve the plan content with `kas task show %[1]s`, then annotate it by wrapping "+
			"all tasks under ## Wave N sections. "+
			"Every plan needs at least ## Wave 1 — even single-task trivial plans. "+
			"Keep all existing task content intact; only add the ## Wave headers.\n\n"+
			"After annotating:\n"+
			"1. Store the updated plan via `kas task update-content %[1]s` (pipe the content)\n"+
			"2. Signal completion: touch .kasmos/signals/planner-finished-%[1]s\n"+
			"Do not edit plan-state.json directly.",
		planFile,
	)
}
