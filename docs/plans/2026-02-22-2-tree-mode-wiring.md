# Tree-Mode Wiring & Integration

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove `DisableTreeMode()`, wire the highlight filter to sidebar selection, and verify end-to-end tree-mode behavior.

**Architecture:** This plan connects the three parallel pieces (renderer, navigation, highlight-boost) into a working whole. It removes the `DisableTreeMode()` gate, replaces `filterInstancesByTopic()` with highlight-filter dispatching, and ensures context menus work from tree selection.

**Tech Stack:** Go, bubbletea

**Design doc:** `docs/plans/2026-02-22-tree-mode-sidebar-design.md`

**Dependencies:** Plans 1a (tree-renderer), 1b (tree-navigation), 1c (highlight-boost-list) must all be complete before starting this plan.

---

### Task 1: Remove DisableTreeMode() call

**Files:**
- Modify: `app/app_state.go`

**Step 1: Remove the DisableTreeMode call and its comment**

In `updateSidebarPlans()` (around line 520-526), remove these lines:

```go
	// NOTE: Tree mode navigation is disabled until String() is updated to
	// render s.rows. Currently String() renders s.items (flat list) while
	// tree mode makes Up()/Down() navigate s.rows, causing an index mismatch
	// where the visual highlight doesn't match the logical selection.
	m.sidebar.DisableTreeMode()
```

**Step 2: Build**

Run: `go build ./...`
Expected: Clean build (DisableTreeMode method still exists but is no longer called)

**Step 3: Commit**

```bash
git add app/app_state.go
git commit -m "feat(app): enable tree-mode sidebar by removing DisableTreeMode gate"
```

---

### Task 2: Replace filterInstancesByTopic with highlight dispatching

**Files:**
- Modify: `app/app_state.go`

**Step 1: Rewrite filterInstancesByTopic to dispatch highlight filters**

Replace the existing `filterInstancesByTopic` method:

```go
// filterInstancesByTopic updates the instance list highlight filter based on the
// current sidebar selection. In tree mode, this highlights matching instances and
// boosts them to the top. In flat mode, it falls back to the existing SetFilter behavior.
func (m *home) filterInstancesByTopic() {
	selectedID := m.sidebar.GetSelectedID()

	if !m.sidebar.IsTreeMode() {
		// Flat mode fallback
		switch {
		case selectedID == SidebarAll:
			m.list.SetFilter("")
		case selectedID == SidebarUngrouped:
			m.list.SetFilter(SidebarUngrouped)
		case strings.HasPrefix(selectedID, SidebarPlanPrefix):
			m.list.SetFilter("")
		default:
			m.list.SetFilter(selectedID)
		}
		return
	}

	// Tree mode: use highlight filter
	switch {
	case strings.HasPrefix(selectedID, SidebarPlanPrefix):
		planFile := selectedID[len(SidebarPlanPrefix):]
		m.list.SetHighlightFilter("plan", planFile)
	case strings.HasPrefix(selectedID, SidebarTopicPrefix):
		topicName := selectedID[len(SidebarTopicPrefix):]
		m.list.SetHighlightFilter("topic", topicName)
	case strings.HasPrefix(selectedID, SidebarPlanStagePrefix):
		// Stage selected — highlight parent plan
		planFile := m.sidebar.GetSelectedPlanFile()
		if planFile != "" {
			m.list.SetHighlightFilter("plan", planFile)
		} else {
			m.list.SetHighlightFilter("", "")
		}
	default:
		m.list.SetHighlightFilter("", "")
	}
}
```

**Step 2: Ensure GetSelectedPlanFile works for stage rows**

Check that `GetSelectedPlanFile()` in `ui/sidebar.go` returns the plan file even when a stage row is selected. Looking at the existing code:

```go
func (s *Sidebar) GetSelectedPlanFile() string {
	id := s.GetSelectedID()
	if strings.HasPrefix(id, SidebarPlanPrefix) {
		return id[len(SidebarPlanPrefix):]
	}
	return ""
}
```

Stage row IDs start with `SidebarPlanStagePrefix`, not `SidebarPlanPrefix`, so this returns "". We need to also handle stage rows. Modify:

```go
func (s *Sidebar) GetSelectedPlanFile() string {
	id := s.GetSelectedID()
	if strings.HasPrefix(id, SidebarPlanPrefix) {
		return id[len(SidebarPlanPrefix):]
	}
	if strings.HasPrefix(id, SidebarPlanStagePrefix) {
		// Stage ID format: "__plan_stage__<planFile>::<stage>"
		rest := id[len(SidebarPlanStagePrefix):]
		if idx := strings.Index(rest, "::"); idx >= 0 {
			return rest[:idx]
		}
	}
	return ""
}
```

