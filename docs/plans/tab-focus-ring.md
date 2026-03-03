# Tab Focus Ring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the 3-panel focus model with a 5-slot Tab focus ring so arrow keys are captured per-pane and Tab cycles between sidebar, agent, diff, git, and instance list.

**Architecture:** Single `focusSlot int` (0-4) replaces `focusedPanel int` (0-2). Tab/Shift+Tab cycle the ring. Each slot routes `up/down/h/l` to its pane. Center tab slots (1-3) auto-switch the visible tab. Insert mode (`stateFocusAgent`) remains a separate state for full PTY forwarding.

**Tech Stack:** Go, bubbletea, lipgloss, existing Rosé Pine Moon palette

**Design doc:** `docs/plans/2026-02-22-tab-focus-ring-design.md`

---

### Task 1: Update Key Definitions

**Files:**
- Modify: `keys/keys.go`

**Step 1: Remove old bindings and add new ones**

Replace the F1/F2/F3 mappings with Shift+1/2/3 (`!`/`@`/`#`), remove `KeyLeft`/`KeyRight` as panel navigation, and remove `KeyShiftUp`/`KeyShiftDown`:

In `GlobalKeyStringsMap`, make these changes:
- Remove: `"f1": KeyTabAgent`, `"f2": KeyTabDiff`, `"f3": KeyTabGit`
- Remove: `"shift+up": KeyShiftUp`, `"shift+down": KeyShiftDown`
- Add: `"!": KeyTabAgent`, `"@": KeyTabDiff`, `"#": KeyTabGit`
- Change `"left"`, `"h"` from `KeyLeft` to new `KeyArrowLeft`
- Change `"right"`, `"l"` from `KeyRight` to new `KeyArrowRight`

In the `KeyName` constants:
- Remove: `KeyLeft`, `KeyRight`, `KeyShiftUp`, `KeyShiftDown`
- Add: `KeyArrowLeft`, `KeyArrowRight` (in-pane horizontal navigation, not panel switching)

In `GlobalkeyBindings`:
- Remove: `KeyLeft`, `KeyRight`, `KeyShiftUp`, `KeyShiftDown` bindings
- Add: `KeyArrowLeft`, `KeyArrowRight` bindings with help text `"←/h"` / `"→/l"`
- Update `KeyTabAgent` binding: keys `"!"`, help `"!/2/3"` → `"switch tab"`
- Update `KeyTabDiff` binding: keys `"@"`, help `"@"` → `"diff tab"`
- Update `KeyTabGit` binding: keys `"#"`, help `"#"` → `"git tab"`

**Step 2: Run tests**

Run: `go build ./...`
Expected: Compile errors in files that reference removed keys — that's expected, we fix them in subsequent tasks.

**Step 3: Commit**

```bash
git add keys/keys.go
git commit -m "refactor: update key definitions for tab focus ring"
```

---

### Task 2: Replace focusedPanel with focusSlot

**Files:**
- Modify: `app/app.go:141-142`
- Modify: `app/app_state.go:77-84`
- Modify: `ui/tabbed_window.go:58,64-76`

**Step 1: Update the home struct**

In `app/app.go`, replace:
```go
// focusedPanel tracks which panel has keyboard focus: 0=sidebar (left), 1=preview/center, 2=instance list (right)
focusedPanel int
```
with:
```go
// focusSlot tracks which pane has keyboard focus in the Tab ring:
// 0=sidebar, 1=agent tab, 2=diff tab, 3=git tab, 4=instance list
focusSlot int
```

**Step 2: Rewrite setFocus → setFocusSlot**

In `app/app_state.go`, replace the `setFocus` function (lines 77-84) with:

