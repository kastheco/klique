# Sidebar Navigation-Driven Show/Hide Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the two-step sidebar reveal with single-motion show+focus, add hide-on-left-from-sidebar, and remove `ctrl+s` from the bottom menu while keeping it as a hidden secondary toggle.

**Architecture:** Modify the `KeyLeft`, `KeyFocusSidebar`, and `KeyToggleSidebar` handlers in `app_input.go`. Remove `KeyToggleSidebar` from menu options in `menu.go`. Update existing tests to match new behavior.

**Tech Stack:** Go, bubbletea, lipgloss

---

### Task 1: Update `KeyLeft` handler — show+focus and hide-from-sidebar

**Files:**
- Modify: `app/app_input.go:1129-1139`

**Step 1: Replace the `KeyLeft` case block**

Find the current `case keys.KeyLeft:` block (lines 1129-1139):

```go
case keys.KeyLeft:
    if m.focusedPanel == 1 && m.sidebarHidden {
        // Two-step reveal: first press shows sidebar without navigating focus
        m.sidebarHidden = false
        return m, tea.WindowSize()
    }
    // Cycle left: list(2) → preview(1) → sidebar(0), no-op at left edge.
    if m.focusedPanel > 0 {
        m.setFocus(m.focusedPanel - 1)
    }
    return m, nil
```

Replace with:

```go
case keys.KeyLeft:
    if m.focusedPanel == 0 {
        // Already on sidebar: hide it and move focus to center
        if !m.sidebar.IsSearchActive() {
            m.sidebarHidden = true
            m.setFocus(1)
            return m, tea.WindowSize()
        }
        return m, nil
    }
    if m.focusedPanel == 1 && m.sidebarHidden {
        // Show sidebar and focus it in one motion
        m.sidebarHidden = false
        m.setFocus(0)
        return m, tea.WindowSize()
    }
    // Normal cycle left: list(2) → preview(1) → sidebar(0)
    if m.focusedPanel > 0 {
        m.setFocus(m.focusedPanel - 1)
    }
    return m, nil
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Clean build

**Step 3: Commit**

```bash
git add app/app_input.go
git commit -m "feat: left key shows+focuses sidebar in one motion, hides from sidebar"
```

---

### Task 2: Update `KeyFocusSidebar` (`s` key) — show+focus in one motion

**Files:**
- Modify: `app/app_input.go:1105-1113`

**Step 1: Replace the `KeyFocusSidebar` case block**

Find the current block (lines 1105-1113):

```go
case keys.KeyFocusSidebar:
    if m.sidebarHidden {
        // Two-step: reveal sidebar first, don't focus yet
        m.sidebarHidden = false
        return m, tea.WindowSize()
    }
    // s key always jumps directly to the sidebar regardless of current panel.
    m.setFocus(0)
    return m, nil
```

Replace with:

```go
case keys.KeyFocusSidebar:
    if m.sidebarHidden {
        // Show and focus in one motion
        m.sidebarHidden = false
        m.setFocus(0)
        return m, tea.WindowSize()
    }
    // s key always jumps directly to the sidebar regardless of current panel.
    m.setFocus(0)
    return m, nil
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Clean build

**Step 3: Commit**

```bash
git add app/app_input.go
git commit -m "feat: s key shows+focuses sidebar in one motion"
```

---

### Task 3: Remove `KeyToggleSidebar` from bottom menu

**Files:**
- Modify: `ui/menu.go:52-53,142`

**Step 1: Remove from `defaultMenuOptions`**

Change line 52 from:

```go
var defaultMenuOptions = []keys.KeyName{keys.KeyNew, keys.KeySearch, keys.KeyToggleSidebar, keys.KeySpace, keys.KeyRepoSwitch, keys.KeyHelp, keys.KeyQuit}
```

To:

```go
var defaultMenuOptions = []keys.KeyName{keys.KeyNew, keys.KeySearch, keys.KeySpace, keys.KeyRepoSwitch, keys.KeyHelp, keys.KeyQuit}
```

**Step 2: Update `defaultSystemGroupSize`**

Change line 53 from:

```go
var defaultSystemGroupSize = 6 // ctrl+s toggle sidebar, / search, space actions, R repo switch, ? help, q quit
```

To:

```go
var defaultSystemGroupSize = 5 // / search, space actions, R repo switch, ? help, q quit
```

**Step 3: Remove from `addInstanceOptions` system group**

Change line 142 from:

```go
systemGroup := []keys.KeyName{keys.KeyToggleSidebar, keys.KeyKillAllInTopic, keys.KeySearch, keys.KeyRepoSwitch, keys.KeyTab, keys.KeyHelp, keys.KeyQuit}
```

