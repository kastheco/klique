# Wave Orchestration Design

## Problem

Plan implementation is single-threaded: one Claude Code instance executes all tasks sequentially. Plans with independent tasks (touching different files/packages) waste time running serially when they could run in parallel. There's no native way for klique to manage task-level parallelism — the only parallelism option is the `subagent-driven-development` skill, which runs subagents inside a single session rather than as visible, independent instances.

## Solution: Wave-Based Parallel Task Execution

The planner groups tasks into **waves** during the planning stage. Tasks within a wave are independent and run in parallel as separate Claude Code instances on a shared worktree. Waves execute sequentially — wave N+1 starts only after all of wave N completes and the user confirms.

## Plan Format

Plans use explicit `## Wave N` headers to group tasks:

```markdown
# Feature Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans...

**Goal:** ...
**Architecture:** ...
**Tech Stack:** ...
**Waves:** 3 (T1,T2 parallel → T3,T4 parallel → T5 sequential)

---

## Wave 1
### Task 1: Update Key Definitions
...
### Task 2: Add Theme Constants
...

## Wave 2
### Task 3: Rewrite Key Routing
...
### Task 4: Update Tests
...

## Wave 3
### Task 5: Final Verification
...
```

**Planner responsibility:** Analyze file dependencies between tasks. Tasks touching entirely different files/packages go in the same wave. Tasks depending on earlier tasks' output go in later waves. When in doubt, later wave (sequential is always safe).

**Validation gate:** When the user clicks "Implement", klique parses for `## Wave` headers. If none found:
- Plan status reverts to `planning`
- Toast: "Plan needs wave annotations before implementation. Returning to planning."
- Planner agent respawned with prompt to add wave groupings

All plans must have wave headers to reach implementation. Legacy plans without them get sent back for annotation.

## Wave Orchestration Engine

### State Machine

```
Idle → Wave1Running → Wave1Complete → (user confirms) → Wave2Running → ... → AllWavesComplete
```

### Component: WaveOrchestrator

Lives in the app layer. One per plan being implemented. Manages:

- Parsing plan into waves/tasks
- Spawning task instances for the current wave
- Monitoring `PromptDetected` on all wave instances (via existing metadata tick)
- Wave transition confirmations
- Failure handling

### Execution Flow

1. User clicks "Implement" on a plan
2. Klique parses plan into `[]Wave`, each containing `[]Task{Number, Title, Body}`
3. Creates `WaveOrchestrator` for this plan
4. Spawns all Wave 1 tasks as separate instances on the shared plan worktree
5. Each instance gets a prompt containing:
   - Plan header (Goal, Architecture, Tech Stack) for context
   - Full task body (steps, code, expected output)
   - "Load the `cli-tools` skill before starting."
   - "You are implementing Task N of a multi-task plan. Other tasks in this wave are running in parallel on the same worktree. Only modify the files listed in your task."
6. Orchestrator monitors `PromptDetected` on all wave instances

### Wave Transition

- All tasks in current wave show `PromptDetected` → orchestrator sets state to `WaveNComplete`
- Confirmation overlay: "Wave 1 complete (3/3). Start Wave 2?"
- On confirm: auto-pause wave 1 instances, spawn wave 2 tasks
- On cancel/dismiss: stay in `WaveNComplete`, user inspects and advances manually later

### Completion Detection

Uses existing `PromptDetected` field on instances. When the Claude Code agent finishes its task and returns to the prompt, `PromptDetected` becomes true. This is the natural "task complete" signal — no new detection mechanism needed.

### Failure Handling

- Task instance dies or user kills it → orchestrator marks task as failed
- Other tasks in the wave continue running (don't waste their work)
- When remaining tasks complete, confirmation changes to: "Wave 1: 2/3 complete, 1 failed. Retry failed task / Skip to Wave 2 / Abort"

### Completed Task Instances

When a wave finishes and the user confirms advancing:
- Completed wave's task instances are auto-paused (tmux killed, worktree kept)
- They dim in the instance list (existing pause rendering)
- User can resume any to inspect the work

## Instance List Integration

### Instance Fields

Each task instance is a normal `session.Instance` with:
- `PlanFile` (existing) — groups under plan in sidebar tree
- `Title` = `{plan-display-name}-T{N}`
- New: `TaskNumber int` — task number in the plan (1-indexed)
- New: `WaveNumber int` — wave this task belongs to

### Rendering

- Task instances cluster under their plan header in the sidebar tree (existing plan grouping)
- Each shows task number and short name from plan heading (e.g., `T1: Update Key Definitions`)
- Subtle `W1` badge next to status indicator in `ColorMuted`
- Running tasks: normal rendering with active status
- Paused tasks (completed waves): dimmed, paused status (existing rendering)
- Future wave tasks: not spawned yet, not in the list

## Edge Cases

- **User kills all tasks in a wave:** Orchestrator detects no running instances, shows "Wave N: all tasks stopped. Retry wave / Abort?"
- **User pauses a task mid-wave:** Treated like failure — other tasks continue, wave won't advance until paused task is resumed and completes (or user skips it)
- **Plan has 1 wave with 1 task:** Degenerates to single-instance behavior, no special casing
- **Concurrent plan implementations:** Each plan gets its own WaveOrchestrator, independent — different plans, different worktrees

## writing-plans Skill Update

The `writing-plans` skill is updated to:
- Always produce wave-annotated plans
- Include `**Waves:**` summary in plan header
- Group tasks by file dependency analysis
- Instruct each task to load `cli-tools` skill

## Dependencies

- Existing: `session.Instance`, `PlanFile` field, `PromptDetected`, shared worktree via `git.NewSharedPlanWorktree`
- Existing: sidebar tree plan grouping, instance pause/resume lifecycle
- Existing: metadata tick for monitoring instance state
- New: `planparser` package for wave/task extraction from markdown
- New: `WaveOrchestrator` in app layer
- New: `TaskNumber`, `WaveNumber` fields on Instance
