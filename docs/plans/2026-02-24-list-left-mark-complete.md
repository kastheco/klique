# Instance List Left + Mark Task Complete Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move the instance list column from the right side to the middle (between plans sidebar and agent tabs), update all directional navigation and mouse hit-testing accordingly, and add a "Mark complete" context menu action for wave task instances.

**Architecture:** Three surgical edits — (1) flip the `JoinHorizontal` render order in `View()`, (2) update the four directional nav cases and two mouse column boundaries in `app_input.go`, (3) add `IsTaskRunning()` to `WaveOrchestrator` and wire a new context menu action through `openContextMenu`, `handleRightClick`, and `executeContextAction`. No new files needed.

**Tech Stack:** Go, bubbletea, lipgloss, existing `WaveOrchestrator` / `overlay.ContextMenuItem` types.

---

### Task 1: Add `IsTaskRunning` to `WaveOrchestrator`

**Files:**
- Modify: `app/wave_orchestrator.go`
- Test: `app/wave_orchestrator_test.go`

**Step 1: Write the failing test**

Add to `app/wave_orchestrator_test.go`:

```go
func TestIsTaskRunning(t *testing.T) {
    plan := &planparser.Plan{
        Waves: []planparser.Wave{
            {Number: 1, Tasks: []planparser.Task{{Number: 1}, {Number: 2}}},
        },
    }
    orch := NewWaveOrchestrator("test.md", plan)
    orch.StartNextWave()

    assert.True(t, orch.IsTaskRunning(1), "task 1 should be running after StartNextWave")
    assert.True(t, orch.IsTaskRunning(2), "task 2 should be running after StartNextWave")

    orch.MarkTaskComplete(1)
    assert.False(t, orch.IsTaskRunning(1), "task 1 should not be running after MarkTaskComplete")
    assert.True(t, orch.IsTaskRunning(2), "task 2 should still be running")

    assert.False(t, orch.IsTaskRunning(99), "unknown task should return false")
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./app/ -run TestIsTaskRunning -v
```
Expected: FAIL — `orch.IsTaskRunning undefined`

**Step 3: Add the method to `wave_orchestrator.go`**

Add after `FailedTaskCount()` (around line 183):

```go
// IsTaskRunning returns true if the given task number is currently in the running state.
// Used to gate the "Mark complete" context menu action.
func (o *WaveOrchestrator) IsTaskRunning(taskNumber int) bool {
    return o.taskStates[taskNumber] == taskRunning
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./app/ -run TestIsTaskRunning -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add app/wave_orchestrator.go app/wave_orchestrator_test.go
git commit -m "feat: add IsTaskRunning to WaveOrchestrator"
```

---

### Task 2: Wire "Mark complete" into context menu build helpers

**Files:**
- Modify: `app/app_actions.go` — `openContextMenu()` and `handleRightClick()` in `app_input.go`

The condition for showing the item:
```go
selected.TaskNumber > 0 &&
m.waveOrchestrators[selected.PlanFile] != nil &&
m.waveOrchestrators[selected.PlanFile].IsTaskRunning(selected.TaskNumber)
```

**Step 1: Add the item in `openContextMenu()` in `app_actions.go`**

In `openContextMenu()`, after the existing items are appended and before `m.contextMenu = overlay.NewContextMenu(...)`, add:

```go
// Wave task: offer manual completion
if selected.TaskNumber > 0 {
    if orch, ok := m.waveOrchestrators[selected.PlanFile]; ok && orch.IsTaskRunning(selected.TaskNumber) {
        items = append(items, overlay.ContextMenuItem{Label: "Mark complete", Action: "mark_task_complete"})
    }
}
```

**Step 2: Add the item in `handleRightClick()` in `app_input.go`**

In `handleRightClick()`, in the instance list branch, after the existing items are appended and before `m.contextMenu = overlay.NewContextMenu(x, y, items)`, add the same block:

