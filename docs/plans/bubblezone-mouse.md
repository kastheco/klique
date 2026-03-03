# Bubblezone Mouse Integration — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace positional mouse math with bubblezone hit detection, enable mouse-click escape from agent focus mode, and forward full SGR mouse events to the embedded PTY.

**Architecture:** Zone marks are added in render paths (`String()` methods) and consumed in the input path (`handleMouse`). A new SGR encoder translates bubbletea `MouseMsg` into terminal escape sequences for PTY forwarding. Focus-mode mouse routing checks `zone-agent-pane` bounds to decide between PTY forwarding and focus-exit.

**Tech Stack:** Go, bubbletea v1.3.10, bubblezone v1.0.0, lipgloss

**Size:** Medium (estimated ~4 hours, 5 tasks, 2 waves)

---

## Wave 1: Zone Infrastructure & SGR Encoder

> **Foundation work:** Zone constants, render-path marks, and the SGR encoder must exist before the mouse handler can consume them.

### Task 1: Zone Constants and SGR Encoder

**Files:**
- Create: `ui/zones.go`
- Create: `app/mouse_sgr.go`
- Create: `app/mouse_sgr_test.go`

**Step 1: Write the failing test for SGR encoding**

Create `app/mouse_sgr_test.go` with table-driven tests:

```go
package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestMouseToSGR(t *testing.T) {
	tests := []struct {
		name    string
		msg     tea.MouseMsg
		offsetX int
		offsetY int
		want    string
	}{
		{
			name: "left click at origin",
			msg: tea.MouseMsg{
				X:      10,
				Y:      5,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<0;1;1M",
		},
		{
			name: "left click with offset",
			msg: tea.MouseMsg{
				X:      15,
				Y:      8,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<0;6;4M",
		},
		{
			name: "left release",
			msg: tea.MouseMsg{
				X:      10,
				Y:      5,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionRelease,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<0;1;1m",
		},
		{
			name: "right click",
			msg: tea.MouseMsg{
				X:      10,
				Y:      5,
				Button: tea.MouseButtonRight,
				Action: tea.MouseActionPress,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<2;1;1M",
		},
		{
			name: "middle click",
			msg: tea.MouseMsg{
				X:      10,
				Y:      5,
				Button: tea.MouseButtonMiddle,
				Action: tea.MouseActionPress,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<1;1;1M",
		},
		{
			name: "wheel up",
			msg: tea.MouseMsg{
				X:      10,
				Y:      5,
				Button: tea.MouseButtonWheelUp,
				Action: tea.MouseActionPress,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<64;1;1M",
		},
		{
			name: "wheel down",
			msg: tea.MouseMsg{
				X:      10,
				Y:      5,
				Button: tea.MouseButtonWheelDown,
				Action: tea.MouseActionPress,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<65;1;1M",
		},
		{
			name: "left drag (motion with button)",
			msg: tea.MouseMsg{
				X:      12,
				Y:      7,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionMotion,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<32;3;3M",
		},
		{
			name: "shift+left click",
			msg: tea.MouseMsg{
				X:      10,
				Y:      5,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
				Shift:  true,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<4;1;1M",
		},
		{
			name: "ctrl+left click",
			msg: tea.MouseMsg{
				X:      10,
				Y:      5,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
				Ctrl:   true,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<16;1;1M",
		},
		{
			name: "alt+left click",
			msg: tea.MouseMsg{
				X:      10,
				Y:      5,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
				Alt:    true,
			},
			offsetX: 10,
			offsetY: 5,
			want:    "\x1b[<8;1;1M",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mouseToSGR(tt.msg, tt.offsetX, tt.offsetY)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestMouseToSGR_NilForUnknownButton(t *testing.T) {
	msg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButton11,
		Action: tea.MouseActionPress,
	}
	got := mouseToSGR(msg, 10, 5)
	assert.Nil(t, got, "unknown button should return nil")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestMouseToSGR -v`
Expected: FAIL — `mouseToSGR` undefined

