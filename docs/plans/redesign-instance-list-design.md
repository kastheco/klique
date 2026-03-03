# Redesign Instance List — Design

## Problem

The current three-column layout (sidebar | instance list | preview) has redundant information. The sidebar shows plans in a tree; the instance list repeats that context with each row. CPU/memory stats take prominent space but aren't useful at-a-glance. Each instance row consumes 5-7 vertical lines, limiting how many you can see simultaneously.

## Decision

Merge the sidebar and instance list into a single **NavigationPanel** component. The layout becomes two columns: navigation (left) and preview/tabs (right). Plans and a "solo" section are the top-level grouping; instances are compact two-line child rows grouped under their parent plan. The preview pane reclaims the former middle column's width.

## Layout

```
┌────── navigation (30%) ──────┐┌──────── preview/tabs (70%) ────────┐
│ 🔍 search                     ││ ┌─ agent ─┬─ diff ─┬─ info ──┐   │
│                                ││ │                            │   │
│ ▾ my-auth-refactor         ●  ││ │  agent terminal output     │   │
│    ● wave 1 · task 1          ││ │                            │   │
│      ⎇ feat/auth +3-1 · edit ││ │                            │   │
│    ● wave 1 · task 2          ││ │                            │   │
│      ⎇ feat/auth +8-2 · test ││ │                            │   │
│ ▸ api-redesign             ◉  ││ │                            │   │
│ ── solo ──                    ││ │                            │   │
│    ● my-experiment            ││ │                            │   │
│      ⎇ exp/thing +1-0 · read ││ │                            │   │
│ + import from clickup         ││ │                            │   │
│ ── ▸ history ──               ││ │                            │   │
│                                ││ │                            │   │
│ ● running  ◉ review  ○ idle  ││ │                            │   │
│ ┌─ repo-name ▾ ─┐            ││ └────────────────────────────┘   │
└────────────────────────────────┘└────────────────────────────────────┘
```

## Row Types

### Plan header
Collapsible. Shows plan display name + trailing status glyph.
```
▾ my-auth-refactor          ●     (has running instances)
▸ api-redesign              ◉     (has notified instances)
▸ deploy-pipeline           ○     (idle)
```

### Instance row
Two lines, indented under parent plan. Line 1: status icon + title. Line 2: branch + diff stats + activity.
```
   ● wave 1 · task 1
     ⎇ feat/auth +3-1 · editing app.go
```

Status icons:
- `●` (spinner, animated) — running
- `●` (foam, static) — ready / waiting for input
- `◉` (rose, pulsing) — notified (finished, user hasn't looked)
- `✓` (foam, faint) — implementation complete
- `⏸` (muted) — paused
- `○` (muted) — loading

### Solo section header
Divider-style separator for ungrouped instances.
```
── solo ──
```

### Solo instance row
Same two-line format as plan instances, directly under the solo header with no plan parent.

### Import action row
Unchanged from current sidebar.
```
+ import from clickup
```

### History toggle
Unchanged from current sidebar.
```
── ▸ history ──
```

### Cancelled plan
Strikethrough style, unchanged from current sidebar.

## Sorting

### Top-level plan ordering
1. Plans with notified instances (needs attention)
2. Plans with running instances — sorted by most recent activity
3. Plans with no active instances (idle) — sorted by most recent activity
4. Solo section (instances sorted by same priority)
5. Import action (if clickup available)
6. History section
7. Cancelled plans

### Instance ordering within a plan
1. Notified (needs attention)
2. Running
3. Ready
4. Paused
5. Completed

## Selection Behavior

### Plan header selected
- Tabbed window switches to **info tab**
- Info tab shows plan summary: name, status, topic, branch, wave progress, aggregated diff stats, instance count
- Summary includes a "view plan doc" button that opens the full markdown in preview tab document mode

### Instance row selected
- Tabbed window switches to **agent tab** (preview)
- Shows that instance's terminal output
- Diff and info tabs update for that instance
- Info tab now includes CPU/memory (relocated from instance list row)

### Solo section header
Navigation skips it (up/down jumps past) — it's a divider, not selectable.

## Keyboard Navigation

- `↑/↓` — move between selectable rows (plan headers, instances, import, history toggle)
- `←/→` — collapse/expand plan
- `Space` or `Enter` on plan header — toggle expand/collapse
- `Enter` on instance — attach/focus the agent
- `/` — activate search (filters tree by plan names and instance titles)
- `Tab` — cycle focus between navigation panel and preview pane

## Auto Expand/Collapse

- Plans with running instances default to **expanded**
- Plans with no active instances default to **collapsed**
- User overrides tracked in state, persist across rebuilds

## Info Tab Plan Summary

When a plan header is selected, the info tab renders:

```
plan
────────────────────────
name              my-auth-refactor
status            implementing
topic             backend
branch            feat/auth-refactor
created           2026-02-24

wave progress
────────────────────────
wave 1            ✓ 3/3 complete
wave 2            ● 2/3 running
  task 1          ✓ complete
  task 2          ● running
  task 3          ○ pending

stats
────────────────────────
instances         5 (3 running, 1 ready, 1 paused)
lines changed     +142 -38

                 ┌─────────────────┐
                 │  view plan doc  │
                 └─────────────────┘
```

CPU and memory for individual instances are shown in the instance section of the info tab when an instance row is selected.

## What Gets Deleted

### Entire files removed
- `ui/list.go` — `List` type (~528 lines)
- `ui/list_renderer.go` — `InstanceRenderer` and `List.String()` (~447 lines)
- `ui/list_styles.go` — filter tabs, sort mode, list border styles (~156 lines)
- Associated test files: `list_renderer_alignment_test.go`, `list_styles_test.go`, `list_scroll_test.go`, `list_cycle_test.go`, `list_highlight_test.go`

### Removed from sidebar.go
- Flat-mode code path (`items []SidebarItem`, `SidebarAll`, `SidebarUngrouped`, all non-tree rendering)
- Plan stage rows (`rowKindStage`, `planStageRows()`, `stageState()`, stage styles)
- The `Sidebar` type itself — replaced by `NavigationPanel`

### Removed from app/
- `listWidth` calculation and middle column in `View()`
- Status filter tabs (All / Active)
- Sort mode cycling
- All `m.list.*` call sites — rewired to `m.nav.*`

### Moved to info tab
- CPU/memory display
- Resource line rendering

## Approach

Build a new `NavigationPanel` type in `ui/` that replaces both `Sidebar` and `List`. It owns the tree data model (topics → plans → instances, solo section, history) and renders all row kinds. Reuses adapted pieces from the current instance renderer (compact two-line rows, spinner, diff stats, activity indicators) and sidebar (search bar, collapse/expand, scroll, click handling). Old `Sidebar` and `List` types are deleted.

Estimated: ~1200 lines deleted, ~600-800 lines new. Net reduction in UI code.
