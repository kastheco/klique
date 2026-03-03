# Plan-Aware Orchestration + Plan Browser Design

## Overview

Two features that make plans first-class citizens in klique:

1. **Plan Browser** — Unfinished plans appear in the sidebar. Selecting one spawns a coder
   session pre-loaded with the plan. Done plans disappear.

2. **Plan-Aware Orchestration** — When a coder session finishes all tasks in a plan, klique
   automatically spawns a reviewer session using a template. Detected via plan-state.json
   polling (primary) and session exit (fallback).

## Plan State Lifecycle

```
ready → in_progress → reviewing → done
```

- `ready` / `in_progress`: Set by coder agent (existing behavior)
- `reviewing`: Set by klique when all plan tasks are done, reviewer session spawned
- `done`: Set by klique when reviewer session completes

## Sidebar

Plans section appears above topics. Entries are plans with status `ready` or `in_progress`
or `reviewing`. Done plans are filtered out.

```
┌─────────────────┬──────────────────────────────────┐
│ ▾ Plans         │  instance list...                │
│   ● skills-dist │                                  │
│   ○ other-plan  │                                  │
│ ▾ All           │                                  │
│ ▾ topic-1       │                                  │
│   ...           │                                  │
└─────────────────┴──────────────────────────────────┘
● = in_progress   ○ = ready   ◉ = reviewing
```

Selecting a plan + Enter spawns a coder session with an initial prompt referencing the plan
file. The instance is tagged with `PlanFile` so klique tracks the binding.

## Instance ↔ Plan Binding

`Instance` gains an optional `PlanFile string` field (persisted in session storage). Used to:
- Know which plan to poll for task completion
- Show plan name in instance metadata
- Prevent duplicate sessions for the same plan

## Completion Detection

**Primary (poll):** On the existing tick loop, if an instance has `PlanFile` set, klique reads
`plan-state.json` and checks whether all entries are `done`. When they are, triggers reviewer
spawn.

**Fallback (session exit):** When a pane dies for an instance with `PlanFile`, klique checks
plan state. If all tasks done → spawn reviewer. If not → leave plan as `in_progress` (user
can re-launch from sidebar).

## Review Prompt Template

Lives in scaffold templates at `templates/shared/review-prompt.md`:

```markdown
Review the implementation of plan: {{PLAN_NAME}}

Plan file: {{PLAN_FILE}}

Read the plan file to understand goals, architecture, and tasks. Then review all changes
made during implementation. Load the `requesting-code-review` skill for structured review.

Focus on:
- Does the implementation match the plan's stated goals and architecture?
- Are there any tasks that were implemented incorrectly or incompletely?
- Code quality, error handling, test coverage
- Any regressions or unintended side effects
```

Embedded in the binary alongside agent templates. Rendered with plan-specific values.

## File Changes Summary

| Area | Files | Change |
|------|-------|--------|
| Model | `session/instance.go` | Add `PlanFile` field to Instance |
| Storage | `session/storage.go` | Persist `PlanFile` in session JSON |
| Plan state | `config/plan_state.go` (new) | Read/write/poll plan-state.json |
| Sidebar | `app/ui/sidebar.go` | Add Plans section, render plan entries |
| Input | `app/app_input.go` | Handle plan selection → spawn coder session |
| Orchestration | `app/app_state.go` | Tick-based plan completion detection |
| Reviewer spawn | `app/app_state.go` | Spawn reviewer session on plan completion |
| Scaffold | `internal/initcmd/scaffold/` | Review prompt template |
| Plan state schema | `docs/plans/plan-state.json` | Add `reviewing` status |