**Step 3: Create zone constants**

Create `ui/zones.go`:

```go
package ui

import "fmt"

// Zone ID constants for bubblezone hit detection.
const (
	ZoneNavPanel  = "zone-nav-panel"
	ZoneNavSearch = "zone-nav-search"
	ZoneNavRepo   = "zone-nav-repo"
	ZoneTabAgent  = "zone-tab-agent"
	ZoneTabDiff   = "zone-tab-diff"
	ZoneTabInfo   = "zone-tab-info"
	ZoneAgentPane = "zone-agent-pane"
)

// Tab zone IDs indexed by tab constant (PreviewTab=0, DiffTab=1, InfoTab=2).
var TabZoneIDs = [3]string{ZoneTabAgent, ZoneTabDiff, ZoneTabInfo}

// NavRowZoneID returns the zone ID for a navigation panel row by index.
func NavRowZoneID(idx int) string {
	return fmt.Sprintf("zone-nav-row-%d", idx)
}
```

**Step 4: Write SGR encoder**

Create `app/mouse_sgr.go`:

```go
package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// mouseToSGR encodes a bubbletea MouseMsg as an SGR extended mouse sequence
// (\x1b[<button;col;row{M|m}) with coordinates translated relative to the
// given offset. Returns nil if the button is not encodable.
//
// SGR protocol reference:
//   - Button: 0=left, 1=middle, 2=right, 64=wheel-up, 65=wheel-down
//   - Modifiers: shift=+4, alt=+8, ctrl=+16
//   - Motion: +32
//   - Coordinates: 1-indexed
//   - Terminator: M=press/motion, m=release
func mouseToSGR(msg tea.MouseMsg, offsetX, offsetY int) []byte {
	var btn int
	switch msg.Button {
	case tea.MouseButtonLeft:
		btn = 0
	case tea.MouseButtonMiddle:
		btn = 1
	case tea.MouseButtonRight:
		btn = 2
	case tea.MouseButtonWheelUp:
		btn = 64
	case tea.MouseButtonWheelDown:
		btn = 65
	case tea.MouseButtonWheelLeft:
		btn = 66
	case tea.MouseButtonWheelRight:
		btn = 67
	default:
		return nil
	}

	// Modifier bits
	if msg.Shift {
		btn += 4
	}
	if msg.Alt {
		btn += 8
	}
	if msg.Ctrl {
		btn += 16
	}

	// Motion bit
	if msg.Action == tea.MouseActionMotion {
		btn += 32
	}

	// Translate coordinates: 1-indexed relative to offset
	col := msg.X - offsetX + 1
	row := msg.Y - offsetY + 1
	if col < 1 {
		col = 1
	}
	if row < 1 {
		row = 1
	}

	// Terminator: M for press/motion, m for release
	term := 'M'
	if msg.Action == tea.MouseActionRelease {
		term = 'm'
	}

	return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", btn, col, row, term))
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./app/ -run TestMouseToSGR -v`
Expected: all PASS

**Step 6: Commit**

```bash
git add ui/zones.go app/mouse_sgr.go app/mouse_sgr_test.go
git commit -m "feat: add zone constants and SGR mouse encoder for bubblezone integration"
```

---

### Task 2: Add Zone Marks to Render Paths

**Files:**
- Modify: `ui/navigation_panel.go:621-698` (String method)
- Modify: `ui/tabbed_window.go:350-430` (String method)

**Step 1: Write test for zone marks in nav panel**

There's no easy unit test for zone marks in rendered output (zone markers are invisible ANSI escapes stripped by `zone.Scan`). Instead, verify the integration works by checking the existing `zone.Get()` API works after Scan. This is best validated by the integration tests in Task 3.

For now, add a simple compile-check test:

Add to existing test files or create `ui/zones_test.go`:

```go
package ui

import "testing"

func TestNavRowZoneID(t *testing.T) {
	id := NavRowZoneID(42)
	if id != "zone-nav-row-42" then {
		t.Fatalf("expected zone-nav-row-42, got %s", id)
	}
}
```

