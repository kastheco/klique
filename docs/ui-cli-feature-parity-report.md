# kasmos UI vs CLI Feature Parity Report

## Executive Summary

kasmos has a **massive gap** between what the TUI can do and what's available headlessly. The TUI (`app/`) is the primary control surface — it contains ~95% of the orchestration logic. The CLI (`cmd/`) exposes only plan state queries and a plan store HTTP server. There is **no CLI surface** for instance management, agent spawning, wave orchestration, git operations, or signal processing. An agent operating headlessly today can only read/write plan metadata via the HTTP API or the `kas plan` subcommands.

## Current CLI Surface

| Command                                     | What it does                                |
| ------------------------------------------- | ------------------------------------------- |
| `kas` (root)                                  | Launches the TUI — not headless             |
| `kas plan list [--status=X]`                  | List plans with optional status filter      |
| `kas plan register <file>`                    | Register an untracked plan file             |
| `kas plan set-status <file> <status> --force` | Force-override plan status (bypasses FSM)   |
| `kas plan transition <file> <event>`          | Apply FSM event to a plan                   |
| `kas plan implement <file> --wave=N`          | Write a wave signal file (TUI picks it up)  |
| `kas plan link-clickup`                       | Backfill ClickUp task IDs from plan content |
| `kas serve`                                   | Start the plan store HTTP server            |
| `kas reset`                                   | Delete all instances, clean tmux/worktrees  |
| `kas debug`                                   | Print config paths                          |
| `kas version`                                 | Print version                               |
| `kas setup`                                   | Interactive agent harness wizard            |

## Current HTTP API Surface (plan store)

| Endpoint                                                       | Method | What it does                                 |
| -------------------------------------------------------------- | ------ | -------------------------------------------- |
| `/v1/ping`                                                       | GET    | Health check                                 |
| `/v1/projects/{project}/plans`                                   | GET    | List plans (with `?status=` / `?topic=` filters) |
| `/v1/projects/{project}/plans`                                   | POST   | Create plan                                  |
| `/v1/projects/{project}/plans/{filename}`                        | GET    | Get plan metadata                            |
| `/v1/projects/{project}/plans/{filename}`                        | PUT    | Update plan metadata                         |
| `/v1/projects/{project}/plans/{filename}/content`                | GET    | Get plan markdown content                    |
| `/v1/projects/{project}/plans/{filename}/content`                | PUT    | Set plan markdown content                    |
| `/v1/projects/{project}/plans/{filename}/clickup-task-id`        | PUT    | Set ClickUp task ID                          |
| `/v1/projects/{project}/plans/{filename}/increment-review-cycle` | POST   | Increment review cycle                       |
| `/v1/projects/{project}/plans/{filename}/rename`                 | POST   | Rename plan                                  |
| `/v1/projects/{project}/topics`                                  | GET    | List topics                                  |
| `/v1/projects/{project}/topics`                                  | POST   | Create topic                                 |

## TUI-Only Features (No CLI/API Equivalent)

### Category 1: Instance Lifecycle Management (Critical)

These are the core operations an agent needs to interact with kasmos headlessly.

| TUI Feature                                               | TUI Location                     | Gap Severity |
| --------------------------------------------------------- | -------------------------------- | ------------ |
| **Create instance** (new agent session)                       | `app_input.go:KeyPrompt`           | **Critical**     |
| **Spawn ad-hoc agent** (name + branch + workpath)             | `app_state.go:spawnAdHocAgent()`   | **Critical**     |
| **Spawn plan agent** (planner/coder/reviewer/solo)            | `app_state.go:spawnPlanAgent()`    | **Critical**     |
| **Kill instance** (soft kill — tmux only)                     | `app_input.go:KeyKill`             | **Critical**     |
| **Abort instance** (full — tmux + worktree + remove)          | `app_input.go:KeyAbort`            | **Critical**     |
| **Pause instance** (commit + detach + remove worktree)        | `app_actions.go:pause_instance`    | **Critical**     |
| **Resume instance** (recreate worktree + restart tmux)        | `app_input.go:KeyResume`           | **Critical**     |
| **Restart instance** (kill tmux + fresh start, keep worktree) | `app_actions.go:restart_instance`  | **Critical**     |
| **List instances** (with status, plan, agent type)            | `nav.GetInstances()`               | **Critical**     |
| **Get instance details** (status, branch, plan, diff stats)   | `app_state.go:updateInfoPane()`    | **Critical**     |
| **Send prompt to instance**                                   | `app_input.go:stateSendPrompt`     | **Critical**     |
| **Send "yes" to waiting instance**                            | `app_input.go:KeySendYes`          | **Critical**     |
| **Rename instance**                                           | `app_input.go:stateRenameInstance` | Medium       |

### Category 2: Plan Lifecycle Orchestration (Critical)

