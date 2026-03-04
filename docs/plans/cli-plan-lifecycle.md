# CLI Plan Lifecycle Commands

**Goal:** Add `kas task` subcommands for full plan lifecycle management — creating plans with slug derivation and branch setup, starting plan stages (plan/implement/solo/review), pushing branches, merging to main, creating PRs, and resetting plans. This closes the ~70% gap identified in the UI/CLI feature parity report (Category 2: Plan Lifecycle Orchestration).

**Order:** 1 of 4 in CLI parity series. Must complete before `cli-signal-processing.md`. Can NOT run in parallel with signal processing (signal processing depends on plan lifecycle primitives).

**Depends on:** `instance-management-cli.md` (done)

**Architecture:** Extends `cmd/task.go` (or a new `cmd/task_lifecycle.go`) following the established `executeXxx()` + thin cobra wrapper pattern. Plan creation uses the existing `taskstate.TaskState.CreateWithContent()` and `git.GitWorktree` for branch setup. Stage transitions use `taskfsm` events. Git operations (push, merge, PR) call `session/git` package functions directly — no TUI dependency.

**Tech Stack:** Go, cobra, `taskstate`, `taskfsm`, `session/git`, `gh` CLI for PR creation

**Size:** Medium (estimated ~4 hours, 3-4 tasks, 2 waves)

---

## Proposed Commands

```
kas task create <description> [--topic=X]    # full creation: AI slug, branch, DB entry
kas task start <file> <stage>                # stage: plan|implement|solo|review — spawns agent
kas task push <file>                         # push the plan's implementation branch
kas task merge <file>                        # merge plan branch to main
kas task pr <file> --title=X [--body=X]      # create PR for plan branch
kas task start-over <file>                   # reset branch + revert to planning
```

## Key Gaps Being Closed

| TUI Feature | TUI Location | CLI Equivalent |
|-------------|-------------|----------------|
| Create plan (name + description + topic) | `app_state.go:createPlanEntry()` | `kas task create` |
| Start plan stage | `app_actions.go:triggerPlanStage()` | `kas task start` |
| Merge plan branch to main | `app_actions.go:merge_plan` | `kas task merge` |
| Start over plan | `app_actions.go:start_over_plan` | `kas task start-over` |
| Push instance branch | `app_actions.go:push_instance` | `kas task push` |
| Create PR | `app_input.go:statePRTitle/statePRBody` | `kas task pr` |
