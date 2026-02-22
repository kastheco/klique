# Tree-Mode Sidebar Navigation

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `Left()`/`Right()` tree-traversal methods to Sidebar and rebind Space/Enter/←/→ for tree-mode interaction.

**Architecture:** New `Left()` and `Right()` methods on Sidebar handle parent/child traversal. `app_input.go` intercepts `KeyLeft`/`KeyRight` when sidebar is focused and in tree mode, delegating to these methods instead of the panel-focus-cycling behavior. Space toggles expand/collapse (already partially wired). Enter opens context menus. Stage rows are not actionable.

**Tech Stack:** Go, bubbletea

**Design doc:** `docs/plans/2026-02-22-tree-mode-sidebar-design.md`

---

### Task 1: Write failing tests for Left() and Right()

**Files:**
- Modify: `ui/sidebar_test.go`

**Step 1: Write tests**

```go
func TestSidebarRight_ExpandsCollapsedTopic(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	// Topic starts collapsed
	s.SelectByID(SidebarTopicPrefix + "auth")
	assert.False(t, s.HasRowID(SidebarPlanPrefix+"tokens.md"))

	s.Right()
	// Should expand topic
	assert.True(t, s.HasRowID(SidebarPlanPrefix+"tokens.md"))
}

func TestSidebarRight_MovesToFirstChildWhenExpanded(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	s.SelectByID(SidebarTopicPrefix + "auth")
	s.ToggleSelectedExpand() // expand
	s.SelectByID(SidebarTopicPrefix + "auth") // re-select topic

	s.Right()
	// Should move to first child plan
	assert.Equal(t, SidebarPlanPrefix+"tokens.md", s.GetSelectedID())
}

func TestSidebarLeft_CollapsesExpandedTopic(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	s.SelectByID(SidebarTopicPrefix + "auth")
	s.ToggleSelectedExpand() // expand
	s.SelectByID(SidebarTopicPrefix + "auth") // re-select topic

	s.Left()
	// Should collapse topic
	assert.False(t, s.HasRowID(SidebarPlanPrefix+"tokens.md"))
}

func TestSidebarLeft_MovesToParentFromPlan(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	s.SelectByID(SidebarTopicPrefix + "auth")
	s.ToggleSelectedExpand()
	s.SelectByID(SidebarPlanPrefix + "tokens.md")

	s.Left()
	// Should move to parent topic
	assert.Equal(t, SidebarTopicPrefix+"auth", s.GetSelectedID())
}

func TestSidebarLeft_MovesToParentPlanFromStage(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "in_progress"}},
		nil,
	)

	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()
	s.SelectByID(SidebarPlanStagePrefix + "fix.md::implement")

	s.Left()
	// Should move to parent plan
	assert.Equal(t, SidebarPlanPrefix+"fix.md", s.GetSelectedID())
}

func TestSidebarLeft_UngroupedPlanMovesUp(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{
			{Filename: "a.md", Status: "ready"},
			{Filename: "b.md", Status: "ready"},
		},
		nil,
	)

	s.Down() // select b.md
	s.Left()
	// Ungrouped plan with no parent topic — should just move up
	assert.Equal(t, SidebarPlanPrefix+"a.md", s.GetSelectedID())
}

func TestSidebarRight_NoopOnStage(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "in_progress"}},
		nil,
	)

	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()
	s.SelectByID(SidebarPlanStagePrefix + "fix.md::plan")
	before := s.GetSelectedID()

	s.Right()
	assert.Equal(t, before, s.GetSelectedID())
}

func TestSidebarRight_ExpandsPlan(t *testing.T) {
	s := NewSidebar()
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "in_progress"}},
		nil,
	)

	s.SelectByID(SidebarPlanPrefix + "fix.md")
	assert.False(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::plan"))

	s.Right()
	// Should expand plan to show stages
	assert.True(t, s.HasRowID(SidebarPlanStagePrefix+"fix.md::plan"))
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/ -run 'TestSidebarRight|TestSidebarLeft' -v`
Expected: FAIL — `Left()` and `Right()` methods don't exist yet.

**Step 3: Commit**

```bash
git add ui/sidebar_test.go
git commit -m "test(ui): add failing tests for tree-mode Left/Right navigation"
```

---

### Task 2: Implement Left() and Right() on Sidebar

**Files:**
- Modify: `ui/sidebar.go`

**Step 1: Add Right() method**

Add after the existing `Down()` method:

```go
// Right expands the selected node if collapsed, or moves to its first child if already expanded.
// No-op on stage rows and in flat mode.
func (s *Sidebar) Right() {
	if !s.useTreeMode || s.selectedIdx >= len(s.rows) {
		return
	}
	row := s.rows[s.selectedIdx]
	switch row.Kind {
	case rowKindTopic:
		if row.Collapsed {
			s.ToggleSelectedExpand()
		} else {
			// Move to first child
			if s.selectedIdx+1 < len(s.rows) {
				s.selectedIdx++
			}
		}
	case rowKindPlan:
		if row.Collapsed {
			s.ToggleSelectedExpand()
		} else {
			// Move to first child stage
			if s.selectedIdx+1 < len(s.rows) && s.rows[s.selectedIdx+1].Kind == rowKindStage {
				s.selectedIdx++
			}
		}
	// Stage, History, Cancelled: no-op
	}
}
```