```go
// focusSlot constants for readability.
const (
	slotSidebar  = 0
	slotAgent    = 1
	slotDiff     = 2
	slotGit      = 3
	slotList     = 4
	slotCount    = 5
)

// setFocusSlot updates which pane has focus and syncs visual state.
func (m *home) setFocusSlot(slot int) {
	m.focusSlot = slot
	m.sidebar.SetFocused(slot == slotSidebar)
	m.list.SetFocused(slot == slotList)

	// Center pane is focused when any of the 3 center tabs is active.
	centerFocused := slot >= slotAgent && slot <= slotGit
	m.tabbedWindow.SetFocused(centerFocused)

	// When focusing a center tab, switch the visible tab to match.
	if centerFocused {
		m.tabbedWindow.SetActiveTab(slot - slotAgent) // slotAgent=1 → PreviewTab=0, etc.
	}
}

// nextFocusSlot advances the focus ring forward, skipping sidebar when hidden.
func (m *home) nextFocusSlot() {
	next := (m.focusSlot + 1) % slotCount
	if next == slotSidebar && m.sidebarHidden {
		next = slotAgent
	}
	m.setFocusSlot(next)
}

// prevFocusSlot moves the focus ring backward, skipping sidebar when hidden.
func (m *home) prevFocusSlot() {
	prev := (m.focusSlot - 1 + slotCount) % slotCount
	if prev == slotSidebar && m.sidebarHidden {
		prev = slotList
	}
	m.setFocusSlot(prev)
}
```

**Step 3: Add focusedTab helper to TabbedWindow**

In `ui/tabbed_window.go`, add a new field and method so the TabbedWindow knows which specific tab is focused (for gradient rendering):

```go
// focusedTab tracks which tab (0=agent, 1=diff, 2=git) has Tab-ring focus.
// -1 means no center tab is focused.
focusedTab int
```

Initialize `focusedTab: -1` in `NewTabbedWindow`.

Add method:
```go
// SetFocusedTab sets which specific tab has focus ring focus. -1 = none.
func (w *TabbedWindow) SetFocusedTab(tab int) {
	w.focusedTab = tab
}
```

Update `setFocusSlot` to call this:
```go
if centerFocused {
	m.tabbedWindow.SetFocusedTab(slot - slotAgent)
} else {
	m.tabbedWindow.SetFocusedTab(-1)
}
```

**Step 4: Commit**

```bash
git add app/app.go app/app_state.go ui/tabbed_window.go
git commit -m "refactor: replace focusedPanel with 5-slot focusSlot ring"
```

---

### Task 3: Rewrite Key Routing in app_input.go

**Files:**
- Modify: `app/app_input.go`
- Modify: `app/app_actions.go:293`

This is the largest task. Replace all `focusedPanel` references and rewrite the key handling for the new model.

**Step 1: Fix all focusedPanel → focusSlot references**

Use `sd` to do a mechanical rename first:
```bash
sd 'focusedPanel' 'focusSlot' app/app_input.go app/app_actions.go
sd 'setFocus\(' 'setFocusSlot(' app/app_input.go
```

Then manually fix the logic — the old `== 0` checks become `== slotSidebar`, `== 1` becomes center-tab checks, `== 2` becomes `== slotList`.

**Step 2: Rewrite KeyUp/KeyDown handler (lines ~856-873)**

Replace the current up/down handler with slot-aware routing:

```go
case keys.KeyUp:
	m.tabbedWindow.ClearDocumentMode()
	switch m.focusSlot {
	case slotSidebar:
		m.sidebar.Up()
		m.filterInstancesByTopic()
	case slotAgent:
		m.tabbedWindow.ScrollUp()
	case slotDiff:
		m.tabbedWindow.ScrollUp()
	case slotGit:
		// Forwarded to lazygit in the git tab key handler below
	case slotList:
		m.list.Up()
	}
	return m, m.instanceChanged()
case keys.KeyDown:
	m.tabbedWindow.ClearDocumentMode()
	switch m.focusSlot {
	case slotSidebar:
		m.sidebar.Down()
		m.filterInstancesByTopic()
	case slotAgent:
		m.tabbedWindow.ScrollDown()
	case slotDiff:
		m.tabbedWindow.ScrollDown()
	case slotGit:
		// Forwarded to lazygit in the git tab key handler below
	case slotList:
		m.list.Down()
	}
	return m, m.instanceChanged()
```

