# Bubblezone Integration Design

## Context

Klique's mouse handling uses manual coordinate math — column boundary checks (`x < m.sidebarWidth`), hardcoded row offsets (`contentY - 4`), and width-division tab detection (`localX / tabWidth`). This is fragile: any layout change (search bar height, padding, tab width) silently breaks click targets. Bubblezone is already imported and wired up (`zone.NewGlobal()`, `zone.Scan()` in View) but only used for the repo switch button.

Focus mode (`stateFocusAgent`) has no mouse escape path — all clicks are swallowed. Users instinctively click outside the agent pane to exit but nothing happens.

## Goals

1. Replace all coordinate-based mouse hit-testing with bubblezone `Mark`/`Get` zones
2. Enable click-outside-to-exit from focus mode
3. Make mouse clicks "just work" as occasional convenience for keyboard-primary users

## Non-Goals

- Comprehensive hover effects beyond the existing repo button
- Drag interactions
- Making the TUI mouse-primary

## Zone ID Scheme

Static zones (fixed string IDs):

| Zone ID | Element | Component |
|---------|---------|-----------|
| `repo-switch` | Repo button (exists) | `sidebar.go` |
| `sidebar-search` | Search bar area | `sidebar.go` |
| `tab-agent` | Agent tab header | `tabbed_window.go` |
| `tab-diff` | Diff tab header | `tabbed_window.go` |
| `tab-git` | Git tab header | `tabbed_window.go` |
| `tab-content` | Center pane content area | `tabbed_window.go` |
| `list-tab-all` | "All" filter tab | `list.go` |
| `list-tab-active` | "Active" filter tab | `list.go` |

Dynamic zones (prefix + visible index):

| Zone Pattern | Element | Component |
|-------------|---------|-----------|
| `sidebar-row-{N}` | Each visible sidebar tree row | `sidebar.go` |
| `list-item-{N}` | Each visible instance row | `list_renderer.go` |

`{N}` is the visible index (0-based from top of rendered output). View functions maintain a visible-index-to-data-index mapping.

## Focus Mode Escape

When `state == stateFocusAgent`, any left-click outside the `tab-content` zone:

1. Calls `exitFocusMode()`
2. Falls through to normal click handling — sets focus slot, selects clicked item

Clicks inside `tab-content` during focus mode continue to be swallowed (embedded terminal owns that area). Scroll wheel behavior unchanged.

One click, three effects: exit focus + switch pane + select item.

## Mouse Handler Rewrite

Current flow uses column boundary checks and row offset arithmetic. New flow uses zone lookups:

```
handleMouse(msg):
  // Hover tracking (repo button, unchanged)
  // Non-press early return (unchanged)
  // Focus mode escape: if stateFocusAgent && left click outside tab-content → exitFocusMode, fall through
  // Scroll wheel: zone.Get("tab-content").InBounds → scroll content
  // Overlay dismissal (unchanged)
  // Zone dispatch:
  //   repo-switch → open picker
  //   sidebar-search → activate search
  //   sidebar-row-{N} → select sidebar item, set focus to sidebar
  //   tab-{agent,diff,git} → switch tab, set focus slot
  //   tab-content → set focus to active center tab
  //   list-tab-{all,active} → set filter
  //   list-item-{N} → select instance, set focus to list
```

**Deleted:** `HandleTabClick` on TabbedWindow and List, all `contentY - 4` offsets, `localX / tabWidth` division, column boundary `if/else` cascade.

**Kept:** Layout dimension fields (for component sizing), right-click `x,y` (for menu placement), overlay dismissal logic.

## View-Side Zone Marking

Each component wraps clickable elements with `zone.Mark()` in its render method, after all lipgloss styling is applied.

- **sidebar.go**: Mark search bar, each tree row, repo button (exists)
- **tabbed_window.go**: Mark each tab header, content area
- **list.go / list_renderer.go**: Mark filter tabs, each instance row

Components expose a visible-index-to-data-index mapping so the mouse handler can translate zone hits to data operations.

## Files Touched

- `app/app_input.go` — rewrite `handleMouse`, delete coordinate math
- `ui/sidebar.go` — add zone marks, expose row index mapping
- `ui/tabbed_window.go` — add zone marks, delete `HandleTabClick`
- `ui/list.go` — add zone marks, delete `HandleTabClick`, expose item index mapping
- `ui/list_renderer.go` — add zone marks to rendered items
- `ui/consts.go` — zone ID constants

## Unchanged

- Scroll wheel behavior
- Right-click context menu logic (uses x,y for placement, not hit-testing)
- Keyboard input handling
- Hover tracking (repo button only)
- Overlay dismissal patterns