| TUI Feature                                                           | TUI Location                                    | Gap Severity                                                                                    |
| --------------------------------------------------------------------- | ----------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| **Create plan** (name + description + topic)                              | `app_state.go:createPlanEntry()`                  | **Critical** — partially covered by HTTP API `POST /plans` but no branch creation, no slug derivation |
| **Start plan stage** (plan/implement/solo/review)                         | `app_actions.go:triggerPlanStage()`               | **Critical**                                                                                        |
| **Wave orchestration** (parse plan, spawn wave tasks, monitor completion) | `wave_orchestrator.go` + `app.go:metadataResultMsg` | **Critical**                                                                                        |
| **Advance to next wave**                                                  | `app.go:waveAdvanceMsg`                           | **Critical**                                                                                        |
| **Retry failed wave**                                                     | `app.go:waveRetryMsg`                             | **Critical**                                                                                        |
| **Abort wave**                                                            | `app.go:waveAbortMsg`                             | **Critical**                                                                                        |
| **Spawn reviewer after implementation**                                   | `app_state.go:spawnReviewer()`                    | **Critical**                                                                                        |
| **Spawn coder with reviewer feedback**                                    | `app_state.go:spawnCoderWithFeedback()`           | **Critical**                                                                                        |
| **Mark plan done**                                                        | `app_actions.go:mark_plan_done`                   | Medium — achievable via `kas plan transition` but requires knowing the right FSM events           |
| **Cancel plan**                                                           | `app_actions.go:cancel_plan`                      | Medium — achievable via `kas plan transition`                                                     |
| **Start over plan** (reset branch + revert to planning)                   | `app_actions.go:start_over_plan`                  | **Critical**                                                                                        |
| **Merge plan branch to main**                                             | `app_actions.go:merge_plan`                       | **Critical**                                                                                        |
| **Rename plan**                                                           | `app_input.go:stateRenamePlan`                    | Low — achievable via HTTP API                                                                   |
| **Change plan topic**                                                     | `app_input.go:stateChangeTopic`                   | Low — achievable via HTTP API                                                                   |
| **Set plan status** (force override)                                      | `app_input.go:stateSetStatus`                     | Low — achievable via `kas plan set-status`                                                        |

### Category 3: Git Operations (Critical)

| TUI Feature                              | TUI Location                          | Gap Severity |
| ---------------------------------------- | ------------------------------------- | ------------ |
| **Push instance branch**                     | `app_actions.go:push_instance`          | **Critical**     |
| **Create PR** (title + body + commit + push) | `app_input.go:statePRTitle/statePRBody` | **Critical**     |
| **Copy worktree path**                       | `app_actions.go:copy_worktree_path`     | Low          |
| **Copy branch name**                         | `app_actions.go:copy_branch_name`       | Low          |

### Category 4: Signal Processing (Critical)

| TUI Feature                                                                                   | TUI Location                              | Gap Severity |
| --------------------------------------------------------------------------------------------- | ----------------------------------------- | ------------ |
| **Process agent signals** (planner-finished, implement-finished, review-approved, review-changes) | `app.go:metadataResultMsg` signal loop      | **Critical**     |
| **Process wave signals**                                                                          | `app.go:metadataResultMsg` wave signal loop | **Critical**     |
| **Auto-spawn reviewer on implement-finished**                                                     | `app.go` signal handler                     | **Critical**     |
| **Auto-spawn coder on review-changes-requested**                                                  | `app.go` signal handler                     | **Critical**     |

### Category 5: Monitoring & Observability (Important)

| TUI Feature                                                               | TUI Location                           | Gap Severity                                          |
| ------------------------------------------------------------------------- | -------------------------------------- | ----------------------------------------------------- |
| **Instance metadata polling** (status, prompt detection, diff stats, CPU/mem) | `app.go:tickUpdateMetadataMessage`       | **Important**                                             |
| **Audit log** (structured events for all actions)                             | `config/auditlog/` — writes to SQLite    | Medium — data exists in SQLite but no CLI to query it |
| **Tmux session browser** (discover orphaned sessions)                         | `app_actions.go:handleTmuxBrowserAction` | Medium                                                |
| **Adopt orphan tmux session**                                                 | `app_actions.go:BrowserAdopt`            | Medium                                                |
| **Permission prompt detection + auto-approve**                                | `app.go:statePermission`                 | **Important**                                             |

### Category 6: ClickUp Integration (Medium)

| TUI Feature                       | TUI Location                     | Gap Severity |
| --------------------------------- | -------------------------------- | ------------ |
| **Search ClickUp tasks**              | `app_input.go:stateClickUpSearch`  | Medium       |
| **Import ClickUp task as plan**       | `app_state.go:importClickUpTask()` | Medium       |
| **Post progress comments to ClickUp** | `clickup_progress.go`              | Medium       |

### Category 7: Configuration & UI State (Low)