**Step 3: Rewrite KeyTab handler (line ~880)**

Replace the current tab handler (which cycles center tabs) with focus ring cycling:

```go
case keys.KeyTab:
	m.nextFocusSlot()
	// Handle git tab lifecycle
	if m.focusSlot == slotGit {
		cmd := m.spawnGitTab()
		return m, tea.Batch(m.instanceChanged(), cmd)
	}
	// Kill lazygit when leaving git tab
	if m.tabbedWindow.IsInGitTab() && m.focusSlot != slotGit {
		m.killGitTab()
	}
	return m, m.instanceChanged()
```

Note: We need to handle Shift+Tab as well. Add to the raw key handler section (around line 1143 where `tea.KeyTab` is handled):

```go
case tea.KeyShiftTab:
	m.prevFocusSlot()
	if m.focusSlot == slotGit {
		cmd := m.spawnGitTab()
		return m, tea.Batch(m.instanceChanged(), cmd)
	}
	if m.tabbedWindow.IsInGitTab() && m.focusSlot != slotGit {
		m.killGitTab()
	}
	return m, m.instanceChanged()
```

**Step 4: Rewrite KeyLeft/KeyRight → KeyArrowLeft/KeyArrowRight**

Remove the entire `case keys.KeyLeft:` and `case keys.KeyRight:` blocks (lines ~1076-1112). Replace with in-pane horizontal navigation:

```go
case keys.KeyArrowLeft:
	switch m.focusSlot {
	case slotSidebar:
		if m.sidebar.IsTreeMode() {
			m.sidebar.Left()
			m.filterInstancesByTopic()
		}
	case slotDiff:
		// Future: collapse file section
	case slotGit:
		// Forwarded to lazygit PTY (handled in git focus section)
	}
	return m, nil
case keys.KeyArrowRight:
	switch m.focusSlot {
	case slotSidebar:
		if m.sidebar.IsTreeMode() {
			m.sidebar.Right()
			m.filterInstancesByTopic()
		}
	case slotDiff:
		// Future: expand file section
	case slotGit:
		// Forwarded to lazygit PTY (handled in git focus section)
	}
	return m, nil
```

**Step 5: Remove KeyShiftUp/KeyShiftDown handler**

Delete the `case keys.KeyShiftUp:` and `case keys.KeyShiftDown:` blocks (lines ~874-879). These are no longer needed.

**Step 6: Update KeyTabAgent/KeyTabDiff/KeyTabGit handler**

The existing `case keys.KeyTabAgent, keys.KeyTabDiff, keys.KeyTabGit:` calls `m.switchToTab(name)`. Rewrite `switchToTab` in `app_state.go` to use `setFocusSlot`:

```go
func (m *home) switchToTab(name keys.KeyName) (tea.Model, tea.Cmd) {
	var targetSlot int
	switch name {
	case keys.KeyTabAgent:
		targetSlot = slotAgent
	case keys.KeyTabDiff:
		targetSlot = slotDiff
	case keys.KeyTabGit:
		targetSlot = slotGit
	default:
		return m, nil
	}

	if m.focusSlot == targetSlot {
		return m, nil
	}

	wasGitTab := m.tabbedWindow.IsInGitTab()
	m.setFocusSlot(targetSlot)

	if wasGitTab && m.focusSlot != slotGit {
		m.killGitTab()
	}
	if m.focusSlot == slotGit {
		cmd := m.spawnGitTab()
		return m, tea.Batch(m.instanceChanged(), cmd)
	}
	return m, m.instanceChanged()
}
```

**Step 7: Update KeyFocusSidebar handler**

