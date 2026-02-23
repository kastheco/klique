# Bubblezone Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace all coordinate-based mouse hit-testing with bubblezone zones, and enable click-outside-to-exit from focus mode.

**Architecture:** Each clickable UI element gets wrapped with `zone.Mark(id, rendered)` in its View/String method. The mouse handler (`handleMouse`) switches from column-boundary checks and row-offset arithmetic to `zone.Get(id).InBounds(msg)` lookups. Focus mode escape triggers when a left-click lands outside the `tab-content` zone.

**Tech Stack:** `github.com/lrstanley/bubblezone` v1.0.0 (already imported), bubbletea mouse events

---

### Task 1: Add Zone ID Constants

**Files:**
- Modify: `ui/consts.go`

**Step 1: Add zone ID constants**

Add these constants to `ui/consts.go` after the existing banner code:

```go
// Zone IDs for bubblezone mouse hit-testing.
const (
	ZoneSidebarSearch = "sidebar-search"
	ZoneSidebarRow    = "sidebar-row-" // append visible index: "sidebar-row-0", "sidebar-row-1", ...
	ZoneTabAgent      = "tab-agent"
	ZoneTabDiff       = "tab-diff"
	ZoneTabGit        = "tab-git"
	ZoneTabContent    = "tab-content"
	ZoneListTabAll    = "list-tab-all"
	ZoneListTabActive = "list-tab-active"
	ZoneListItem      = "list-item-" // append visible index: "list-item-0", "list-item-1", ...
)
```

Note: `ZoneRepoSwitch` already exists in `ui/sidebar.go` — leave it there.

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: clean build

**Step 3: Commit**

```
feat(ui): add bubblezone ID constants for mouse zones
```

---

### Task 2: Zone-Mark Sidebar Elements

**Files:**
- Modify: `ui/sidebar.go`

**Step 1: Import fmt (if not already imported)**

`ui/sidebar.go` already imports `fmt` — verify and skip if present.

**Step 2: Zone-mark the search bar**

In `Sidebar.String()` (~line 1001-1014), wrap the search bar output with `zone.Mark`. Replace:

```go
	if s.searchActive {
		searchText := s.searchQuery
		if searchText == "" {
			searchText = " "
		}
		b.WriteString(searchActiveBarStyle.Width(searchWidth).Render(searchText))
	} else {
		b.WriteString(searchBarStyle.Width(searchWidth).Render("\uf002 search"))
	}
```

With:

```go
	if s.searchActive {
		searchText := s.searchQuery
		if searchText == "" {
			searchText = " "
		}
		b.WriteString(zone.Mark(ZoneSidebarSearch, searchActiveBarStyle.Width(searchWidth).Render(searchText)))
	} else {
		b.WriteString(zone.Mark(ZoneSidebarSearch, searchBarStyle.Width(searchWidth).Render("\uf002 search")))
	}
```

**Step 3: Zone-mark tree rows**

In `renderTreeRows()` (~line 801-826), wrap each rendered row with `zone.Mark`. The loop currently writes styled rows to the builder. After the selection styling is applied (the `if i == s.selectedIdx` block), wrap the result. Replace:

```go
		// Apply selection styling
		if i == s.selectedIdx && s.focused {
			b.WriteString(selectedTopicStyle.Width(itemWidth).Render(line))
		} else if i == s.selectedIdx && !s.focused {
			b.WriteString(activeTopicStyle.Width(itemWidth).Render(line))
		} else {
			b.WriteString(topicItemStyle.Width(itemWidth).Render(line))
		}
		b.WriteString("\n")
```

With:

```go
		// Apply selection styling
		var styledLine string
		if i == s.selectedIdx && s.focused {
			styledLine = selectedTopicStyle.Width(itemWidth).Render(line)
		} else if i == s.selectedIdx && !s.focused {
			styledLine = activeTopicStyle.Width(itemWidth).Render(line)
		} else {
			styledLine = topicItemStyle.Width(itemWidth).Render(line)
		}
		b.WriteString(zone.Mark(fmt.Sprintf("%s%d", ZoneSidebarRow, i), styledLine))
		b.WriteString("\n")
```