```go
// Wave task: offer manual completion
if selected.TaskNumber > 0 {
    if orch, ok := m.waveOrchestrators[selected.PlanFile]; ok && orch.IsTaskRunning(selected.TaskNumber) {
        items = append(items, overlay.ContextMenuItem{Label: "Mark complete", Action: "mark_task_complete"})
    }
}
```

**Step 3: Handle the action in `executeContextAction()` in `app_actions.go`**

Add a new case in the switch:

```go
case "mark_task_complete":
    selected := m.list.GetSelectedInstance()
    if selected == nil || selected.TaskNumber == 0 {
        return m, nil
    }
    orch, ok := m.waveOrchestrators[selected.PlanFile]
    if !ok {
        return m, nil
    }
    orch.MarkTaskComplete(selected.TaskNumber)
    m.toastManager.Success(fmt.Sprintf("Task %d marked complete", selected.TaskNumber))
    return m, m.toastTickCmd()
```

**Step 4: Build to verify no compile errors**

```bash
go build ./...
```
Expected: no errors

**Step 5: Commit**

```bash
git add app/app_actions.go app/app_input.go
git commit -m "feat: add Mark complete context menu action for wave task instances"
```

---

### Task 3: Move instance list to middle column — View()

**Files:**
- Modify: `app/app.go` — `View()` function

**Step 1: Update `View()` render order**

Current (line 909-914):
```go
// Layout: sidebar | preview (center/main) | instance list (right)
var listAndPreview string
if m.sidebarHidden {
    listAndPreview = lipgloss.JoinHorizontal(lipgloss.Top, previewWithPadding, listWithPadding)
} else {
    listAndPreview = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, previewWithPadding, listWithPadding)
}
```

Replace with:
```go
// Layout: sidebar | instance list (middle) | preview/tabs (right)
var listAndPreview string
if m.sidebarHidden {
    listAndPreview = lipgloss.JoinHorizontal(lipgloss.Top, listWithPadding, previewWithPadding)
} else {
    listAndPreview = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, listWithPadding, previewWithPadding)
}
```

**Step 2: Build and visually verify**

```bash
go build ./... && ./kasmos
```
Expected: instance list now appears between the plans sidebar and the agent tabs.

**Step 3: Commit**

```bash
git add app/app.go
git commit -m "feat: move instance list to middle column"
```

---

### Task 4: Update mouse hit-testing for new column order

**Files:**
- Modify: `app/app_input.go` — `handleMouse()` and `handleRightClick()`

The current column boundaries are:
- `x < sidebarWidth` → sidebar
- `x < sidebarWidth + tabsWidth` → tabs (center)
- else → list (right)

New boundaries (list is now middle, tabs are right):
- `x < sidebarWidth` → sidebar
- `x < sidebarWidth + listWidth` → list (middle)
- else → tabs (right)

**Step 1: Update `handleMouse()` left-click column detection**

Current (lines 131-160):
```go
} else if x < m.sidebarWidth+m.tabsWidth {
    // Click in preview/diff area (center column): focus whichever center tab is visible
    m.setFocusSlot(slotAgent + m.tabbedWindow.GetActiveTab())
    localX := x - m.sidebarWidth
    if m.tabbedWindow.HandleTabClick(localX, contentY) {
        m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
        return m, m.instanceChanged()
    }
} else {
    // Click in instance list (right column)
    m.setFocusSlot(slotList)

    localX := x - m.sidebarWidth - m.tabsWidth
    // Check if clicking on filter tabs
    if filter, ok := m.list.HandleTabClick(localX, contentY); ok {
        m.list.SetStatusFilter(filter)
        return m, m.instanceChanged()
    }

    // Instance list items start after the header (blank lines + tabs + blank lines)
    listY := contentY - 4
    if listY >= 0 {
        itemIdx := m.list.GetItemAtRow(listY)
        if itemIdx >= 0 {
            m.tabbedWindow.ClearDocumentMode()
            m.list.SetSelectedInstance(itemIdx)
            return m, m.instanceChanged()
        }
    }
}
```

