# Plan-Centric Sidebar Design

## Problem

The klique sidebar currently has two parallel grouping systems: **plans** (from plan-state.json, tracking implementation lifecycle) and **topics** (from session storage, grouping instances with optional shared worktrees). These overlap conceptually — both represent "a unit of work" — but they're independent, leading to:

- Cognitive overhead: users must decide whether something is a "topic" or a "plan"
- Feature branches from topics don't connect to plans
- Plans show as flat list items with no lifecycle visibility
- No clear workflow for: create idea → plan it → implement → review → ship

## Approach

Merge plans and topics into a single concept: **plans are the organizational unit**. A plan represents one complete unit of work, from ideation through merge. The sidebar becomes a tree view where each plan expands to show its lifecycle stages.

Topics are removed entirely. The plan inherits the useful topic features (instance grouping, shared worktree, sidebar filtering). Instances that aren't part of any plan show in "ungrouped".

## Sidebar Layout

```
 search
╭──────────────────────────╮
│ ▸ all                (5) │
│                          │
│ ── plans ──              │
│ ▾ my-feature             │  ← expanded, selected
│     ✓ plan               │  ← done (plan doc committed)
│     ● implement          │  ← active (coder running)
│     ○ review             │  ← locked (greyed out)
│     ○ finished           │  ← locked (greyed out)
│ ▸ auth-refactor      (2) │  ← collapsed, 2 instances
│ ▸ fix-perf               │  ← collapsed
│                          │
│ ── ungrouped ──          │
│   ungrouped          (1) │
│                          │
│ ── plan history ──       │
│ ▸ wizard-ux              │  ← greyed out
│ ▸ init-scaffold          │
│                          │
│ ╭────────────────────╮   │
│ │ klique           ▾ │   │
│ ╰────────────────────╯   │
╰──────────────────────────╯
```

### Tree Navigation

- `Space` on plan header: expand/collapse
- `Enter` on plan header: context menu
- `Enter` on sub-item: trigger that action (if unlocked)
- Arrow keys navigate into/out of children naturally
- Plan headers show instance count in parentheses when they have associated instances

### Progressive Unlock

Sub-items unlock sequentially — each stage requires the previous to be complete:

| Sub-item | Available when | Glyph states |
|----------|---------------|--------------|
| plan | always | `○` available, `●` active, `✓` done |
| implement | plan is done (status >= `implementing`) | `○` locked → `○` available → `●` active → `✓` done |
| review | implementation is done (status >= `reviewing`) | same progression |
| finished | review is done (status = `reviewed` or `finished`) | same |

**Visual treatment:**
- `✓` done — `ColorMuted`, non-interactive
- `●` active — `ColorFoam` (teal), interactive
- `○` available (next action) — `ColorText` (normal), interactive
- `○` locked — `ColorOverlay` (very dim), shows toast "complete {previous stage} first" on Enter

### Plan History

Plans with `finished` status appear in a collapsed "plan history" section at the bottom of the sidebar. They render in `ColorMuted` (greyed out) and expand to show the same sub-items (all `✓`). This provides a browsable log of completed work.

## Plan State Model

### Schema

```json
{
  "2026-02-21-my-feature.md": {
    "status": "ready",
    "description": "refactor auth to use JWT tokens",
    "branch": "plan/my-feature",
    "created_at": "2026-02-21T14:30:00Z"
  }
}
```

New fields vs current schema:
- `description` — captured at creation, seeds the planner agent prompt
- `branch` — feature branch name, created when plan is registered
- `created_at` — for sorting in plan history

### Status Lifecycle

```
ready → planning → implementing → reviewing → finished
```

| Status | Meaning |
|--------|---------|
| `ready` | just created, no planner spawned yet |
| `planning` | planner agent is running |
| `implementing` | plan doc exists, coder is running or waiting |
| `reviewing` | reviewer agent is running |
| `finished` | terminal state, moves to plan history |

