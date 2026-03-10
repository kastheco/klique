package orchestration

import (
	"fmt"
	"strings"

	"github.com/kastheco/kasmos/config/taskparser"
)

// BuildTaskPrompt constructs the prompt for a single task instance.
func BuildTaskPrompt(planFile string, plan *taskparser.Plan, task taskparser.Task, waveNumber, totalWaves, peerCount int, meta *TaskMeta) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Implement Task %d: %s\n\n", task.Number, task.Title))

	// Inline coder rules — avoids the context cost of loading the kasmos-coder skill
	sb.WriteString("## Rules\n\n")
	sb.WriteString("- Implement ONLY this task. Do not modify files outside your scope.\n")
	sb.WriteString("- Do NOT load agent skills — rules are inlined here.\n")
	sb.WriteString("- Use `rg` (not grep), `sd` (not sed), `fd` (not find), `comby`/`ast-grep` for structural changes.\n")
	sb.WriteString("- Run scoped tests before committing: `go test ./pkg/... -run Test<Name> -v`\n")
	sb.WriteString("- Verify build: `go build ./...`\n")
	sb.WriteString("- Commit: `git add <specific-files> && git commit -m \"feat(task-N): description\"`\n")
	sb.WriteString(fmt.Sprintf("- When done: signal completion with `kas signal emit implement_task_finished %s --payload '{\"wave_number\":%d,\"task_number\":%d}'` (or fallback: `touch .kasmos/signals/implement-task-finished-w%d-t%d-%s`), then stop.\n\n",
		planFile, waveNumber, task.Number, waveNumber, task.Number, planFile))

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
		sb.WriteString("If you encounter a build failure caused by missing types, functions, or interfaces that your task ")
		sb.WriteString("imports from a package being modified by a sibling agent: this is an import dependency that should have ")
		sb.WriteString("been in a separate wave. Do NOT stub, mock, or work around it. Commit whatever work you have completed ")
		sb.WriteString("so far, report the dependency in your commit message (e.g. 'partial: blocked on task N types'), and stop.\n\n")
		sb.WriteString("- DO `git add` only the specific files you changed\n")
		sb.WriteString("- DO commit frequently with your task number in the message\n")
		sb.WriteString("- DO expect untracked files and uncommitted changes that are not yours - ignore them\n\n")
	}

	// Task body
	sb.WriteString("## Task Instructions\n\n")
	sb.WriteString(task.Body)
	sb.WriteString("\n")

	if meta != nil && len(meta.VerifyChecks) > 0 {
		sb.WriteString("\n## Verification Commands\n\n")
		for _, check := range meta.VerifyChecks {
			sb.WriteString("- `" + check + "`\n")
		}
	}

	return sb.String()
}

// BuildElaborationPrompt returns the prompt for an elaborator agent session.
// The elaborator reads the plan, deeply reads the codebase for each task's files,
// and expands task bodies with detailed implementation instructions.
func BuildElaborationPrompt(planFile string) string {
	return fmt.Sprintf(
		"You are the elaborator agent. Your job: enrich a plan's task descriptions with "+
			"detailed implementation instructions so coder agents make fewer decisions.\n\n"+
			"Load the `kasmos-elaborator` skill before starting. Also load `cli-tools`.\n\n"+
			"## Instructions\n\n"+
			"1. Retrieve the plan: `kas task show %[1]s`\n"+
			"2. For each task, read the codebase files listed in its **Files:** section. "+
			"Study existing patterns, interfaces, function signatures, error handling, "+
			"and data flow in those files and their neighbors.\n"+
			"3. Expand each task body with concrete implementation detail:\n"+
			"   - Exact function signatures to create or modify\n"+
			"   - Existing codebase patterns to follow (with file references)\n"+
			"   - Edge cases and error handling requirements\n"+
			"   - Import paths and dependencies\n"+
			"   - Concrete code snippets where helpful\n"+
			"4. Preserve the plan structure — do not change wave organization, "+
			"task numbering, file lists, or the header fields. Only expand task bodies.\n"+
			"5. Write the updated plan: pipe content to `kas task update-content %[1]s`\n"+
			"6. Signal completion: `kas signal emit elaborator_finished %[1]s` (or fallback: `touch .kasmos/signals/elaborator-finished-%[1]s`)\n",
		planFile,
	)
}