Wait — use testify:

```go
package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNavRowZoneID(t *testing.T) {
	assert.Equal(t, "zone-nav-row-0", NavRowZoneID(0))
	assert.Equal(t, "zone-nav-row-42", NavRowZoneID(42))
}

func TestTabZoneIDs(t *testing.T) {
	assert.Equal(t, ZoneTabAgent, TabZoneIDs[PreviewTab])
	assert.Equal(t, ZoneTabDiff, TabZoneIDs[DiffTab])
	assert.Equal(t, ZoneTabInfo, TabZoneIDs[InfoTab])
}
```

**Step 2: Run test to verify it passes (these are for the constants, already created)**

Run: `go test ./ui/ -run TestNavRowZoneID -v && go test ./ui/ -run TestTabZoneIDs -v`
Expected: PASS

**Step 3: Add zone marks to NavigationPanel.String()**

Modify `ui/navigation_panel.go` — the `String()` method.

The key changes inside the row rendering loop (lines 644-675):

1. Track the mapping from visible-row index to `rows` index (needed for click handling later)
2. Wrap each visible row's rendered line with `zone.Mark(NavRowZoneID(visibleIdx), line)`
3. Wrap the search box with `zone.Mark(ZoneNavSearch, searchBox)`
4. Replace `ZoneRepoSwitch` with `ZoneNavRepo` in the repo label mark
5. Wrap the entire panel output with `zone.Mark(ZoneNavPanel, ...)`

In the `String()` method, change:

```go
// Before the row loop:
searchBox := zone.Mark(ZoneNavSearch, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorOverlay).Padding(0, 1).Width(innerWidth - 4).Render(search))

// Inside the row loop, replace:
//   visible = append(visible, prefix+line)
// with:
visible = append(visible, zone.Mark(NavRowZoneID(i), prefix+line))

// For the repo label, replace ZoneRepoSwitch with ZoneNavRepo:
repoLabel = zone.Mark(ZoneNavRepo, lipgloss.NewStyle()...)

// Wrap the entire return:
panelContent := lipgloss.Place(n.width, n.height, lipgloss.Left, lipgloss.Top, border.Width(innerWidth).Height(height).Render(content))
return zone.Mark(ZoneNavPanel, panelContent)
```

Also update the `ZoneRepoSwitch` constant usage — rename to `ZoneNavRepo` or keep both for backward compat. Since `ZoneRepoSwitch` is only used in `navigation_panel.go` and `app_input.go`, replace it:
- Remove `const ZoneRepoSwitch = "repo-switch"` from `navigation_panel.go`
- Update `app_input.go` references from `ui.ZoneRepoSwitch` to `ui.ZoneNavRepo`

**Step 4: Add zone marks to TabbedWindow.String()**

Modify `ui/tabbed_window.go` — the `String()` method.

In the tab rendering loop (lines 371-408), wrap each rendered tab:

```go
// After the switch/case block that produces the styled tab string,
// wrap it with zone.Mark before appending:
zoneID := TabZoneIDs[i]
renderedTabs = append(renderedTabs, zone.Mark(zoneID, styledTab))
```

For the content area, wrap the window with `zone-agent-pane` when on PreviewTab:

```go
// After line 427 where window is built:
if w.activeTab == PreviewTab {
    window = zone.Mark(ZoneAgentPane, window)
}
```