### Design Docs

Design docs (`*-design.md`) are companion files, not sidebar entries. They follow the naming convention `{date}-{name}-design.md` pairing with `{date}-{name}.md`. Only the implementation plan gets registered in plan-state.json.

- `v` keybind shows the implementation plan
- Context menu includes "view design doc" for the companion file
- Planner agent produces both files but only registers the implementation plan

## Keybind Changes

| Key | Old | New |
|-----|-----|-----|
| `p` | push branch | new plan (prompt for name + description) |
| `m` | move to topic | assign instance to plan |
| `Space` | context menu | expand/collapse plan header |
| `Enter` | open/attach | context menu on plan header; trigger action on sub-item |
| `T` | new topic | **removed** |
| `X` | kill all in topic | **removed** (moves to plan context menu) |
| `P` | create PR | **unchanged** |
| `s` | focus sidebar | **unchanged** |
| `v` | view plan | **unchanged** |

### Plan Context Menu (Enter on plan header)

- modify plan — spawns planner agent to refine, resets to `planning` state
- start over — clears implementation work, resets to `planning`, cleans up worktree
- kill running instances
- push branch
- create PR
- rename plan
- delete plan
- view design doc

## Instance & Session Model

### Instance Changes

- `TopicName` field removed — instances group by `PlanFile` instead
- New `AgentType string` field: `"planner"`, `"coder"`, `"reviewer"`, or `""` (ad-hoc)
- `IsReviewer bool` removed (redundant with `AgentType == "reviewer"`)
- `--agent {type}` flag appended to program command at launch

### Session Spawning

| Sub-item | Agent flag | Branch | Prompt |
|----------|-----------|--------|--------|
| plan | `--agent planner` | main (no worktree) | "Plan {name}. Goal: {description}." |
| implement | `--agent coder` | plan's feature branch (shared worktree) | "Implement docs/plans/{file}..." |
| review | `--agent reviewer` | plan's feature branch | review prompt from scaffold |
| modify plan | `--agent planner` | main | "Modify existing plan at docs/plans/{file}..." |

### Branch Policy

- Planner agent works on main — plan files commit directly to main branch so they can be picked up from anywhere
- Implementation and review happen on the plan's feature branch (`plan/{name}`)
- The feature branch + shared worktree are created by klique when "implement" is first triggered
- End-of-implementation: klique detects coder finished, shows "push branch?" confirmation

## Topic System Removal

The entire topic system is removed:

- `session/topic.go`, `session/topic_storage.go` — deleted
- `Topic` struct, `TopicData`, `TopicOptions`, `NewTopic`, `FromTopicData` — deleted
- `Storage.LoadTopics()`, `Storage.SaveTopics()` — deleted
- `allTopics`, `topics` fields on `home` struct — deleted
- `stateNewTopic`, `stateNewTopicConfirm`, `stateRenameTopic` states — deleted
- `filterTopicsByRepo`, `saveAllTopics`, `getMovableTopicNames` — deleted
- Shared worktree concept migrates to plans — plan-state.json `branch` field tracks it

## Agent Configuration

The planner agent prompt (`.opencode/agents/planner.md`) gains:

```
## Branch Policy
Always commit plan files to the main branch. Do NOT create feature branches
for planning work. The feature branch for implementation is created by klique
when the user triggers "implement".

Only register implementation plans in plan-state.json — never register
design docs (*-design.md) as separate entries.
```

## Implementation Strategy

Three phased plans, each independently implementable:

1. **Plan-Centric Sidebar** — remove topics, refactor sidebar to tree view, keybind changes, plan-state schema evolution
2. **Plan Lifecycle & Agent Sessions** — creation flow, `AgentType` field, `--agent` flag, sub-item action handlers, push prompt
3. **Instance Grouping & Filtering** — `PlanFile` replaces `TopicName` for grouping, sidebar filtering, search, context menu adaptation