**Step 3: Build and test**

Run: `go build ./...`
Expected: Clean build

Run: `go test ./... -count=1`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add app/app_state.go ui/sidebar.go
git commit -m "feat(app): wire highlight filter to tree-mode sidebar selection"
```

---

### Task 3: Update context menus for tree-mode actions

**Files:**
- Modify: `app/app_actions.go`

**Step 1: Review openPlanContextMenu**

The existing `openPlanContextMenu` (around line 334) gets the plan file from `GetSelectedPlanFile()` which now works for both plan rows and stage rows. Verify the context menu items still make sense. The current items are:

```go
items := []overlay.ContextMenuItem{
	{Label: "Start plan", Action: "start_plan"},
	{Label: "View plan", Action: "view_plan"},
	{Label: "Push branch", Action: "push_plan_branch"},
	{Label: "Create PR", Action: "create_plan_pr"},
	{Label: "Mark done", Action: "mark_plan_done"},
	{Label: "Cancel plan", Action: "cancel_plan"},
}
```

These are fine for the tree mode. No changes needed to the context menu items themselves.

**Step 2: Update context menu positioning**

The context menu position calculation uses `m.sidebar.GetSelectedIdx()` which returns the raw index. Verify this produces a reasonable y-position in tree mode:

```go
x := m.sidebarWidth
y := 1 + 4 + m.sidebar.GetSelectedIdx()
```

This should work since `GetSelectedIdx()` returns the row index in `s.rows`, and each row is one line. The `1 + 4` accounts for border + search bar. This is acceptable.

**Step 3: Review topic context menu**

The `openTopicContextMenu` should work as-is since `GetSelectedTopicName()` already branches on tree mode.

**Step 4: Build and test**

Run: `go build ./...`
Expected: Clean build

Run: `go test ./... -count=1`
Expected: ALL PASS

**Step 5: Commit (only if changes were needed)**

```bash
git add app/app_actions.go
git commit -m "fix(app): adjust context menu positioning for tree-mode sidebar"
```

---

### Task 4: Clean up dead code

**Files:**
- Modify: `ui/sidebar.go`
- Modify: `app/app_state.go`

**Step 1: Remove DisableTreeMode method if unused**

Check: `rg 'DisableTreeMode' --include='*.go'`

If the only reference is the method definition itself, remove the method:

```go
// DisableTreeMode forces flat-mode navigation (using s.items) even if tree
// data has been loaded. Use this until String() is updated to render s.rows.
func (s *Sidebar) DisableTreeMode() {
	s.useTreeMode = false
}
```

**Step 2: Remove dead topic-filter code in rebuildFilteredItems**

The old comment `// Topics are now plan-state-based; all instances are shown regardless of filter.` and the dead code path `topicFiltered = l.allItems` can be cleaned up since highlight-boost replaces it.

**Step 3: Build and test**

Run: `go build ./...`
Expected: Clean build

Run: `go test ./... -count=1`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add ui/sidebar.go ui/list.go app/app_state.go
git commit -m "refactor(ui): remove dead DisableTreeMode and topic-filter code"
```

---

### Task 5: Integration smoke test

**Files:**
- No file changes — manual verification

**Step 1: Build and run klique**

Run: `go build -o klique . && ./klique`

**Step 2: Verify tree mode rendering**

- Plans should display as a tree in the left sidebar
- Topics should show with ▸/▾ chevrons
- Plans should show with status glyphs (○/●/◉)
- Expanding a plan with Space or → should show stage rows

**Step 3: Verify navigation**

- ↑/↓ moves between visible rows
- → expands collapsed topics/plans, moves to first child if expanded
- ← collapses expanded nodes, moves to parent if collapsed
- Space toggles expand/collapse
- Enter opens context menu on plans and topics
- Enter/Space on stage rows are no-ops

**Step 4: Verify highlight-boost**

- Selecting a plan should highlight its instances and bump them to top
- Selecting a topic should highlight all instances in that topic
- Non-matching instances should appear dimmed below
- When nothing specific is selected, all instances render normally

**Step 5: Verify existing functionality**

- Plan context menu actions (Start plan, View plan, etc.) work
- Topic context menu actions work
- Status filters (All/Active) still work
- Sort modes still work within each partition
- Search still works
