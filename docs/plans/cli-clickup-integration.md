# CLI ClickUp Integration Commands

**Goal:** Add CLI commands for searching ClickUp tasks and importing them as kasmos plans with automatic planner spawning. Closes Category 6 (ClickUp Integration) in the UI/CLI feature parity report.

**Order:** 4 of 4 in CLI parity series. Fully independent — CAN run in parallel with `cli-signal-processing.md` and `cli-monitoring.md`. Only requires `cli-plan-lifecycle.md` to be done (uses `kas task create` under the hood for plan creation).

**Depends on:** `cli-plan-lifecycle.md`

**Architecture:** Extends the existing `internal/clickup` package with CLI wrappers. The TUI already has ClickUp search (`app_input.go:stateClickUpSearch`) and import (`app_state.go:importClickUpTask()`) — this plan exposes those operations as CLI commands. Import reuses the `kas task create` flow from `cli-plan-lifecycle.md` and optionally spawns a planner agent.

**Tech Stack:** Go, cobra, `internal/clickup`, `taskstate`, `session`

**Size:** Small (estimated ~2 hours, 2 tasks, 1 wave)

---

## Proposed Commands

```
kas clickup search <query>                  # search ClickUp tasks
kas clickup import <task-id>                # import task as plan + optionally spawn planner
```

## Key Gaps Being Closed

| TUI Feature | TUI Location | CLI Equivalent |
|-------------|-------------|----------------|
| Search ClickUp tasks | `app_input.go:stateClickUpSearch` | `kas clickup search` |
| Import ClickUp task as plan | `app_state.go:importClickUpTask()` | `kas clickup import` |
| Post progress comments | `clickup_progress.go` | Future (could be `kas clickup update`) |
