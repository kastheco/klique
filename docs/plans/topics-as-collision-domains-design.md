# Topics as Collision Domains — Design

## Problem

The plan-centric sidebar design (2026-02-21) removes topics entirely, making plans the sole organizational unit. But topics serve a purpose that plans don't cover: they represent **workspace collision domains** — groupings of plans that touch the same code area and shouldn't run concurrently.

Without topics, there's no way to express "these three plans all touch the sidebar code — don't run their coders at the same time." The result is agents committing to wrong branches, state drifting silently, and garbled git history that's painful to untangle.

## Conceptual Model

**Plan** = a unit of work with a lifecycle (`ready → planning → implementing → reviewing → finished`). Owns its own feature branch and worktree.

**Topic** = a collision domain. A named group of plans that touch the same workspace area. Topics have no git state, no worktree, no branch. They're a label with concurrency enforcement.

### Rules

- A plan belongs to zero or one topics.
- An ungrouped plan is a topic-of-one — same worktree/branch mechanics, just no collision warnings since there's nothing to collide against.
- Topics are created implicitly during plan creation (optional prompt — typing a new name creates the topic on the fly).
- Running two coders concurrently within the same topic triggers a confirmation warning with override.

## Sidebar Layout

Two-level tree. Topics and ungrouped plans sit at the top level under "plans." Topics expand to show their plans. Plans expand to show lifecycle stages.

```
 search
╭──────────────────────────╮
│ ▸ all                (5) │
│                          │
│ ── plans ──              │
│ ▾ ui-refactor            │  ← topic (expanded)
│     ▸ sidebar-redesign   │  ← plan (collapsed)
│     ▾ menu-overhaul      │  ← plan (expanded)
│         ✓ plan           │
│         ● implement      │  ← active stage
│         ○ review         │
│         ○ finished       │
│ ▸ auth-system        (1) │  ← topic (collapsed)
│ ▾ quick-bugfix           │  ← ungrouped plan (top-level)
│     ✓ plan               │
│     ○ implement          │
│     ○ review             │
│     ○ finished           │
│                          │
│ ── plan history ──       │
│ ▸ old-feature            │
│                          │
│ ╭────────────────────╮   │
│ │ klique           ▾ │   │
│ ╰────────────────────╯   │
╰──────────────────────────╯
```

### Navigation

- `Space` on topic header: expand/collapse (shows/hides its plans)
- `Space` on plan header: expand/collapse (shows/hides lifecycle stages)
- `Enter` on topic header: topic context menu (rename, delete, etc.)
- `Enter` on plan header: plan context menu (unchanged from plan-centric design)
- `Enter` on lifecycle stage: trigger that action (if unlocked)

### Visual Treatment

- Topic headers use a distinct prefix glyph (e.g., `◆`) and `ColorSubtle` foreground to differentiate from plan headers' `○/●/✓`.
- Instance counts on topics aggregate across all their plans.
- Concurrency indicator: when multiple coders are running within a topic, the topic header shows a warning glyph (`⚠`) in `ColorGold` or `ColorRose`.

## Topic Data Model

Topics become radically simpler than the current `session/topic.go`. No git state, no worktree, no `Setup()`/`Cleanup()`. Just a name stored in plan-state.json.

### Schema

```json
{
  "topics": {
    "ui-refactor": {
      "created_at": "2026-02-21T14:30:00Z"
    }
  },
  "plans": {
    "2026-02-21-sidebar-redesign.md": {
      "status": "implementing",
      "description": "convert sidebar to tree view",
      "branch": "plan/sidebar-redesign",
      "topic": "ui-refactor",
      "created_at": "2026-02-21T14:30:00Z"
    },
    "2026-02-21-quick-bugfix.md": {
      "status": "ready",
      "description": "fix off-by-one in pagination",
      "branch": "plan/quick-bugfix",
      "topic": "",
      "created_at": "2026-02-21T15:00:00Z"
    }
  }
}
```

### What Changes from Plan-Centric Design

- `PlanEntry` gains a `topic` field (string, empty = ungrouped).
- Topics stored in plan-state.json alongside plans — just a name + `created_at`. No separate storage system.
- `session/topic.go` and `session/topic_storage.go` still deleted — the old topic system with worktrees, branches, `TopicData`, `FromTopicData` is gone.
- `TopicName` on `Instance` still removed — instances group by `PlanFile`, and plans know their topic.

### What Stays the Same

- Plans own their own branch/worktree (unchanged).
- Plan lifecycle statuses (unchanged).
- Instance `PlanFile` field for grouping (unchanged).

## Plan Creation Flow

Three-step prompt when pressing `p`:

1. **"Plan name:"** → `sidebar-redesign`
2. **"Description:"** → `convert sidebar to tree view`
3. **"Topic (optional):"** → picker showing existing topics + free-text for new ones. Enter on empty = ungrouped.

```
╭─ Topic (optional) ──────────╮
│                              │
│   ui-refactor                │
│   auth-system                │
│                              │
│   Type a new name or ↵ skip  │
╰──────────────────────────────╯
```

- Selecting an existing topic assigns the plan to it.
- Typing a name that doesn't exist creates the topic and assigns.
- Pressing Enter with empty input = ungrouped.

The `T` keybind for explicit topic creation is removed — topics only come into existence through this flow.

## Concurrency Gate

When triggering "implement" on a plan that belongs to a topic, klique checks if any other plan in the same topic already has a running coder instance.

**If conflict detected:**

```
╭─────────────────────────────────────────────╮
│                                             │
│  ⚠ sidebar-redesign is already running      │
│    in topic "ui-refactor"                   │
│                                             │
│  Running both plans may cause issues.       │
│  Continue anyway?                           │
│                                             │
│           [y] yes    [n] no                 │
╰─────────────────────────────────────────────╯
```

**If no conflict:** Proceeds normally.

**Ungrouped plans:** Never trigger this check.

**Scope:** Only gates coder sessions (`implement` stage). Planner and reviewer agents don't write to the feature branch concurrently.

## Impact on Existing Implementation Plans

**Plan 1 (plan-centric-sidebar):** Most changes. Task 2 "Remove Topic System" changes scope — delete the old topic system but reimplement topics as lightweight entries in plan-state.json. Task 3 sidebar tree needs a third nesting level (topic → plan → stages). Task 4 plan creation adds the third "topic" prompt. Recommend rewriting Plan 1 from scratch.

**Plan 2 (plan-lifecycle-sessions):** Minor addition. Session spawning for "implement" gains the concurrency gate check. Otherwise unchanged.

**Plan 3 (instance-grouping):** Minor addition. Sidebar filtering now has three levels: topic → plan → instances. Filtering by topic shows all instances across all its plans.