Import `zone "github.com/lrstanley/bubblezone"` at the top of tabbed_window.go (it's not imported yet).

**Step 5: Remove HandleTabClick from TabbedWindow**

Delete the `HandleTabClick` method (lines 325-348) — it will be replaced by zone-based detection in `handleMouse`. Also remove `HandleTabClick` from `List` if it exists (check — it was in the old code but the list was merged into nav).

**Step 6: Run existing tests to verify nothing breaks**

Run: `go test ./ui/... -v`
Expected: PASS (existing tests should still pass)

Run: `go test ./app/... -v`
Expected: PASS (may need to update any tests calling HandleTabClick)

**Step 7: Commit**

```bash
git add ui/navigation_panel.go ui/tabbed_window.go ui/zones_test.go
git commit -m "feat: add bubblezone marks to nav panel and tabbed window render paths"
```

---

## Wave 2: Mouse Handler Rewrite

> **Depends on Wave 1:** Zone marks must exist in the render output before the mouse handler can query them via `zone.Get()`.

### Task 3: Rewrite handleMouse with Zone-Based Hit Detection

**Files:**
- Modify: `app/app_input.go:52-159` (handleMouse + handleRightClick)

**Step 1: Write test for focus-mode mouse escape**

Add to `app/app_test.go` or a new `app/app_mouse_test.go`:

```go
package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestFocusMode_ClickOutsideExitsFocus(t *testing.T) {
	// Create a home model in focus mode
	h := &home{
		state: stateFocusAgent,
		// ... minimal fields needed
	}
	// Simulate a click at X=0 (nav panel area, outside agent pane)
	msg := tea.MouseMsg{
		X:      0,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	model, _ := h.handleMouse(msg)
	result := model.(*home)
	assert.NotEqual(t, stateFocusAgent, result.state,
		"clicking outside agent pane should exit focus mode")
}
```

Note: This test will need the same test harness setup as the existing `app_test.go` tests (mock nav, tabbedWindow, etc.). Match the existing test patterns.

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestFocusMode_ClickOutsideExitsFocus -v`
Expected: FAIL — current handleMouse bails on `m.state != stateDefault`

**Step 3: Rewrite handleMouse**

Replace the `handleMouse` method in `app/app_input.go`. The new structure:

```go
func (m *home) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Track hover state for the repo button on any mouse event
	repoHovered := zone.Get(ui.ZoneNavRepo).InBounds(msg)
	m.nav.SetRepoHovered(repoHovered)

	// === Focus mode mouse routing ===
	if m.state == stateFocusAgent {
		return m.handleFocusModeMouse(msg)
	}

	// Only handle press events for non-focus-mode interactions
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	// Handle scroll wheel
	if msg.IsWheel() {
		return m.handleScrollWheel(msg)
	}

	// Dismiss overlays on click-outside
	if m.state == stateContextMenu && msg.Button == tea.MouseButtonLeft {
		m.contextMenu = nil
		m.state = stateDefault
		return m, nil
	}
	if m.state == stateRepoSwitch && msg.Button == tea.MouseButtonLeft {
		m.pickerOverlay = nil
		m.state = stateDefault
		return m, nil
	}
	if m.state != stateDefault {
		return m, nil
	}

	// Right-click: show context menu
	if msg.Button == tea.MouseButtonRight {
		return m.handleRightClick(msg)
	}

	// Only handle left clicks from here
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	// Zone-based click: repo switch button
	if zone.Get(ui.ZoneNavRepo).InBounds(msg) {
		m.state = stateRepoSwitch
		m.pickerOverlay = overlay.NewPickerOverlay("Switch repo", m.buildRepoPickerItems())
		return m, nil
	}

	// Zone-based click: search box
	if zone.Get(ui.ZoneNavSearch).InBounds(msg) {
		m.setFocusSlot(slotNav)
		m.nav.ActivateSearch()
		m.state = stateSearch
		return m, nil
	}

	// Zone-based click: tab headers
	for i, zoneID := range ui.TabZoneIDs {
		if zone.Get(zoneID).InBounds(msg) {
			m.setFocusSlot(slotAgent + i)
			m.tabbedWindow.SetActiveTab(i)
			m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
			return m, m.instanceChanged()
		}
	}

	// Zone-based click: nav panel rows
	if zone.Get(ui.ZoneNavPanel).InBounds(msg) {
		m.setFocusSlot(slotNav)
		for i := range m.nav.RowCount() {
			if zone.Get(ui.NavRowZoneID(i)).InBounds(msg) {
				m.tabbedWindow.ClearDocumentMode()
				m.nav.ClickItem(i)
				return m, m.instanceChanged()
			}
		}
		return m, nil
	}

	// Click in tabbed window area (not on tab headers) — focus the active tab
	m.setFocusSlot(slotAgent + m.tabbedWindow.GetActiveTab())
	return m, nil
}
```

Note: `m.nav.RowCount()` — we need to expose the number of rows from NavigationPanel. Add a simple method: `func (n *NavigationPanel) RowCount() int { return len(n.rows) }`.

**Step 4: Implement handleFocusModeMouse**

Add a new method:

```go
// handleFocusModeMouse routes mouse events during agent focus mode.
// Clicks inside the agent pane are forwarded to the PTY as SGR sequences.
// Clicks outside exit focus mode and fall through to normal handling.
// Scroll wheel is position-aware: inside pane → PTY, outside → kasmos UI.
func (m *home) handleFocusModeMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	agentZone := zone.Get(ui.ZoneAgentPane)
	inAgent := agentZone.InBounds(msg)

	// Scroll wheel: position-aware routing
	if msg.IsWheel() {
		if inAgent && m.previewTerminal != nil {
			// Forward scroll to PTY
			data := mouseToSGR(msg, agentZone.StartX, agentZone.StartY)
			if data != nil {
				_ = m.previewTerminal.SendKey(data)
			}
			return m, nil
		}
		// Scroll outside agent pane: route to kasmos UI without exiting focus
		return m.handleScrollWheel(msg)
	}

	// Click/motion inside agent pane: forward to PTY
	if inAgent && m.previewTerminal != nil {
		data := mouseToSGR(msg, agentZone.StartX, agentZone.StartY)
		if data != nil {
			_ = m.previewTerminal.SendKey(data)
		}
		return m, nil
	}

	// Click outside agent pane: exit focus, then handle normally
	if msg.Action == tea.MouseActionPress {
		m.exitFocusMode()
		return m.handleMouse(msg) // re-enter for normal routing
	}

	return m, nil
}
```

**Step 5: Extract handleScrollWheel helper**

```go
// handleScrollWheel routes scroll wheel events to the appropriate pane.
func (m *home) handleScrollWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.tabbedWindow.ContentScrollUp()
	case tea.MouseButtonWheelDown:
		m.tabbedWindow.ContentScrollDown()
	}
	return m, nil
}
```

**Step 6: Rewrite handleRightClick with zones**

```go
func (m *home) handleRightClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Right-click in nav panel: select the clicked row, show context menu
	if zone.Get(ui.ZoneNavPanel).InBounds(msg) {
		for i := range m.nav.RowCount() {
			if zone.Get(ui.NavRowZoneID(i)).InBounds(msg) {
				m.nav.ClickItem(i)
				break
			}
		}
		if planFile := m.nav.GetSelectedPlanFile(); planFile != "" {
			return m.openPlanContextMenu()
		}
		return m, nil
	}
	return m, nil
}
```

**Step 7: Add RowCount to NavigationPanel**

In `ui/navigation_panel.go`, add:

```go
// RowCount returns the number of rows in the navigation panel.
func (n *NavigationPanel) RowCount() int { return len(n.rows) }
```

**Step 8: Run all tests**

Run: `go test ./app/... -v && go test ./ui/... -v`
Expected: PASS

**Step 9: Commit**

```bash
git add app/app_input.go ui/navigation_panel.go
git commit -m "feat: rewrite mouse handler with bubblezone hit detection and focus-mode routing"
```

---

### Task 4: Wire Up Mouse Forwarding in Focus Mode

**Files:**
- Modify: `app/app_input.go` (handleFocusModeMouse — if not already complete from Task 3)
- Modify: `app/app.go:57` (enable mouse motion events for drag support)

**Step 1: Write test for SGR forwarding to PTY**

Add to `app/mouse_sgr_test.go` or `app/app_mouse_test.go`:

```go
func TestMouseToSGR_CoordinateTranslation(t *testing.T) {
	// Simulate agent pane at offset (30, 4) — typical nav width + status bar
	msg := tea.MouseMsg{
		X:      35,
		Y:      10,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	got := mouseToSGR(msg, 30, 4)
	// Relative coords: (35-30+1, 10-4+1) = (6, 7)
	assert.Equal(t, "\x1b[<0;6;7M", string(got))
}
```

**Step 2: Run test**

Run: `go test ./app/ -run TestMouseToSGR_CoordinateTranslation -v`
Expected: PASS (SGR encoder already handles this from Task 1)

**Step 3: Enable mouse motion reporting**

In `app/app.go`, find where bubbletea options are set (the `tea.NewProgram` call). Add `tea.WithMouseAllMotion()` to enable motion events (needed for drag forwarding). Check current mouse option:

```bash
rg 'WithMouse' app/app.go
```

If it's `tea.WithMouseCellMotion()`, change to `tea.WithMouseAllMotion()`. If no mouse option is set, add `tea.WithMouseAllMotion()`.

Note: `WithMouseAllMotion` enables all motion tracking (not just button-held). This is needed for full SGR support. If there are performance concerns, we can gate this — only enable all-motion when in focus mode. But bubbletea doesn't support toggling mouse modes mid-session, so `WithMouseAllMotion` at startup is the pragmatic choice. The extra motion events are just ignored when not in focus mode (the `msg.Action != tea.MouseActionPress` guard at line 57 filters them).

**Step 4: Verify full mouse flow manually**

This is a manual test step:
1. Start kasmos with an agent running
2. Select the agent, press the focus key to enter focus mode
3. Click within the agent pane — verify the agent's TUI responds to the click
4. Scroll within the agent pane — verify the agent's TUI scrolls
5. Click outside the agent pane (on nav panel) — verify focus exits and the nav item is selected
6. Scroll outside the agent pane while in focus — verify kasmos UI scrolls without exiting focus

**Step 5: Run all tests**

Run: `go test ./... -v 2>&1 | tail -20`
Expected: all PASS

**Step 6: Commit**

```bash
git add app/app.go app/app_input.go
git commit -m "feat: enable SGR mouse forwarding to PTY in focus mode"
```

---

### Task 5: Cleanup and Backward Compatibility

**Files:**
- Modify: `ui/navigation_panel.go` (remove old `ZoneRepoSwitch` constant)
- Modify: `app/app_input.go` (remove old positional math references)
- Modify: `app/app.go` (optionally remove `navWidth`/`tabsWidth` if unused)

**Step 1: Remove deprecated ZoneRepoSwitch**

In `ui/navigation_panel.go`, the old constant `ZoneRepoSwitch = "repo-switch"` should be removed. All references now use `ZoneNavRepo` from `ui/zones.go`. Search for any remaining references:

```bash
rg 'ZoneRepoSwitch' --include='*.go'
```

Update any remaining references to use `ui.ZoneNavRepo`.

**Step 2: Clean up unused layout fields**

Check if `m.navWidth` and `m.tabsWidth` are still referenced anywhere after the zone rewrite:

```bash
rg 'm\.navWidth|m\.tabsWidth' --include='*.go'
```

If they're only used in `handleMouse` (which now uses zones), remove them from the `home` struct and from `updateHandleWindowSizeEvent`. If they're used elsewhere (e.g., overlay positioning), keep them.

**Step 3: Remove HandleTabClick from TabbedWindow (if not done in Task 2)**

Verify it's been removed:

```bash
rg 'HandleTabClick' --include='*.go'
```

Remove any remaining references.

**Step 4: Run full test suite**

Run: `go test ./... -v 2>&1 | tail -30`
Expected: all PASS

Run: `go build ./...`
Expected: clean build

**Step 5: Commit**

```bash
git add -A
git commit -m "refactor: remove deprecated zone constants and positional mouse math"
```