Replace `m.setFocus(0)` calls with `m.setFocusSlot(slotSidebar)`.

**Step 8: Update KeyToggleSidebar handler**

Replace `m.setFocus(1)` with `m.setFocusSlot(slotAgent)` when sidebar hides and focus was on sidebar.

**Step 9: Update KeySpace handler**

Replace `m.focusedPanel == 0` checks with `m.focusSlot == slotSidebar`.

**Step 10: Update KeyEnter handler**

Replace `m.focusedPanel == 0` check with `m.focusSlot == slotSidebar`.

**Step 11: Update mouse click handler**

In the mouse click section (~lines 113-159), replace `m.setFocus(0/1/2)` calls:
- Sidebar click: `m.setFocusSlot(slotSidebar)`
- Center click: `m.setFocusSlot(slotAgent + m.tabbedWindow.GetActiveTab())` — focus whichever center tab is visible
- List click: `m.setFocusSlot(slotList)`

**Step 12: Update app_actions.go**

In `app/app_actions.go:293`, replace `m.focusedPanel == 0` with `m.focusSlot == slotSidebar`.

**Step 13: Update KeySearch handler**

Replace `m.setFocus(0)` with `m.setFocusSlot(slotSidebar)`.

**Step 14: Git slot arrow key forwarding**

When `focusSlot == slotGit` and the git pane is running, forward arrow keys to the lazygit PTY. Add a helper or inline the forwarding in the up/down/left/right handlers. The git pane already has PTY forwarding in focus mode — reuse the `keyToBytes` function and `gitPane.Write()` pattern from `stateFocusAgent` handling.

In the `case keys.KeyUp/KeyDown` and `case keys.KeyArrowLeft/KeyArrowRight` handlers, for `slotGit`:
```go
case slotGit:
	gitPane := m.tabbedWindow.GetGitPane()
	if gitPane.IsRunning() {
		gitPane.Write(keyToBytes(msg.(tea.KeyMsg)))
	}
```

Note: The `msg` in `handleKeyPress` is the original `tea.KeyMsg`. You'll need to thread it through or capture it. Check how the existing `stateFocusAgent` handler accesses the key msg.

**Step 15: Run tests and fix compilation**

Run: `go build ./...`
Fix any remaining compilation errors.

Run: `go test ./app/... -v`
Expect test failures — the tests reference `focusedPanel`. Fix in Task 4.

**Step 16: Commit**

```bash
git add app/app_input.go app/app_state.go app/app_actions.go
git commit -m "feat: implement tab focus ring key routing"
```

---

### Task 4: Update Tests

**Files:**
- Modify: `app/app_test.go`

**Step 1: Rewrite focus navigation tests**

The existing tests (lines ~491-577) test `focusedPanel` with left/right arrow navigation. Rewrite them for the new model:

- Remove all tests that assert `focusedPanel` values
- Remove tests for left/right arrow panel switching (those keys no longer switch panels)
- Add tests for Tab cycling:
  - Tab from slot 4 (list) → slot 0 (sidebar)
  - Tab from slot 0 → slot 1 (agent)
  - Tab from slot 3 (git) → slot 4 (list)
  - Shift+Tab from slot 0 → slot 4
- Add tests for Tab skipping hidden sidebar:
  - Tab from slot 4 with sidebar hidden → slot 1 (skips 0)
  - Shift+Tab from slot 1 with sidebar hidden → slot 4 (skips 0)
- Add tests for `!`/`@`/`#` jumping to specific slots
- Add tests for `s` jumping to sidebar slot
- Keep `ctrl+s` toggle tests but update assertions to use `focusSlot`

All assertions change from `homeModel.focusedPanel` to `homeModel.focusSlot` and use the slot constants.

**Step 2: Run tests**

Run: `go test ./app/... -v`
Expected: All tests pass.

Run: `go test ./... -v`
Expected: All tests pass.

