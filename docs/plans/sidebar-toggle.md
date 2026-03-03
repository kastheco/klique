# Sidebar Toggle Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add ctrl+s keybind to toggle sidebar visibility, with center panel expanding to fill freed space.

**Architecture:** Boolean `sidebarHidden` on `home` struct gates layout calculation and view rendering. Two-step arrow-key reveal: first press shows sidebar, second press navigates to it. No persistence — sidebar starts visible every launch.

**Tech Stack:** Go, bubbletea, lipgloss, bubblezone

---

### Task 1: Add KeyToggleSidebar to keys package

**Files:**
- Modify: `keys/keys.go`

**Step 1: Add the key constant**

In the `const` block, add after `KeyViewPlan`:

```go
KeyToggleSidebar // Key for toggling sidebar visibility
```

**Step 2: Add the key string mapping**

In `GlobalKeyStringsMap`, add:

```go
"ctrl+s": KeyToggleSidebar,
```

**Step 3: Add the key binding**

In `GlobalkeyBindings`, add:

```go
KeyToggleSidebar: key.NewBinding(
    key.WithKeys("ctrl+s"),
    key.WithHelp("ctrl+s", "toggle sidebar"),
),
```

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Clean build

**Step 5: Commit**

```bash
git add keys/keys.go
git commit -m "feat: add KeyToggleSidebar keybind constant (ctrl+s)"
```

---

### Task 2: Add sidebarHidden state and layout logic

**Files:**
- Modify: `app/app.go`

**Step 1: Add `sidebarHidden` field to `home` struct**

After the `sidebarWidth` / `listWidth` / `tabsWidth` / `contentHeight` block (~line 156), add:

```go
// sidebarHidden tracks whether the sidebar is collapsed (ctrl+s toggle)
sidebarHidden bool
```

**Step 2: Update `updateHandleWindowSizeEvent` to respect sidebarHidden**

Replace the layout calculation block (lines 267-272) with:

```go
var sidebarWidth int
if m.sidebarHidden {
    sidebarWidth = 0
} else {
    sidebarWidth = int(float32(msg.Width) * 0.18)
    if sidebarWidth < 20 {
        sidebarWidth = 20
    }
}
listWidth := int(float32(msg.Width) * 0.20)
tabsWidth := msg.Width - sidebarWidth - listWidth
```

**Step 3: Update `View()` to conditionally render sidebar**

Replace the `JoinHorizontal` line (line 522) with:

```go
var listAndPreview string
if m.sidebarHidden {
    listAndPreview = lipgloss.JoinHorizontal(lipgloss.Top, previewWithPadding, listWithPadding)
} else {
    listAndPreview = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, previewWithPadding, listWithPadding)
}
```

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Clean build

**Step 5: Commit**

```bash
git add app/app.go
git commit -m "feat: add sidebarHidden state with layout recalculation"
```

---

### Task 3: Handle ctrl+s keypress and edge cases

**Files:**
- Modify: `app/app_input.go`

**Step 1: Add KeyToggleSidebar case to handleKeyPress**

In the `switch name` block (after the existing `case keys.KeyViewPlan:` block, around line 1110), add:

```go
case keys.KeyToggleSidebar:
    if m.sidebarHidden {
        // Show sidebar, keep current focus
        m.sidebarHidden = false
    } else {
        // Hide sidebar
        m.sidebarHidden = true
        // If sidebar was focused, move focus to tabbed view
        if m.focusedPanel == 0 {
            m.setFocus(1)
        }
    }
    return m, tea.WindowSize()
```

**Step 2: Update KeyLeft handler for two-step reveal**

Replace the existing `case keys.KeyLeft:` block (lines 1111-1115) with:

```go
case keys.KeyLeft:
    if m.focusedPanel == 1 && m.sidebarHidden {
        // Two-step reveal: first press shows sidebar without navigating
        m.sidebarHidden = false
        return m, tea.WindowSize()
    }
    // Normal: cycle left, no-op at left edge
    if m.focusedPanel > 0 {
        m.setFocus(m.focusedPanel - 1)
    }
    return m, nil
```

**Step 3: Update KeyFocusSidebar handler for two-step reveal**

Replace the existing `case keys.KeyFocusSidebar:` block (lines 1105-1108) with:

```go
case keys.KeyFocusSidebar:
    if m.sidebarHidden {
        // Two-step: reveal sidebar first, don't focus yet
        m.sidebarHidden = false
        return m, tea.WindowSize()
    }
    m.setFocus(0)
    return m, nil
```

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Clean build

**Step 5: Commit**

```bash
git add app/app_input.go
git commit -m "feat: handle ctrl+s toggle and two-step arrow reveal"
```

---

### Task 4: Add toggle to menu and run tests

**Files:**
- Modify: `ui/menu.go`

**Step 1: Add KeyToggleSidebar to menu system group**

In `addInstanceOptions()` (line 142), add `keys.KeyToggleSidebar` to the system group slice:

```go
systemGroup := []keys.KeyName{keys.KeyToggleSidebar, keys.KeyKillAllInTopic, keys.KeySearch, keys.KeyRepoSwitch, keys.KeyTab, keys.KeyHelp, keys.KeyQuit}
```

Also update `defaultMenuOptions` (line 52) to include it:

```go
var defaultMenuOptions = []keys.KeyName{keys.KeyNew, keys.KeySearch, keys.KeyToggleSidebar, keys.KeySpace, keys.KeyRepoSwitch, keys.KeyHelp, keys.KeyQuit}
```

And update `defaultSystemGroupSize` to account for the new entry:

```go
var defaultSystemGroupSize = 6
```

**Step 2: Run all tests**

Run: `go test ./...`
Expected: All tests pass

**Step 3: Manual verification**

Run: `go build -o klique . && ./klique`
Verify:
- ctrl+s hides the sidebar, center panel expands
- ctrl+s again shows the sidebar
- When sidebar is focused and ctrl+s pressed, focus moves to tabbed view
- Arrow left from preview when sidebar hidden: sidebar appears, focus stays on preview
- Arrow left again: focus moves to sidebar
- `s` key when sidebar hidden: sidebar appears without focus change
- `s` key again: focus moves to sidebar
- Menu bar shows `ctrl+s toggle sidebar`

**Step 4: Commit**

```bash
git add ui/menu.go
git commit -m "feat: add sidebar toggle to menu bar"
```