**Step 4: Zone-mark flat-mode sidebar items (legacy path)**

In `Sidebar.String()` (~line 1025-1098), the flat-mode rendering loop also needs zone marking. For each non-section item, wrap the final styled output. After the selection styling block for flat-mode items (around line 1099-1110 area), apply the same pattern: wrap with `zone.Mark(fmt.Sprintf("%s%d", ZoneSidebarRow, i), styledLine)`.

Find the flat-mode item rendering (the `else` branch of `if s.useTreeMode`). Each item gets selection styling applied. Wrap those outputs with zone marks using the loop index `i`.

**Step 5: Verify it compiles and the sidebar renders correctly**

Run: `go build ./...`
Run: `go test ./ui/... -v -run TestSidebar`
Expected: clean build, tests pass

**Step 6: Commit**

```
feat(ui): zone-mark sidebar search bar and tree rows
```

---

### Task 3: Zone-Mark Tab Headers and Content Area

**Files:**
- Modify: `ui/tabbed_window.go`

**Step 1: Add bubblezone import**

Add `zone "github.com/lrstanley/bubblezone"` to the imports in `ui/tabbed_window.go`.

**Step 2: Zone-mark each tab header**

In `TabbedWindow.String()` (~line 361-398), each tab is rendered and appended to `renderedTabs`. Wrap each tab's rendered output with its zone ID. The tab names are `Agent`, `Diff`, `Git` at indices 0, 1, 2.

Add a zone ID lookup slice before the loop:

```go
	tabZoneIDs := []string{ZoneTabAgent, ZoneTabDiff, ZoneTabGit}
```

Then in the loop, where each tab is appended to `renderedTabs`, wrap with zone.Mark. Replace the three `renderedTabs = append(...)` calls:

```go
		switch {
		case isActive && i == w.focusedTab && !w.focusMode:
			renderedTabs = append(renderedTabs, zone.Mark(tabZoneIDs[i], style.Render(GradientText(t, GradientStart, GradientEnd))))
		case isActive:
			renderedTabs = append(renderedTabs, zone.Mark(tabZoneIDs[i], style.Render(lipgloss.NewStyle().Foreground(ColorText).Render(t))))
		default:
			renderedTabs = append(renderedTabs, zone.Mark(tabZoneIDs[i], style.Render(lipgloss.NewStyle().Foreground(ColorMuted).Render(t))))
		}
```

**Step 3: Zone-mark the content area**

After the window content is rendered (~line 418-421), wrap the `window` variable with `zone.Mark(ZoneTabContent, ...)`. Replace:

```go
	window := ws.Render(
		lipgloss.Place(
			innerWidth, w.height-2-ws.GetVerticalFrameSize()-tabHeight,
			lipgloss.Left, lipgloss.Top, content))
```

With:

```go
	window := zone.Mark(ZoneTabContent, ws.Render(
		lipgloss.Place(
			innerWidth, w.height-2-ws.GetVerticalFrameSize()-tabHeight,
			lipgloss.Left, lipgloss.Top, content)))
```

**Step 4: Delete HandleTabClick method**

Delete the `HandleTabClick` method from `TabbedWindow` (~line 315-338). This coordinate-based hit-testing is replaced by zone lookups.

**Step 5: Verify it compiles**

Run: `go build ./...`
Expected: compile error in `app/app_input.go` referencing `HandleTabClick` — that's expected, fixed in Task 5.

**Step 6: Commit**

```
feat(ui): zone-mark tab headers and content area, remove HandleTabClick
```

---

### Task 4: Zone-Mark Instance List Elements

**Files:**
- Modify: `ui/list_renderer.go`
- Modify: `ui/list.go`

**Step 1: Add bubblezone import to list_renderer.go**

Add `zone "github.com/lrstanley/bubblezone"` to imports in `ui/list_renderer.go`.

**Step 2: Zone-mark filter tabs in List.String()**

In `List.String()` (~line 263-266), the filter tabs are rendered. Wrap each tab individually before joining. Replace:

```go
	tabs := lipgloss.JoinHorizontal(lipgloss.Bottom,
		allTab.Render(allTabText),
		activeTab.Render(activeTabText),
	)
```

