# Solo Agent Mode — Design

## Problem

The current plan lifecycle (plan → wave-parsed implement → review → done) has significant overhead for simple tasks. Every implementation requires wave headers, multi-agent orchestration, automatic push/review transitions, and FSM machinery. For small or straightforward tasks — especially those with just a name+description stub in plan-state.json — this is overkill.

## Solution

Add a "start solo agent" action that spawns a single coder agent in an isolated worktree with a minimal prompt. No wave parsing, no automatic FSM transitions after spawn, no review cycle. The user manages the instance manually like any ad-hoc session.

## Design

### Context menu action

Add `"start solo agent"` to the plan context menu for plans in `ready`, `planning`, or `implementing` status. It appears alongside the existing lifecycle actions.

### Agent spawning

Reuse `spawnPlanAgent` with a new `"solo"` stage value that maps to `AgentTypeCoder`. The instance gets:
- A worktree + feature branch (same isolation as normal implement)
- A minimal prompt: `"Implement {name}. Goal: {description}."` — no skill directives
- If a plan `.md` file exists, the prompt references it: `"Implement docs/plans/{file}. Goal: {description}."`

### Instance marking

Set a `SoloAgent bool` field on `Instance` so downstream handlers can identify solo-spawned agents. This is the gate for skipping all lifecycle automation.

### FSM transition

The plan transitions to `implementing` on spawn (so the sidebar shows correct status). No further automatic transitions — no `coderCompleteMsg`, no `promptPushBranchThenAdvance`, no review spawning.

### Exit gates

Two existing handlers need solo-agent gates:

1. **`shouldPromptPushAfterCoderExit`** — returns false when `inst.SoloAgent` is true. This prevents the automatic "push branch and start review?" overlay.

2. **Wave completion monitor** (metadata tick) — already gated on `inst.TaskNumber > 0`, and solo agents have `TaskNumber == 0`, so no wave interference. No change needed.

### Sidebar rendering

Solo-spawned instances appear in the instance list like normal coder instances. The plan's lifecycle sub-items (plan/implement/review) still render in the sidebar — the user can still manually trigger full lifecycle actions if they want to later.

### What doesn't change

- Worktree/branch creation (same as normal implement)
- Instance list behavior (same as any coder)
- Manual context menu actions (user can still "start review", "mark done", etc.)
- FSM state (plan is "implementing" — user manages it from there)
