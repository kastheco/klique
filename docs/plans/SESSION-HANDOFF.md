# Session Handoff — 2026-02-22

## What Was Done (Stages 0 + 1)

### Stage 0: Housekeeping
- Marked `tab-focus-ring` → `done` in plan-state.json (already merged PR #9)

### Stage 1A: Dupe task bug fix
- **Commit:** `fix: guard checkPlanCompletion against duplicate reviewer spawn from stale plan state`
- **Root cause:** Async metadata tick goroutine loads plan state snapshot. If it runs between `transitionToReview()` setting `StatusReviewing` in memory and the next tick, the stale snapshot overwrites `m.planState` back to `StatusDone`, causing `checkPlanCompletion` to spawn a second reviewer.
- **Fix:** Added a pre-scan in `checkPlanCompletion()` that builds a `reviewerPlans` map of plans already having a reviewer instance. Skips plans in that map regardless of plan-state contents.
- **File:** `app/app_state.go` lines ~563-590

### Stage 1B: Shortcut hints update
- **Commit:** `refactor: update shortcut hints (K kill, i interactive, space shortcuts, hide P/c/R)`
- `D` → `K` for kill (keys.go, help.go)
- `"focus agent"` → `"interactive"` (keys.go, help.go)
- `"actions"` → `"shortcuts"` (keys.go)
- Removed from bottom bar: `P` (create PR), `c` (checkout), `R` (switch repo)
- Removed from help screen: P/c from Handoff, R from Navigation
- **Files:** `keys/keys.go`, `ui/menu.go`, `app/help.go`

### Plan state update
- **Commit:** `chore(plans): mark tab-focus-ring, update-shortcut-hints, dupe-task done; add roadmap`
- Added `docs/plans/PRIORITY-ROADMAP.md` with full analysis

## Build Status
- `go build ./...` — clean
- `go test ./...` — all passing

## Bug to Fix: Wave Orchestration Can't Start Implementation

### Problem (screenshot at ~/screenshot-bad-start.png)
User selected `wave-orchestration` in sidebar, expanded it, and saw:
```
✓ Plan
· Implement
○ Review
○ Finished
```
Clicking "Plan" spawned a **planner agent** that started exploring the codebase from scratch. The planner doesn't know `docs/plans/2026-02-22-wave-orchestration.md` already has a complete 1287-line implementation plan.

Clicking "Implement" did nothing because `isLocked()` in `app/app_actions.go:487-498` returns true when `status == StatusReady` for the implement stage. The plan needs to pass through `StatusPlanning` → `StatusImplementing` first.

### Root Cause
Two issues:
1. **Plan status stuck at `ready`** — the plan-state.json still shows `"status": "ready"` for wave-orchestration, even though the plan doc is fully written. There's no mechanism to detect "plan file has content, skip to implement".
2. **isLocked gate** — `isLocked("implement")` requires status != `StatusReady`, so the user can't click Implement directly on a `ready` plan.

### How to Fix
The simplest approach: manually advance the wave-orchestration plan status to `"planning"` or `"in_progress"` in plan-state.json, which would unlock the Implement stage. Or change `isLocked` to allow implement when plan doc exists on disk with wave headers.

But the deeper UX issue is: plans created with a full plan doc already written (like wave-orchestration, which was authored in this chat session) need a way to skip the planner stage.

## What's Next (from PRIORITY-ROADMAP.md)

```
DONE       Stage 0: mark tab-focus-ring done
DONE       Stage 1A: dupe-task bug fix
DONE       Stage 1B: update-shortcut-hints
─────────────────────────────────────────────
NEXT       Stage 2: wave-orchestration                [SEPARATE session]
           - Fix the start-plan bug first (see above)
           - Then implement the 7-task plan
─────────────────────────────────────────────
THEN       Stage 3A: center-col-v-align ─────┐        [worktrees OK]
           Stage 3B: bubblezone ─────────────┘
─────────────────────────────────────────────
LATER      Stage 4A: contextual-status-bar ──┐
           Stage 4B: detect-permissions ─────┤         [separate sessions]
           Stage 4C: superpowers bundle ─────┘
─────────────────────────────────────────────
LAST       Stage 5: rename-to-kasmos                   [SEPARATE session]
```

## Key Files for Next Session
- `docs/plans/PRIORITY-ROADMAP.md` — full roadmap
- `docs/plans/2026-02-22-wave-orchestration.md` — 7-task implementation plan (1287 lines)
- `docs/plans/2026-02-22-wave-orchestration-design.md` — design doc (149 lines)
- `docs/plans/plan-state.json` — plan status tracking
- `app/app_actions.go:487-498` — `isLocked()` gate preventing implement
- `app/app_actions.go:430-467` — `executePlanStageAction` where stages dispatch
- `~/screenshot-bad-start.png` — screenshot of the broken wave-orchestration start