Replace with:
```go
} else if x < m.sidebarWidth+m.listWidth {
    // Click in instance list (middle column)
    m.setFocusSlot(slotList)

    localX := x - m.sidebarWidth
    // Check if clicking on filter tabs
    if filter, ok := m.list.HandleTabClick(localX, contentY); ok {
        m.list.SetStatusFilter(filter)
        return m, m.instanceChanged()
    }

    // Instance list items start after the header (blank lines + tabs + blank lines)
    listY := contentY - 4
    if listY >= 0 {
        itemIdx := m.list.GetItemAtRow(listY)
        if itemIdx >= 0 {
            m.tabbedWindow.ClearDocumentMode()
            m.list.SetSelectedInstance(itemIdx)
            return m, m.instanceChanged()
        }
    }
} else {
    // Click in preview/diff area (right column): focus whichever center tab is visible
    m.setFocusSlot(slotAgent + m.tabbedWindow.GetActiveTab())
    localX := x - m.sidebarWidth - m.listWidth
    if m.tabbedWindow.HandleTabClick(localX, contentY) {
        m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
        return m, m.instanceChanged()
    }
}
```

**Step 2: Update `handleRightClick()` column detection**

Current (lines 183-216):
```go
} else if x >= m.sidebarWidth+m.tabsWidth {
    // Right-click in instance list (right column) — select the item first
    ...
    m.contextMenu = overlay.NewContextMenu(x, y, items)
    ...
}
```

The condition `x >= m.sidebarWidth+m.tabsWidth` (list was rightmost) becomes `x >= m.sidebarWidth && x < m.sidebarWidth+m.listWidth` (list is now middle). Also update the `else if` to match the new boundary:

Replace:
```go
} else if x >= m.sidebarWidth+m.tabsWidth {
```
With:
```go
} else if x >= m.sidebarWidth && x < m.sidebarWidth+m.listWidth {
```

**Step 3: Build and test mouse clicks**

```bash
go build ./... && ./kasmos
```
Expected: clicking the middle column selects instances, clicking the right column focuses the agent/diff/git tabs.

**Step 4: Commit**

```bash
git add app/app_input.go
git commit -m "fix: update mouse hit-testing for new list-middle column layout"
```

---

### Task 5: Update directional arrow key navigation

**Files:**
- Modify: `app/app_input.go` — `KeyArrowLeft` and `KeyArrowRight` cases

**Current nav (old layout: sidebar | tabs | list):**
- `←` from agent/diff → sidebar
- `←` from list → agent
- `←` from git (not running) → sidebar
- `→` from sidebar → agent
- `→` from agent/diff → list
- `→` from git (not running) → list

**New nav (new layout: sidebar | list | tabs):**
- `←` from list → sidebar
- `←` from agent/diff → list
- `←` from git (not running) → list
- `→` from sidebar → list
- `→` from list → agent
- `→` from agent/diff → (no-op, already rightmost — keep as-is or wrap to sidebar)
- `→` from git (not running) → (no-op or wrap)

**Step 1: Update `KeyArrowLeft` case**

Current:
```go
case keys.KeyArrowLeft:
    switch m.focusSlot {
    case slotGit:
        gitPane := m.tabbedWindow.GetGitPane()
        if gitPane != nil && gitPane.IsRunning() {
            _ = gitPane.SendKey(keyToBytes(msg))
            return m, nil
        }
        // Not running — fall through to panel navigation
        m.setFocusSlot(slotSidebar)
    case slotAgent, slotDiff:
        m.setFocusSlot(slotSidebar)
    case slotList:
        m.setFocusSlot(slotAgent)
    }
    return m, nil
```