// BuildArchitectPrompt returns the prompt for an architect agent session.
// The architect identifies task relationships and emits metadata for planning
// and orchestration decisions.
func BuildArchitectPrompt(planFile string) string {
	return fmt.Sprintf(
		"You are the architect agent. Your job: analyze a plan, identify architectural dependencies, and emit compact metadata for downstream orchestration.\n\n"+
			"Load the `kasmos-architect` and `cli-tools` skills before starting.\n\n"+
			"## Instructions\n\n"+
			"1. Retrieve the plan: `kas task show %[1]s`\n"+
			"2. For each task, classify it as `parallel` when it has no file or execution dependency on other tasks in the same wave; otherwise classify it as serial.\n"+
			"3. Estimate token budgets for each task, including required context depth and expected implementation footprint.\n"+
			"4. Write the enriched plan back: pipe content to `kas task update-content %[1]s`\n"+
			"5. Write architect metadata to `.kasmos/cache/%[1]s-architect.json` using the schema example in `architect-v1.json`.\n"+
			"6. Signal completion: `touch .kasmos/signals/architect-finished-%[1]s`\n"+
			"7. Note: app/FSM consumption of this new architect-finished signal is follow-up work and should be implemented separately.\n",
		planFile,
	)
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
			"2. Signal completion: `kas signal emit planner_finished %[1]s` (or fallback: `touch .kasmos/signals/planner-finished-%[1]s`)\n"+
			"Do not edit plan-state.json directly.",
		planFile,
	)
}

// BuildBlueprintSkipPrompt builds the prompt for a single coder agent that must
// implement all tasks in a small plan sequentially. Used when the plan's task
// count is at or below the blueprint_skip_threshold so wave orchestration is skipped.
// The agent signals implement_finished directly when done, which triggers the
// existing review flow without any wave orchestration machinery.
func BuildBlueprintSkipPrompt(planFile string, plan *taskparser.Plan) string {
	var sb strings.Builder

	totalTasks := 0
	for _, wave := range plan.Waves {
		totalTasks += len(wave.Tasks)
	}

	sb.WriteString(fmt.Sprintf("Implement the entire plan %s as a single coder agent.\n\n", planFile))
	sb.WriteString(fmt.Sprintf("This is a small plan (%d total tasks), so kasmos is skipping wave orchestration.\n", totalTasks))
	sb.WriteString("Implement all tasks sequentially in one session, preserving the plan intent and existing codebase patterns.\n\n")

	header := plan.HeaderContext()
	if header != "" {
		sb.WriteString("## Plan Context\n\n")
		sb.WriteString(header)
		sb.WriteString("\n")
	}

	sb.WriteString("## Tasks\n\n")
	for _, wave := range plan.Waves {
		sb.WriteString(fmt.Sprintf("### Wave %d\n\n", wave.Number))
		for _, task := range wave.Tasks {
			sb.WriteString(fmt.Sprintf("#### Task %d: %s\n\n", task.Number, task.Title))
			sb.WriteString(task.Body)
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("## Completion\n\n")
	sb.WriteString(fmt.Sprintf("When all tasks are implemented and verified, signal completion with `kas signal emit implement_finished %s` (or fallback: `touch .kasmos/signals/implement-finished-%s`).\n", planFile, planFile))

	return sb.String()
}

// BuildMasterReviewPrompt defines the review task prompt for the kasmos-master role.
// Signal consumption is intentionally left for follow-up app/FSM work, so this builder
// only standardizes the instructions and completion signal contract.
func BuildMasterReviewPrompt(planFile, diffContent, testResults string) string {
	return fmt.Sprintf(
		"You are the master review agent. Load the `kasmos-master` skill, read the plan with "+
			"`kas task show %[1]s`, then review the proposed change for plan alignment and merge readiness.\n\n"+
			"## Review Task\n"+
			"- Determine whether the diff should be merged and signal your decision with `touch .kasmos/signals/master-approved-%[1]s` when complete.\n\n"+
			"## Test Results\n%s\n\n"+
			"## Diff\n%s\n",
		planFile,
		testResults,
		diffContent,
	)
}