| TUI Feature                                     | TUI Location                       | Gap Severity |
| ----------------------------------------------- | ---------------------------------- | ------------ |
| **Toggle auto-advance waves**                       | `app_actions.go:toggle_auto_advance` | Low          |
| **View plan markdown** (rendered)                   | `app_state.go:viewSelectedPlan()`    | Low          |
| **Chat about plan** (spawn agent with plan context) | `app_input.go:stateChatAboutPlan`    | Low          |

## Recommended CLI Commands for Feature Parity

Based on the gap analysis, here's the minimum set of CLI subcommands needed for headless operation, ordered by priority:

### Wave 1: Instance Management (enables basic headless agent control)

```
kas instance list [--status=X] [--plan=X] [--agent-type=X]
kas instance create <name> [--program=X] [--branch=X] [--path=X] [--plan=X] [--agent-type=X]
kas instance kill <name>
kas instance abort <name>
kas instance pause <name>
kas instance resume <name>
kas instance restart <name>
kas instance send-prompt <name> <prompt>
kas instance send-yes <name>
kas instance get <name>                    # JSON output: status, branch, diff stats, etc.
```

### Wave 2: Plan Lifecycle (enables headless plan orchestration)

```
kas plan create <description> [--topic=X]  # full creation: slug, branch, DB entry
kas plan start <file> <stage>              # stage: plan|implement|solo|review
kas plan push <file>                       # push the plan's implementation branch
kas plan merge <file>                      # merge plan branch to main
kas plan pr <file> --title=X [--body=X]    # create PR for plan branch
kas plan start-over <file>                 # reset branch + revert to planning
```

### Wave 3: Signal Processing (enables autonomous lifecycle)

```
kas signal process [--once]                # scan + process signals (run in loop or once)
kas signal list                            # show pending signals
```

This is the most architecturally significant gap. Today, signal processing is **embedded in the TUI's `Update()` loop**. For headless operation, this logic needs to be extracted into a standalone signal processor that can run as a daemon or be invoked periodically.

### Wave 4: Monitoring & Observability

```
kas audit list [--limit=N] [--event=X]     # query audit log
kas tmux list                              # list orphaned kas_ tmux sessions
kas tmux adopt <session-name> <title>      # adopt orphan into instance list
kas tmux kill <session-name>               # kill orphan session
kas instance status                        # summary: running/ready/paused counts
```

### Wave 5: ClickUp Integration

```
kas clickup search <query>                 # search ClickUp tasks
kas clickup import <task-id>               # import task as plan + spawn planner
```

## Architectural Observations

1. **Signal processing is the linchpin.** The TUI's `metadataResultMsg` handler is a ~300-line state machine that processes agent signals, spawns follow-up agents, manages wave orchestration, and handles permission prompts. This is the single biggest piece of logic that needs extraction for headless parity. Without it, the plan lifecycle cannot advance autonomously.

2. **The daemon (`daemon.go`) is primitive.** It only does auto-yes (pressing enter on prompts). It doesn't process signals, manage waves, or handle plan lifecycle transitions. A headless kasmos needs a much richer daemon — essentially the signal processing loop from the TUI running without a screen.

3. **Instance state is file-based.** Instances are serialized to `~/.config/kasmos/state.json` via `session.Storage`. The CLI can read/write this, but concurrent access with the TUI would need coordination (file locking or moving to SQLite).

4. **The plan store HTTP API is solid.** Plan metadata CRUD is well-covered. The gap is in the *orchestration* layer that sits on top of it — spawning agents, processing signals, managing waves.

5. **The `session` package is CLI-ready.** `Instance`, `git.GitWorktree`, and `tmux.TmuxSession` are all usable without the TUI. The TUI wraps them in bubbletea messages, but the underlying operations are synchronous Go functions that a CLI can call directly.

## Summary

| Category           | TUI Features | CLI/API Coverage | Gap          |
| ------------------ | ------------ | ---------------- | ------------ |
| Instance lifecycle | 13           | 0                | **100% gap**     |
| Plan orchestration | 14           | 5 (partial)      | **~70% gap**     |
| Git operations     | 4            | 0                | **100% gap**     |
| Signal processing  | 4            | 0                | **100% gap**     |
| Monitoring         | 5            | 0                | **100% gap**     |
| ClickUp            | 3            | 1 (link-clickup) | **~80% gap**     |
| Config/UI state    | 3            | 1 (debug)        | Low priority |

**Bottom line:** kasmos is currently a TUI-first application with a thin CLI veneer for plan queries. Reaching headless feature parity requires extracting the orchestration logic from `app/` into a shared `orchestrator` or `engine` package, then exposing it through both CLI subcommands and (optionally) additional HTTP API endpoints. The signal processing loop is the critical path — without it, no autonomous agent lifecycle is possible outside the TUI.