Replace with:
```go
case keys.KeyArrowLeft:
    switch m.focusSlot {
    case slotGit:
        gitPane := m.tabbedWindow.GetGitPane()
        if gitPane != nil && gitPane.IsRunning() {
            _ = gitPane.SendKey(keyToBytes(msg))
            return m, nil
        }
        // Not running — fall through to panel navigation
        m.setFocusSlot(slotList)
    case slotAgent, slotDiff:
        m.setFocusSlot(slotList)
    case slotList:
        m.setFocusSlot(slotSidebar)
    }
    return m, nil
```

**Step 2: Update `KeyArrowRight` case**

Current:
```go
case keys.KeyArrowRight:
    switch m.focusSlot {
    case slotGit:
        gitPane := m.tabbedWindow.GetGitPane()
        if gitPane != nil && gitPane.IsRunning() {
            _ = gitPane.SendKey(keyToBytes(msg))
            return m, nil
        }
        // Not running — fall through to panel navigation
        m.setFocusSlot(slotList)
    case slotSidebar:
        m.setFocusSlot(slotAgent)
    case slotAgent, slotDiff:
        m.setFocusSlot(slotList)
    }
    return m, nil
```

Replace with:
```go
case keys.KeyArrowRight:
    switch m.focusSlot {
    case slotGit:
        gitPane := m.tabbedWindow.GetGitPane()
        if gitPane != nil && gitPane.IsRunning() {
            _ = gitPane.SendKey(keyToBytes(msg))
            return m, nil
        }
        // Not running — no-op (already rightmost)
    case slotSidebar:
        m.setFocusSlot(slotList)
    case slotList:
        m.setFocusSlot(slotAgent)
    case slotAgent, slotDiff:
        // Already rightmost — no-op
    }
    return m, nil
```

**Step 3: Build and verify nav**

```bash
go build ./... && ./kasmos
```
Expected: `←/h` from agent tab moves to instance list; `→/l` from sidebar moves to instance list; `→/l` from instance list moves to agent tab.

**Step 4: Commit**

```bash
git add app/app_input.go
git commit -m "fix: update arrow key navigation for new list-middle column layout"
```

---

### Task 6: Update context menu x-position for instance list

**Files:**
- Modify: `app/app_actions.go` — `openContextMenu()`

The Space-key context menu for the instance list positions itself at `x := m.sidebarWidth + m.tabsWidth` (left edge of the old right column). With the list now in the middle, the left edge is `m.sidebarWidth`.

**Step 1: Update position in `openContextMenu()`**

Current (line 440):
```go
x := m.sidebarWidth + m.tabsWidth
```

Replace with:
```go
x := m.sidebarWidth
```

**Step 2: Build and verify context menu appears at correct position**

```bash
go build ./... && ./kasmos
```
Expected: Space on an instance in the list shows the context menu anchored to the left edge of the middle column.

**Step 3: Commit**

```bash
git add app/app_actions.go
git commit -m "fix: update instance context menu x-position for middle column layout"
```

---

### Task 7: Update navigation tests for new arrow key behavior

**Files:**
- Modify: `app/app_test.go`

The existing tests for `←/→` navigation assert the old column order. Update them to match the new layout.

**Step 1: Find and update affected tests**

Search for tests asserting arrow key navigation:
```bash
grep -n "KeyArrowLeft\|KeyArrowRight\|left\|right" app/app_test.go | grep -i "slot\|focus"
```

Update any test that asserts:
- `←` from `slotAgent` → `slotSidebar` — change to `slotList`
- `←` from `slotList` → `slotAgent` — change to `slotSidebar`
- `→` from `slotSidebar` → `slotAgent` — change to `slotList`
- `→` from `slotAgent` → `slotList` — change to no-op (stays `slotAgent`) or remove

**Step 2: Run all navigation tests**

```bash
go test ./app/ -run TestNavigation -v
```
Expected: all PASS

**Step 3: Run full test suite**

```bash
go test ./... 
```
Expected: all PASS

**Step 4: Commit**

```bash
git add app/app_test.go
git commit -m "test: update arrow key navigation tests for new list-middle layout"
```
