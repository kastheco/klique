# Tree-Mode Sidebar + Highlight-Boost Instance List

## Problem

The sidebar has two rendering systems: a flat `s.items` list rendered by `String()`, and a tree `s.rows` structure navigated by `Up()/Down()/GetSelectedID()`. Tree mode is disabled by a `DisableTreeMode()` call because `String()` only renders `s.items`, causing a visual/logical mismatch when tree mode is active.

The instance list currently shows all instances regardless of sidebar selection (topic filter is dead code). There's no visual connection between sidebar selection and the instance list.

## Design

### Sidebar Tree Rendering

`String()` gets a tree-mode branch that renders `s.rows` when `useTreeMode == true`. The flat `s.items` path stays as fallback.

```
bubblezone                   ○    ← ungrouped plan, ready
center-col-v-align           ●    ← ungrouped plan, implementing
auth-refactor              ▾ ●    ← topic, expanded, has running child
  ├ api-tokens               ●    ← plan under topic
  └ session-mgmt             ○
deployment                 ▸      ← topic, collapsed
── History ──                     ← toggle row
  old-plan                   ✓
```

Row kinds and visual treatment:

| Kind | Indent | Content |
|------|--------|---------|
| `rowKindTopic` | 0 | label + chevron (▸/▾) + aggregate status dot |
| `rowKindPlan` | 2 (under topic) or 0 (ungrouped) | label + status glyph (○/●/◉/✕) + chevron if expandable |
| `rowKindStage` | +2 from parent plan | ✓/▸/🔒 + stage label — display-only, not actionable |
| `rowKindHistoryToggle` | 0 | "── History ──" section divider |
| `rowKindCancelled` | 0 | strikethrough label |

### Tree Navigation

| Key | On Topic | On Plan | On Stage |
|-----|----------|---------|----------|
| **Space** | Toggle expand/collapse | Toggle expand/collapse | No-op |
| **Enter** | Open topic context menu | Open plan context menu | No-op |
| **→** | Expand if collapsed, else first child | Expand if collapsed, else first child stage | No-op |
| **←** | Collapse if expanded, else up | Collapse if expanded, else parent topic (or up if ungrouped) | Move to parent plan |
| **↑/↓** | Prev/next visible row | Same | Same |

New `Left()` and `Right()` methods on Sidebar. Stage rows are visual-only — Enter and Space on stages are no-ops.

### Instance List Highlight + Boost

When sidebar selection changes, the list gets a `highlightFilter` (topic name, plan filename, or empty string).

On rebuild:
1. Partition instances into **matched** (PlanFile or topic matches sidebar selection) and **unmatched**
2. Sort each partition independently using the current sort mode
3. Concat: matched first, then unmatched
4. Pass `Highlighted bool` flag per-item to the renderer

Rendering: matched items get normal styling, unmatched items get dimmed foreground. When no filter is active, everything renders normally.

### Wiring

- Remove `DisableTreeMode()` call in `updateSidebarPlans()`
- Replace `filterInstancesByTopic()` calls with `SetHighlightFilter()` based on tree selection
- Context menus work from `GetSelectedPlanFile()` / `GetSelectedTopicName()` which already branch on tree mode

## Parallelization

Four implementation plans, A-C parallel, D sequential after all complete:

| Plan | Scope | Deps |
|------|-------|------|
| **1a-tree-renderer** | `String()` tree-mode rendering branch | None |
| **1b-tree-navigation** | `Left()`/`Right()` + Space/Enter keybinds | None |
| **1c-highlight-boost-list** | `SetHighlightFilter()`, partition sort, accent style | None |
| **2-tree-mode-wiring** | Remove DisableTreeMode, wire highlight filter, integration | 1a, 1b, 1c |