**Step 2: Add Left() method**

```go
// Left collapses the selected node if expanded, or moves to its parent.
// On ungrouped plans with no parent topic, behaves like Up().
func (s *Sidebar) Left() {
	if !s.useTreeMode || s.selectedIdx >= len(s.rows) {
		return
	}
	row := s.rows[s.selectedIdx]
	switch row.Kind {
	case rowKindTopic:
		if !row.Collapsed {
			s.ToggleSelectedExpand()
		} else if s.selectedIdx > 0 {
			s.selectedIdx--
		}
	case rowKindPlan:
		if !row.Collapsed {
			s.ToggleSelectedExpand()
			return
		}
		// Move to parent topic
		for i := s.selectedIdx - 1; i >= 0; i-- {
			if s.rows[i].Kind == rowKindTopic {
				s.selectedIdx = i
				return
			}
		}
		// No parent topic — move up
		if s.selectedIdx > 0 {
			s.selectedIdx--
		}
	case rowKindStage:
		// Move to parent plan
		for i := s.selectedIdx - 1; i >= 0; i-- {
			if s.rows[i].Kind == rowKindPlan {
				s.selectedIdx = i
				return
			}
		}
	case rowKindHistoryToggle, rowKindCancelled:
		if s.selectedIdx > 0 {
			s.selectedIdx--
		}
	}
}
```

**Step 3: Run tests**

Run: `go test ./ui/ -run 'TestSidebarRight|TestSidebarLeft' -v`
Expected: PASS

Run: `go test ./ui/ -v`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(ui): implement Left/Right tree-traversal for sidebar"
```

---

### Task 3: Rebind ←/→ in app_input.go for tree mode

**Files:**
- Modify: `app/app_input.go` (lines 1054-1089, the KeyLeft and KeyRight cases)

**Step 1: Intercept KeyLeft for tree mode**

Modify the `KeyLeft` case (line 1054) to check if sidebar is focused and in tree mode first:

```go
case keys.KeyLeft:
	if m.focusedPanel == 0 && m.sidebar.IsTreeMode() {
		m.sidebar.Left()
		m.filterInstancesByTopic()
		return m, nil
	}
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
		m.sidebarHidden = false
		m.setFocus(0)
		return m, tea.WindowSize()
	}
	if m.focusedPanel > 0 {
		m.setFocus(m.focusedPanel - 1)
	}
	return m, nil
```

**Step 2: Intercept KeyRight for tree mode**

Modify the `KeyRight` case (line 1075):

```go
case keys.KeyRight:
	if m.focusedPanel == 0 && m.sidebar.IsTreeMode() {
		m.sidebar.Right()
		m.filterInstancesByTopic()
		return m, nil
	}
	// Cycle right: sidebar(0) → preview(1) → list(2) → enter focus mode.
	if m.focusedPanel == 2 {
		if m.tabbedWindow.IsInGitTab() {
			return m, m.enterGitFocusMode()
		}
		selected := m.list.GetSelectedInstance()
		if selected != nil && selected.Started() && !selected.Paused() {
			return m, m.enterFocusMode()
		}
	} else {
		m.setFocus(m.focusedPanel + 1)
	}
	return m, nil
```

**Step 3: Add IsTreeMode() to Sidebar**

In `ui/sidebar.go`, add:

```go
// IsTreeMode returns true when the sidebar is rendering in tree mode.
func (s *Sidebar) IsTreeMode() bool {
	return s.useTreeMode
}
```

**Step 4: Make Enter on stages a no-op**

In `app_input.go`, the Enter handler (around line 991) already checks `GetSelectedPlanStage`. Modify it to skip if on a stage row:

```go
case keys.KeyEnter:
	if m.focusedPanel == 0 {
		// Stage rows are display-only — no action
		if _, _, isStage := m.sidebar.GetSelectedPlanStage(); isStage {
			return m, nil
		}
		// Plan header: open plan context menu
		if m.sidebar.IsSelectedPlanHeader() {
			return m.openPlanContextMenu()
		}
		// Topic header: open topic context menu
		if m.sidebar.IsSelectedTopicHeader() {
			return m.openTopicContextMenu()
		}
		// ... rest of existing Enter handling
	}
```

**Step 5: Make Space on stages a no-op**

The existing Space handler (line 885) already calls `ToggleSelectedExpand()` which returns false for stages (only topics and plans toggle). The current code falls through to `openContextMenu()` on false — which is correct for non-tree items but should be a no-op for stages in tree mode:

```go
case keys.KeySpace:
	if m.focusedPanel == 0 && m.sidebar.ToggleSelectedExpand() {
		return m, nil
	}
	// In tree mode, Space on non-expandable rows (stages) is a no-op
	if m.focusedPanel == 0 && m.sidebar.IsTreeMode() {
		return m, nil
	}
	return m.openContextMenu()
```

**Step 6: Build and test**

Run: `go build ./...`
Expected: Clean build

Run: `go test ./... -count=1`
Expected: ALL PASS

**Step 7: Commit**

```bash
git add ui/sidebar.go app/app_input.go
git commit -m "feat(app): rebind arrow keys and Space/Enter for tree-mode sidebar navigation"
```
