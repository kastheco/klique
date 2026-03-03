# Orphaned Tmux Session Recovery for Plans

## Problem

When kasmos crashes while a plan agent is running, the tmux session survives but kasmos loses its reference to it on restart. The plan shows "planning" or "implementing" status in the sidebar, but there's no instance to interact with. The user's only recourse is manually running `tmux kill-session` and restarting the plan from scratch — losing any agent work in progress.

## Design

### Detection

On startup (and when `updateSidebarPlans` builds `PlanDisplay` entries), check for orphaned tmux sessions that match plan agent naming. Plan agents create tmux sessions named `kas_<plan-display-name>-<action>` (e.g. `kas_clickup-import-plan`). For each plan in "planning" or "implementing" state that has no managed instance, probe `tmux has-session -t=<expected-name>` to detect survivors.

Add a `HasOrphanedSession bool` field to `PlanDisplay` and a helper function that derives the expected tmux session name from a plan filename + action and checks existence.

### Sidebar Indicator

The plan row already has three dot states:
- `●` foam/green (`HasRunning`) — active managed instance
- `◉` rose (`HasNotification`) — needs attention (reviewing)
- `○` muted — idle

Add a fourth state: `◎` gold (`HasOrphanedSession`) — recoverable session exists but no managed instance. Priority order: notification > running > orphaned > idle.

### Context Menu

When a plan has `HasOrphanedSession == true`, add two items to the plan context menu:
- **"Attach to session"** (first position) — creates a new `Instance`, wires up the existing tmux session via `Restore()`, and adds it to the instance list as a running session.
- **"Kill stale session"** — kills the orphaned tmux session and resets plan state to "ready" via FSM.

### Attach Flow

1. Derive the tmux session name: `kas_` + sanitized(`<displayName>-<action>`)
2. Create a new `Instance` via `NewInstance` with the plan's title and path
3. Create a `TmuxSession` with the same name, set it on the instance
4. Call `Restore()` on the `TmuxSession` (attaches PTY to existing session)
5. Mark instance as started/running, add to instance list, save state

### Kill Flow

1. Run `tmux kill-session -t=<session-name>`
2. Transition plan state: if "planning" → reset to "ready"; if "implementing" → keep as "implementing" (partial work may exist on branch)
3. Refresh sidebar

## Files Changed

- `ui/sidebar.go` — `PlanDisplay.HasOrphanedSession`, `renderPlanRow` gold dot, `sidebarRow.HasOrphanedSession`
- `app/app_state.go` — `updateSidebarPlans` orphan detection, attach/kill helper functions
- `app/app_actions.go` — context menu items, `executeContextAction` cases
- `session/tmux/tmux.go` — export `toKasTmuxName` or add `SessionExists(name)` helper
