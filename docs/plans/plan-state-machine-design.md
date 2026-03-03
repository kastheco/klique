# Plan State Machine Gateway Design

## Problem

Plan lifecycle management is fragile and unreliable. The root causes:

1. **Scattered mutations** — 22+ `SetStatus` call sites across 4 files with no centralized validation
2. **Race conditions** — metadata tick goroutine reads stale state and overwrites in-memory state that was just updated by a user action (documented double-reviewer-spawn bug)
3. **Two-writer problem** — both klique TUI and Claude agents write plan-state.json with no locking
4. **Status confusion** — 9 statuses with overlapping semantics (`done` vs `completed` vs `finished`, `in_progress` vs `implementing`), no enforced transition rules
5. **Agent coupling** — planner agent writes directly to plan-state.json; any agent can corrupt state

## Solution: Centralized FSM + Sentinel Files

Replace all scattered `SetStatus` calls with a `PlanStateMachine` that enforces valid transitions and is the sole writer of plan-state.json. Agents communicate state changes via sentinel files instead of modifying plan-state.json directly.

## Status Consolidation

| Current statuses                       | New status     | Meaning                               |
|----------------------------------------|----------------|---------------------------------------|
| `ready`                                | `ready`        | Plan written, awaiting implementation |
| `planning`                             | `planning`     | Planner agent actively working        |
| `implementing`, `in_progress`          | `implementing` | Coder agent(s) actively working       |
| `reviewing`                            | `reviewing`    | Reviewer agent actively working       |
| `done`, `completed`, `finished`        | `done`         | Terminal success state                |
| `cancelled`                            | `cancelled`    | Terminal failure state                |

## FSM Transitions

```
ready          → planning          (PlanStart)
planning       → ready             (PlannerFinished)
ready          → implementing      (ImplementStart)
implementing   → reviewing         (ImplementFinished)
reviewing      → done              (ReviewApproved)
reviewing      → implementing      (ReviewChangesRequested)
done           → implementing      (StartOver)              [user-only]
any            → cancelled         (Cancel)                  [user-only]
cancelled      → planning          (Reopen)                  [user-only]
```

Events marked `[user-only]` can only be triggered from the TUI, never by agent sentinels. Any transition not in this table is rejected with an error.

## Sentinel Files

Agents communicate lifecycle events by dropping files in `docs/plans/.signals/` (gitignored).

**Format:** `<event>-<planfile>` e.g. `planner-finished-2026-02-22-bubblezone.md`

**Supported sentinels:**

| Sentinel prefix      | FSM event              | Body contents          |
|----------------------|------------------------|------------------------|
| `planner-finished-`  | PlannerFinished        | empty                  |
| `implement-finished-`| ImplementFinished      | empty                  |
| `review-approved-`   | ReviewApproved         | empty                  |
| `review-changes-`    | ReviewChangesRequested | review feedback (text) |

**Processing flow:**
1. Metadata tick scans `docs/plans/.signals/`
2. For each sentinel file, parse event and plan file from the filename
3. Feed event to FSM — if valid transition, update state and delete sentinel
4. If invalid transition, log warning and delete sentinel (don't accumulate garbage)
5. For `review-changes-*`, read file body and pass to coder session as prompt context

**Agent skill instructions** updated to say: "Write your sentinel to `.signals/<event>-<planfile>` when done. Never modify plan-state.json directly."

## File Locking

- `flock` on `docs/plans/.plan-state.lock` for every write
- `FSM.Transition()` acquires lock → read current state → validate transition → write → unlock
- Metadata tick goroutine reads without lock (stale reads are acceptable — it only detects events)
- All mutations flow through `FSM.Transition()` — no direct `SetStatus` calls anywhere

## Architecture

```
                  ┌─────────────┐
                  │  Agent      │
                  │  (Claude)   │
                  └──────┬──────┘
                         │ writes sentinel file
                         ▼
              docs/plans/.signals/
              planner-finished-foo.md
                         │
                         │ metadata tick scans
                         ▼
              ┌──────────────────────┐
              │  PlanStateMachine    │
              │                      │
              │  Transition(file,    │◄── TUI actions (user-only events)
              │            event)    │
              │                      │
              │  - validates FSM     │
              │  - acquires flock    │
              │  - writes JSON       │
              │  - returns new state │
              └──────────┬───────────┘
                         │
                         ▼
              docs/plans/plan-state.json
              (sole writer, version controlled)
```

## What Changes in App Code

### New package: `config/planfsm`
- `PlanStateMachine` struct with `Transition(planFile string, event Event) error`
- Event type enum: `PlanStart`, `PlannerFinished`, `ImplementStart`, etc.
- Transition table as a `map[Status]map[Event]Status`
- File locking via `flock`
- Sentinel scanner: `ScanSignals(dir string) []Signal`

### Refactored call sites
- All 22 `SetStatus` calls → `m.fsm.Transition(planFile, event)`
- `planStageStatus()` function deleted — FSM replaces it
- `checkPlanCompletion()` replaced by sentinel scanning
- Planner-exit, coder-exit, reviewer-exit detection in metadata tick simplified — they feed events to the FSM instead of calling `SetStatus` directly

### Metadata tick changes
- Goroutine scans `.signals/` directory alongside existing metadata collection
- Returns detected signals as part of `metadataResultMsg`
- Update handler feeds signals to FSM in the main goroutine (no concurrent writes)

### Superpowers skill updates
- Planner skill: "write `planner-finished-<file>` to `.signals/` when plan is complete"
- Executing-plans skill: "write `implement-finished-<file>` to `.signals/` when all tasks done"
- Review skill: "write `review-approved-<file>` or `review-changes-<file>` to `.signals/`"

### Migration
- On first load, map old statuses to new ones (`completed` → `done`, `in_progress` → `implementing`, `finished` → `done`)
- Keep reading old format, write new format
- One-time migration, no backwards compatibility needed after

## Files Changed

- New: `config/planfsm/fsm.go` — state machine, transition table, file locking
- New: `config/planfsm/signals.go` — sentinel file scanner
- New: `config/planfsm/fsm_test.go` — transition validation tests
- Modify: `config/planstate/planstate.go` — remove `SetStatus`, keep read-only queries
- Modify: `app/app.go` — add `fsm *planfsm.PlanStateMachine` field, sentinel scanning in metadata tick
- Modify: `app/app_actions.go` — replace all `SetStatus` calls with `fsm.Transition`
- Modify: `app/app_state.go` — replace `SetStatus` calls, simplify completion checks
- Modify: `app/app_input.go` — replace `SetStatus` calls
- Modify: superpowers skills (planner, executing-plans, review)
