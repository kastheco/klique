# Rewrite UI Panels Implementation Plan

**Goal:** Clean-room rewrite all upstream-derived code in the UI panel layer (diff.go, preview.go, menu.go, tabbed_window.go) to remove AGPL-tainted lines. These are the main content panels of the TUI: diff viewer, terminal preview, bottom menu bar, and tabbed window container.

**Architecture:** Four files rewritten in-place. Each panel is a bubbletea component with Init/Update/View methods. The panels depend on the overlay system (rewritten in plan 04) for modal rendering. `ui/consts.go` has minimal upstream (10 lines at fork, now 61 lines — essentially all original) and is excluded. Existing tests (preview_test.go, preview_fallback_test.go, menu_test.go, tabbed_window_test.go) serve as the regression suite.

**Tech Stack:** Go 1.24, bubbletea v1.3.x, lipgloss v1.1.x, charmbracelet/x/ansi, testify

**Size:** Medium (estimated ~3 hours, 4 tasks, 2 waves)

---

## Wave 1: Independent Panels

### Task 1: Rewrite diff.go — Diff Viewer Panel

**Files:**
- Modify: `ui/diff.go`
- Test: `ui/main_test.go` (existing, exercises diff rendering)

**Step 1: write the failing test**

Existing tests exercise diff rendering through the main UI test harness. No new tests needed.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `ui/diff.go` from scratch:

- `DiffView` struct — viewport model, content string, width/height, styles
- `NewDiffView()` — constructor with default styles
- `SetContent(diff string)` — parse unified diff, apply syntax highlighting (green for additions, red for deletions, cyan for headers)
- `SetSize(width, height)` — resize viewport
- `Update(msg)` — handle scroll, viewport messages
- `View()` — render viewport with styled diff content
- Line-level diff coloring: `+` lines green, `-` lines red, `@@` headers cyan, file headers bold

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/diff.go
git commit -m "feat(clean-room): rewrite ui/diff.go from scratch"
```

### Task 2: Rewrite menu.go — Bottom Menu Bar

**Files:**
- Modify: `ui/menu.go`
- Test: `ui/menu_test.go` (existing)

**Step 1: write the failing test**

Existing `menu_test.go` covers menu rendering, key hint display, and context-sensitive items.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "TestMenu" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `ui/menu.go` from scratch:

- `MenuItem` struct — key string, label string, enabled bool
- `Menu` struct — items list, width, style configuration
- `NewMenu()` — constructor
- `SetItems(items)` — update menu items
- `SetWidth(width)` — resize
- `View()` — render horizontal bar of key hints: `key label` pairs separated by dots, truncated to fit width
- Context-sensitive rendering: different items shown based on app state (instance selected, overlay active, etc.)

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run "TestMenu" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/menu.go
git commit -m "feat(clean-room): rewrite ui/menu.go from scratch"
```

## Wave 2: Container Panels

> **depends on wave 1:** tabbed_window.go contains the diff viewer tab. preview.go uses menu items for context. Both need the wave 1 components to be stable.

### Task 3: Rewrite tabbed_window.go — Tabbed Content Container

**Files:**
- Modify: `ui/tabbed_window.go`
- Test: `ui/tabbed_window_test.go` (existing)

**Step 1: write the failing test**

Existing `tabbed_window_test.go` covers tab switching, content rendering, and resize behavior.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "TestTabbedWindow" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `ui/tabbed_window.go` from scratch:

- `Tab` struct — title string, content tea.Model, active bool
- `TabbedWindow` struct — tabs slice, active tab index, width/height, tab bar style
- `NewTabbedWindow(tabs)` — constructor
- `SetActiveTab(index)` — switch active tab
- `SetSize(width, height)` — resize all tabs
- `Update(msg)` — delegate to active tab, handle tab switch keys (Shift+1/2/3)
- `View()` — render tab bar (styled tab titles with active indicator) + active tab content below
- Tab bar rendering: active tab highlighted, inactive tabs dimmed, separator line

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run "TestTabbedWindow" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/tabbed_window.go
git commit -m "feat(clean-room): rewrite ui/tabbed_window.go from scratch"
```

### Task 4: Rewrite preview.go — Terminal Preview Panel

**Files:**
- Modify: `ui/preview.go`
- Test: `ui/preview_test.go`, `ui/preview_fallback_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover preview rendering, fallback mode, resize, and content display.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/... -run "TestPreview" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `ui/preview.go` from scratch:

- `Preview` struct — viewport, content, width/height, embedded terminal reference, fallback mode flag, styles
- `NewPreview()` — constructor
- `SetContent(content string)` — update preview content (from tmux capture-pane or embedded terminal)
- `SetSize(width, height)` — resize viewport and embedded terminal
- `SetEmbeddedTerminal(term)` — switch to embedded terminal mode
- `Update(msg)` — handle scroll, terminal tick, viewport messages
- `View()` — render content: if embedded terminal, use its Render(); otherwise use viewport with captured content
- Fallback rendering for when no instance is selected (empty state message)

**Step 4: run test to verify it passes**

```bash
go test ./ui/... -run "TestPreview" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/preview.go
git commit -m "feat(clean-room): rewrite ui/preview.go from scratch"
```
