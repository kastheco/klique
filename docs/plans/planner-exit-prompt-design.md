# Planner-Exit Auto-Prompt Design

## Problem

When a planner session finishes and writes `ready` to plan-state.json, the dead session sits in the instance list with no indication the user needs to act. The only way to start implementation is through the sidebar context menu — but nothing tells the user this. The UX gap between "planner done" and "implementation starts" is invisible.

## Solution: Detect Planner Death → Show Confirmation Dialog

Mirror the existing coder-exit and reviewer-exit detection patterns in the metadata tick loop. When a planner's tmux pane dies and the plan status is `ready`, show a confirmation dialog asking whether to start implementation.

## Detection Logic

In the metadata tick handler (after reviewer-exit, before coder-exit), iterate instances looking for:
- `AgentType == AgentTypePlanner`
- `PlanFile != ""`
- `!tmuxAlive` (pane has exited)
- Plan status is `StatusReady` (planner successfully wrote the plan back)
- Not already showing a confirm overlay (`m.state != stateConfirm`)

A `plannerPrompted map[string]bool` field on the `home` struct prevents the prompt from re-firing every tick after the user has already responded.

## Three Outcomes

| Key | Action | Instance | Plan Status |
|-----|--------|----------|-------------|
| **y** (confirm) | Kill planner instance, trigger `triggerPlanStage(planFile, "implement")` | Removed | → `implementing` |
| **n** (cancel) | Kill planner instance, leave plan at `ready` | Removed | Stays `ready` |
| **esc** | No-op, preserve everything | Stays | Stays `ready` |

On **esc**, the plan file is NOT added to `plannerPrompted`, so the prompt reappears on the next metadata tick (same pattern as `ResetConfirm` on wave-confirm cancel).

## Cleanup

On yes/no: remove the dead planner instance from `allInstances`, call `saveAllInstances()`, update sidebar. This prevents the dead session from lingering in the instance list.

## New Types

- `plannerCompleteMsg{planFile string}` — dispatched by the yes action, handled in `Update` to call `triggerPlanStage(planFile, "implement")`

## Files Changed

- `app/app.go` — add `plannerPrompted` field, planner-exit detection in metadata tick, `plannerCompleteMsg` handler in `Update`
- `app/app_input.go` — no/esc handling needs to kill planner instance and mark `plannerPrompted` (for no) or skip it (for esc)
- `app/app_state.go` — helper to find and kill a dead planner instance by plan file