To:

```go
systemGroup := []keys.KeyName{keys.KeyKillAllInTopic, keys.KeySearch, keys.KeyRepoSwitch, keys.KeyTab, keys.KeyHelp, keys.KeyQuit}
```

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Clean build

**Step 5: Commit**

```bash
git add ui/menu.go
git commit -m "refactor: remove ctrl+s toggle from bottom menu bar"
```

---

### Task 4: Update tests to match new behavior

**Files:**
- Modify: `app/app_test.go:491-566`

**Step 1: Rewrite the sidebar toggle test cases**

Replace the entire sidebar toggle test block (from `t.Run("ctrl+s hides sidebar..."` through the end of the `s moves focus to sidebar` test) with tests matching the new behavior:

```go
t.Run("ctrl+s hides sidebar and moves focus from sidebar to panel 1", func(t *testing.T) {
    h := newTestHome()
    h.sidebarHidden = false
    h.setFocus(0)

    homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyCtrlS})

    assert.True(t, homeModel.sidebarHidden)
    assert.Equal(t, 1, homeModel.focusedPanel)
})

t.Run("ctrl+s hides sidebar and keeps focus when panel 1 is focused", func(t *testing.T) {
    h := newTestHome()
    h.sidebarHidden = false
    h.setFocus(1)

    homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyCtrlS})

    assert.True(t, homeModel.sidebarHidden)
    assert.Equal(t, 1, homeModel.focusedPanel)
})

t.Run("ctrl+s shows sidebar and keeps focus when sidebar is hidden", func(t *testing.T) {
    h := newTestHome()
    h.sidebarHidden = true
    h.setFocus(2)

    homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyCtrlS})

    assert.False(t, homeModel.sidebarHidden)
    assert.Equal(t, 2, homeModel.focusedPanel)
})

t.Run("left from panel 1 shows and focuses sidebar when hidden", func(t *testing.T) {
    h := newTestHome()
    h.sidebarHidden = true
    h.setFocus(1)

    homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyLeft})

    assert.False(t, homeModel.sidebarHidden)
    assert.Equal(t, 0, homeModel.focusedPanel)
})

t.Run("left from sidebar hides sidebar and focuses panel 1", func(t *testing.T) {
    h := newTestHome()
    h.sidebarHidden = false
    h.setFocus(0)

    homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyLeft})

    assert.True(t, homeModel.sidebarHidden)
    assert.Equal(t, 1, homeModel.focusedPanel)
})

t.Run("h moves focus to sidebar when panel 1 is focused and sidebar is visible", func(t *testing.T) {
    h := newTestHome()
    h.sidebarHidden = false
    h.setFocus(1)

    homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})

    assert.False(t, homeModel.sidebarHidden)
    assert.Equal(t, 0, homeModel.focusedPanel)
})

t.Run("s shows and focuses sidebar when hidden", func(t *testing.T) {
    h := newTestHome()
    h.sidebarHidden = true
    h.setFocus(2)

    homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

    assert.False(t, homeModel.sidebarHidden)
    assert.Equal(t, 0, homeModel.focusedPanel)
})

t.Run("s moves focus to sidebar when sidebar is visible", func(t *testing.T) {
    h := newTestHome()
    h.sidebarHidden = false
    h.setFocus(2)

    homeModel := handle(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

    assert.False(t, homeModel.sidebarHidden)
    assert.Equal(t, 0, homeModel.focusedPanel)
})
```

**Step 2: Run the tests**

Run: `go test ./app/ -run TestSidebar -v`
Expected: All 8 tests pass

**Step 3: Run full test suite**

Run: `go test ./...`
Expected: All tests pass

**Step 4: Commit**

```bash
git add app/app_test.go
git commit -m "test: update sidebar toggle tests for nav-driven show/hide"
```

---

### Task 5: Manual verification

**Step 1: Build and run**

Run: `go build -o klique . && ./klique`

**Step 2: Verify all behaviors**

- Start on sidebar (panel 0). Press `left` → sidebar hides, focus moves to center pane.
- Press `left` from center pane → sidebar shows AND focuses in one motion.
- Press `right` to go back to center. Press `s` → sidebar shows and focuses.
- `ctrl+s` still toggles sidebar (hidden shortcut, not in bottom menu).
- Bottom menu bar no longer shows `ctrl+s toggle sidebar`.
- Normal `left/right` navigation between visible panels works unchanged.
- `left` from panel 2 → panel 1 (unchanged).
- `right` from panel 0 → panel 1 (unchanged).