**Step 3: Commit**

```bash
git add app/app_test.go
git commit -m "test: rewrite focus navigation tests for tab ring model"
```

---

### Task 5: Update Gradient and Tab Rendering

**Files:**
- Modify: `ui/theme.go:25-26`
- Modify: `ui/tabbed_window.go:333-385`

**Step 1: Update gradient constants**

In `ui/theme.go`, change:
```go
GradientStart = "#ea9a97" // rose
GradientEnd   = "#c4a7e7" // iris
```
to:
```go
GradientStart = "#9ccfd8" // foam
GradientEnd   = "#c4a7e7" // iris
```

**Step 2: Update tab label rendering for focus state**

In `ui/tabbed_window.go`, in the `String()` method's tab rendering loop (around line 380), change the gradient condition to only apply when the specific tab is focused:

Replace:
```go
if isActive && !w.focusMode {
	renderedTabs = append(renderedTabs, style.Render(GradientText(t, GradientStart, GradientEnd)))
} else {
	renderedTabs = append(renderedTabs, style.Render(t))
}
```

With:
```go
switch {
case isActive && i == w.focusedTab && !w.focusMode:
	// Focused tab in the ring: foam→iris gradient
	renderedTabs = append(renderedTabs, style.Render(GradientText(t, GradientStart, GradientEnd)))
case isActive:
	// Active but not ring-focused: normal text color
	renderedTabs = append(renderedTabs, style.Render(lipgloss.NewStyle().Foreground(ColorText).Render(t)))
default:
	// Inactive tab: muted
	renderedTabs = append(renderedTabs, style.Render(lipgloss.NewStyle().Foreground(ColorMuted).Render(t)))
}
```

**Step 3: Run and verify**

Run: `go build ./...`
Expected: Compiles cleanly.

Run: `go test ./ui/... -v`
Expected: All tests pass.

**Step 4: Commit**

```bash
git add ui/theme.go ui/tabbed_window.go
git commit -m "feat: foam→iris gradient on focused tab, muted inactive tabs"
```

---

### Task 6: Update Help Screen

**Files:**
- Modify: `app/help.go` (or wherever the help keybinding display is rendered)

**Step 1: Find and update help text**

The help screen likely lists keybindings. Update it to reflect:
- `Tab` / `Shift+Tab`: cycle panes
- `!`/`@`/`#`: jump to agent/diff/git
- Remove `←/h` / `→/l` as panel navigation
- Remove `Shift+↑/↓` as scroll
- Remove F1/F2/F3 references

**Step 2: Run and verify**

Run: `go build ./...`
Expected: Compiles cleanly.

**Step 3: Commit**

```bash
git add app/help.go
git commit -m "docs: update help screen for tab focus ring keybindings"
```

---

### Task 7: Startup Default and Final Verification

**Files:**
- Modify: `app/app.go` (wherever `focusedPanel` is initialized)

**Step 1: Set startup default**

Find where the `home` struct is initialized and set `focusSlot: slotList` (value 4) so the instance list is focused on startup, matching current behavior.

**Step 2: Full test suite**

Run: `go test ./... -v`
Expected: All tests pass.

Run: `go build ./...`
Expected: Clean build.

**Step 3: Manual smoke test**

Launch the app and verify:
- Tab cycles: list → sidebar → agent → diff → git → list
- Shift+Tab cycles backward
- `!`/`@`/`#` jump to center tabs
- `s` jumps to sidebar
- Up/down navigate within the focused pane
- Arrow keys in sidebar do tree expand/collapse (not panel switch)
- Git tab receives arrow keys when Tab-focused
- Insert mode (`i`) still works, Ctrl+Space exits
- Gradient visible on focused tab label (foam→iris)
- Unfocused tabs show muted text

**Step 4: Commit**

```bash
git add app/app.go
git commit -m "feat: set instance list as default focus slot on startup"
```