With:

```go
	tabs := lipgloss.JoinHorizontal(lipgloss.Bottom,
		zone.Mark(ZoneListTabAll, allTab.Render(allTabText)),
		zone.Mark(ZoneListTabActive, activeTab.Render(activeTabText)),
	)
```

**Step 3: Zone-mark each instance row**

In `List.String()` (~line 292-297), the item rendering loop writes each item. Wrap each rendered item with its zone ID. Replace:

```go
	for i, item := range l.items {
		b.WriteString(l.renderer.Render(item, i == l.selectedIdx, l.focused, len(l.repos) > 1, i, l.IsHighlighted(item)))
		if i != len(l.items)-1 {
			b.WriteString("\n\n")
		}
	}
```

With:

```go
	for i, item := range l.items {
		rendered := l.renderer.Render(item, i == l.selectedIdx, l.focused, len(l.repos) > 1, i, l.IsHighlighted(item))
		b.WriteString(zone.Mark(fmt.Sprintf("%s%d", ZoneListItem, i), rendered))
		if i != len(l.items)-1 {
			b.WriteString("\n\n")
		}
	}
```

Add `"fmt"` to imports in `list_renderer.go` if not already present (it is).

**Step 4: Delete HandleTabClick method from List**

Delete the `HandleTabClick` method from `List` (~line 82-100 in `list.go`). This coordinate-based hit-testing is replaced by zone lookups.

**Step 5: Delete GetItemAtRow method from List**

Delete the `GetItemAtRow` method (~line 330-339 in `list_renderer.go`). Zone-based clicking gives us the exact item index directly — no row-to-index mapping needed.

**Step 6: Verify it compiles**

Run: `go build ./...`
Expected: compile errors in `app/app_input.go` — fixed in Task 5.

**Step 7: Commit**

```
feat(ui): zone-mark list filter tabs and instance rows, remove coordinate methods
```

---

### Task 5: Rewrite handleMouse with Zone Lookups

**Files:**
- Modify: `app/app_input.go`

This is the core task. Replace the entire `handleMouse` method with zone-based dispatch.

**Step 1: Write the new handleMouse**

Replace the entire `handleMouse` method (~line 50-163) with:

```go
// handleMouse processes mouse events for click and scroll interactions.
func (m *home) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Track hover state for the repo button on any mouse event
	repoHovered := zone.Get(ui.ZoneRepoSwitch).InBounds(msg)
	m.sidebar.SetRepoHovered(repoHovered)

	// --- Focus mode escape ---
	// Any left-click outside the content area exits focus mode and falls through
	// to normal click handling so the clicked element gets selected.
	if m.state == stateFocusAgent && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if !zone.Get(ui.ZoneTabContent).InBounds(msg) {
			m.exitFocusMode()
			// fall through to normal handling below
		} else {
			// Click inside content during focus mode — swallowed by embedded terminal
			return m, nil
		}
	}

	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	// --- Scroll wheel ---
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		selected := m.list.GetSelectedInstance()
		if selected != nil && selected.Status != session.Paused {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.tabbedWindow.ContentScrollUp()
			case tea.MouseButtonWheelDown:
				m.tabbedWindow.ContentScrollDown()
			}
		}
		return m, nil
	}

	// --- Overlay dismissal ---
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

	// --- Right-click ---
	if msg.Button == tea.MouseButtonRight {
		return m.handleRightClick(msg)
	}

	// Only handle left clicks from here
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	// --- Zone-based click dispatch ---

	// Repo switch button
	if zone.Get(ui.ZoneRepoSwitch).InBounds(msg) {
		m.state = stateRepoSwitch
		m.pickerOverlay = overlay.NewPickerOverlay("Switch repo", m.buildRepoPickerItems())
		return m, nil
	}

	// Sidebar search bar
	if zone.Get(ui.ZoneSidebarSearch).InBounds(msg) {
		m.setFocusSlot(slotSidebar)
		m.sidebar.ActivateSearch()
		m.state = stateSearch
		return m, nil
	}

	// Sidebar rows
	for i := 0; i < len(m.sidebar.GetRows()); i++ {
		if zone.Get(fmt.Sprintf("%s%d", ui.ZoneSidebarRow, i)).InBounds(msg) {
			m.setFocusSlot(slotSidebar)
			m.tabbedWindow.ClearDocumentMode()
			m.sidebar.ClickItem(i)
			m.filterInstancesByTopic()
			return m, m.instanceChanged()
		}
	}

	// Tab headers
	if zone.Get(ui.ZoneTabAgent).InBounds(msg) {
		m.setFocusSlot(slotAgent)
		m.menu.SetInDiffTab(false)
		return m, m.instanceChanged()
	}
	if zone.Get(ui.ZoneTabDiff).InBounds(msg) {
		m.setFocusSlot(slotDiff)
		m.menu.SetInDiffTab(true)
		return m, m.instanceChanged()
	}
	if zone.Get(ui.ZoneTabGit).InBounds(msg) {
		wasGitTab := m.tabbedWindow.IsInGitTab()
		m.setFocusSlot(slotGit)
		if !wasGitTab {
			cmd := m.spawnGitTab()
			return m, tea.Batch(m.instanceChanged(), cmd)
		}
		return m, m.instanceChanged()
	}

	// Tab content area (click to focus whichever center tab is visible)
	if zone.Get(ui.ZoneTabContent).InBounds(msg) {
		m.setFocusSlot(slotAgent + m.tabbedWindow.GetActiveTab())
		return m, nil
	}

	// List filter tabs
	if zone.Get(ui.ZoneListTabAll).InBounds(msg) {
		m.setFocusSlot(slotList)
		m.list.SetStatusFilter(ui.StatusFilterAll)
		return m, m.instanceChanged()
	}
	if zone.Get(ui.ZoneListTabActive).InBounds(msg) {
		m.setFocusSlot(slotList)
		m.list.SetStatusFilter(ui.StatusFilterActive)
		return m, m.instanceChanged()
	}

	// Instance list items
	for i := 0; i < m.list.NumInstances(); i++ {
		if zone.Get(fmt.Sprintf("%s%d", ui.ZoneListItem, i)).InBounds(msg) {
			m.setFocusSlot(slotList)
			m.tabbedWindow.ClearDocumentMode()
			m.list.SetSelectedInstance(i)
			return m, m.instanceChanged()
		}
	}

	return m, nil
}
```

**Step 2: Update handleRightClick signature**

The old `handleRightClick` takes `(x, y, contentY int)`. The new version receives the mouse message directly for zone-based hit testing. Change the signature and update the body:

```go
// handleRightClick builds and shows a context menu based on what was right-clicked.
func (m *home) handleRightClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	x, y := msg.X, msg.Y

	// Right-click in sidebar
	for i := 0; i < len(m.sidebar.GetRows()); i++ {
		if zone.Get(fmt.Sprintf("%s%d", ui.ZoneSidebarRow, i)).InBounds(msg) {
			m.sidebar.ClickItem(i)
			m.filterInstancesByTopic()
			// Plan header: show plan context menu
			if planFile := m.sidebar.GetSelectedPlanFile(); planFile != "" {
				return m.openPlanContextMenu()
			}
			// Topic header: show topic context menu
			if m.sidebar.IsSelectedTopicHeader() {
				return m.openTopicContextMenu()
			}
			return m, nil
		}
	}

	// Right-click in instance list
	for i := 0; i < m.list.NumInstances(); i++ {
		if zone.Get(fmt.Sprintf("%s%d", ui.ZoneListItem, i)).InBounds(msg) {
			m.list.SetSelectedInstance(i)
			selected := m.list.GetSelectedInstance()
			if selected == nil {
				return m, nil
			}
			items := []overlay.ContextMenuItem{
				{Label: "Open", Action: "open_instance"},
				{Label: "Kill", Action: "kill_instance"},
			}
			if selected.Status == session.Paused {
				items = append(items, overlay.ContextMenuItem{Label: "Resume", Action: "resume_instance"})
			} else {
				items = append(items, overlay.ContextMenuItem{Label: "Pause", Action: "pause_instance"})
			}
			if selected.Started() && selected.Status != session.Paused {
				items = append(items, overlay.ContextMenuItem{Label: "Focus agent", Action: "send_prompt_instance"})
			}
			items = append(items, overlay.ContextMenuItem{Label: "Rename", Action: "rename_instance"})
			items = append(items, overlay.ContextMenuItem{Label: "Push branch", Action: "push_instance"})
			items = append(items, overlay.ContextMenuItem{Label: "Create PR", Action: "create_pr_instance"})
			items = append(items, overlay.ContextMenuItem{Label: "Copy worktree path", Action: "copy_worktree_path"})
			items = append(items, overlay.ContextMenuItem{Label: "Copy branch name", Action: "copy_branch_name"})
			m.contextMenu = overlay.NewContextMenu(x, y, items)
			m.state = stateContextMenu
			return m, nil
		}
	}

	return m, nil
}
```

**Step 3: Add GetRows method to Sidebar**

The new `handleMouse` needs to know how many sidebar rows exist for the zone loop. Add to `ui/sidebar.go`:

```go
// GetRows returns the current tree-mode rows for zone iteration.
func (s *Sidebar) GetRows() []sidebarRow {
	return s.rows
}
```

If `sidebarRow` is unexported (it is), expose the count instead:

```go
// RowCount returns the number of visible sidebar rows (for zone iteration).
func (s *Sidebar) RowCount() int {
	if s.useTreeMode {
		return len(s.rows)
	}
	return len(s.items)
}
```

Then use `m.sidebar.RowCount()` instead of `len(m.sidebar.GetRows())` in the handleMouse loops.

**Step 4: Remove unused imports and fields**

In `app/app_input.go`, remove the `"github.com/mattn/go-runewidth"` import if it's no longer used (check — it was used for `runewidth.StringWidth` in the old code but may still be needed elsewhere in the file).

**Step 5: Verify it compiles and tests pass**

Run: `go build ./...`
Run: `go test ./... -v`
Expected: clean build, all tests pass

**Step 6: Commit**

```
feat(app): rewrite handleMouse with bubblezone zone dispatch

Replace all coordinate-based mouse hit-testing (column boundaries,
row offsets, width division) with zone.Get(id).InBounds(msg) lookups.
Add click-outside-to-exit from focus mode.
```

---

### Task 6: Update Tests

**Files:**
- Modify: `app/app_test.go` (if any mouse tests reference HandleTabClick or coordinate math)
- Modify: `ui/tabbed_window_test.go` (if HandleTabClick tests exist)

**Step 1: Search for tests referencing deleted methods**

Run: `grep -rn "HandleTabClick\|GetItemAtRow" app/ ui/ --include='*_test.go'`

If any tests reference these deleted methods, update or remove them. The zone-based approach doesn't need unit tests for coordinate math — the zones are self-verifying (if the zone is marked in View, InBounds works).

**Step 2: Run full test suite**

Run: `go test ./... -v`
Expected: all pass

**Step 3: Commit (if changes needed)**

```
test: update tests for bubblezone migration
```

---

### Task 7: Verify End-to-End

**Step 1: Build and run**

Run: `go build -o klique . && ./klique`

**Step 2: Manual verification checklist**

- [ ] Click sidebar search bar → activates search, focuses sidebar
- [ ] Click sidebar tree rows → selects item, focuses sidebar, filters instances
- [ ] Click Agent/Diff/Git tab headers → switches tab, sets focus
- [ ] Click center content area → focuses active center tab
- [ ] Click "All" / "Active" filter tabs → switches filter
- [ ] Click instance list items → selects instance, focuses list
- [ ] Right-click sidebar item → context menu appears
- [ ] Right-click instance → context menu appears at click position
- [ ] Enter focus mode (i key) → click sidebar → exits focus mode + selects sidebar item
- [ ] Enter focus mode → click instance list → exits focus mode + selects instance
- [ ] Enter focus mode → click inside content area → nothing happens (swallowed)
- [ ] Scroll wheel → scrolls content (unchanged)
- [ ] Repo switch button → opens picker (unchanged, already zone-based)

**Step 3: Commit any fixes**

```
fix: address issues found in bubblezone end-to-end testing
```
