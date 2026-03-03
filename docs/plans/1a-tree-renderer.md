# Tree-Mode Sidebar Renderer

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a tree-mode rendering branch to `Sidebar.String()` that renders `s.rows` with proper indentation, chevrons, and status glyphs — using per-kind render functions and indent stored on the row struct.

**Architecture:** `sidebarRow` gets an `Indent int` field computed during `rebuildRows()`. `String()` branches on `s.useTreeMode`: true dispatches to `renderTreeRows()` which calls per-kind render functions; false keeps the existing flat loop. Each per-kind function returns a line string; the caller applies selection styling.

**Tech Stack:** Go, lipgloss, go-runewidth

**Design doc:** `docs/plans/2026-02-22-tree-mode-sidebar-design.md`

---

### Task 1: Add `Indent` field to `sidebarRow` and compute it in `rebuildRows`

**Files:**
- Modify: `ui/sidebar.go:95-109` (sidebarRow struct)
- Modify: `ui/sidebar.go:450-529` (rebuildRows)
- Modify: `ui/sidebar.go:532-555` (planStageRows)

**Step 1: Add `Indent` field to `sidebarRow`**

In the `sidebarRow` struct, add `Indent` after `Locked`:

```go
type sidebarRow struct {
	Kind      sidebarRowKind
	ID        string
	Label     string
	PlanFile  string
	Stage     string
	Collapsed bool
	// display flags
	HasRunning      bool
	HasNotification bool
	Count           int
	Done            bool // stage is completed
	Active          bool // stage is currently active
	Locked          bool // stage is not yet reachable
	Indent          int  // visual indent in characters (0, 2, or 4)
}
```

**Step 2: Update `rebuildRows` to set `Indent`**

In `rebuildRows()`, set `Indent` on every row:

- Ungrouped plans: `Indent: 0`
- Stage rows under ungrouped plans: need indent 2 — update `planStageRows` to accept an `indent int` parameter
- Topic headers: `Indent: 0`
- Plans under a topic: `Indent: 2`
- Stage rows under topic plans: indent 4 — pass `4` to `planStageRows`
- History toggle: `Indent: 0`
- Cancelled plans: `Indent: 0`

Change the `planStageRows` signature from:
```go
func planStageRows(p PlanDisplay) []sidebarRow {
```
to:
```go
func planStageRows(p PlanDisplay, indent int) []sidebarRow {
```

And set `Indent: indent` on each stage row it creates.

Update all call sites in `rebuildRows`:
- Ungrouped plan stages: `planStageRows(effective, 2)`
- Topic plan stages: `planStageRows(effective, 4)`

The full updated `rebuildRows`:

```go
func (s *Sidebar) rebuildRows() {
	rows := []sidebarRow{}

	// Ungrouped plans (shown at top level, always visible)
	for _, p := range s.treeUngrouped {
		effective := p
		effective.Status = s.effectivePlanStatus(p)
		rows = append(rows, sidebarRow{
			Kind:            rowKindPlan,
			ID:              SidebarPlanPrefix + p.Filename,
			Label:           planstate.DisplayName(p.Filename),
			PlanFile:        p.Filename,
			Collapsed:       !s.expandedPlans[p.Filename],
			HasRunning:      effective.Status == string(planstate.StatusInProgress),
			HasNotification: effective.Status == string(planstate.StatusReviewing),
			Indent:          0,
		})
		if s.expandedPlans[p.Filename] {
			rows = append(rows, planStageRows(effective, 2)...)
		}
	}

	// Topic headers (collapsed by default)
	for _, t := range s.treeTopics {
		// Aggregate status from child plans
		hasRunning := false
		hasNotification := false
		for _, p := range t.Plans {
			eff := s.effectivePlanStatus(p)
			if eff == string(planstate.StatusInProgress) {
				hasRunning = true
			}
			if eff == string(planstate.StatusReviewing) {
				hasNotification = true
			}
		}

		rows = append(rows, sidebarRow{
			Kind:            rowKindTopic,
			ID:              SidebarTopicPrefix + t.Name,
			Label:           t.Name,
			Collapsed:       !s.expandedTopics[t.Name],
			HasRunning:      hasRunning,
			HasNotification: hasNotification,
			Indent:          0,
		})
		if s.expandedTopics[t.Name] {
			for _, p := range t.Plans {
				effective := p
				effective.Status = s.effectivePlanStatus(p)
				rows = append(rows, sidebarRow{
					Kind:            rowKindPlan,
					ID:              SidebarPlanPrefix + p.Filename,
					Label:           planstate.DisplayName(p.Filename),
					PlanFile:        p.Filename,
					Collapsed:       !s.expandedPlans[p.Filename],
					HasRunning:      effective.Status == string(planstate.StatusInProgress),
					HasNotification: effective.Status == string(planstate.StatusReviewing),
					Indent:          2,
				})
				if s.expandedPlans[p.Filename] {
					rows = append(rows, planStageRows(effective, 4)...)
				}
			}
		}
	}

	// History toggle (if there are finished plans)
	if len(s.treeHistory) > 0 {
		rows = append(rows, sidebarRow{
			Kind:   rowKindHistoryToggle,
			ID:     SidebarPlanHistoryToggle,
			Label:  "History",
			Indent: 0,
		})
	}

	// Cancelled plans (shown at bottom with strikethrough)
	for _, p := range s.treeCancelled {
		rows = append(rows, sidebarRow{
			Kind:     rowKindCancelled,
			ID:       SidebarPlanPrefix + p.Filename,
			Label:    planstate.DisplayName(p.Filename),
			PlanFile: p.Filename,
			Indent:   0,
		})
	}

	s.rows = rows

	// Clamp selectedIdx
	if s.selectedIdx >= len(rows) {
		s.selectedIdx = len(rows) - 1
	}
	if s.selectedIdx < 0 {
		s.selectedIdx = 0
	}
}
```

The updated `planStageRows`:

```go
func planStageRows(p PlanDisplay, indent int) []sidebarRow {
	stages := []struct{ name, label string }{
		{"plan", "Plan"},
		{"implement", "Implement"},
		{"review", "Review"},
		{"finished", "Finished"},
	}
	rows := make([]sidebarRow, 0, 4)
	for _, st := range stages {
		done, active, locked := stageState(p.Status, st.name)
		rows = append(rows, sidebarRow{
			Kind:     rowKindStage,
			ID:       SidebarPlanStagePrefix + p.Filename + "::" + st.name,
			Label:    st.label,
			PlanFile: p.Filename,
			Stage:    st.name,
			Done:     done,
			Active:   active,
			Locked:   locked,
			Indent:   indent,
		})
	}
	return rows
}
```

**Step 3: Run existing tests to verify no regressions**

Run: `go test ./ui/ -v`
Expected: ALL PASS — no rendering tests exist yet, data model tests should still pass.

**Step 4: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(ui): add Indent field to sidebarRow and compute in rebuildRows"
```

---

### Task 2: Add tree-mode lipgloss styles

**Files:**
- Modify: `ui/sidebar.go:125-128` (after `sidebarCancelledStyle`)

**Step 1: Add new styles**

Add these after `sidebarCancelledStyle` (line 127):

```go
// Tree-mode styles

// stageCheckStyle is for completed stage indicators (✓).
var stageCheckStyle = lipgloss.NewStyle().Foreground(ColorFoam)

// stageActiveStyle is for the currently active stage indicator (▸).
var stageActiveStyle = lipgloss.NewStyle().Foreground(ColorIris)

// stageLockedStyle is for locked/unreachable stage indicators (○).
var stageLockedStyle = lipgloss.NewStyle().Foreground(ColorMuted)

// topicLabelStyle is for topic header labels in tree mode.
var topicLabelStyle = lipgloss.NewStyle().Foreground(ColorText).Bold(true)

// historyToggleStyle is for the history section divider.
var historyToggleStyle = lipgloss.NewStyle().Foreground(ColorMuted)
```

**Step 2: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(ui): add lipgloss styles for tree-mode sidebar rows"
```

---

### Task 3: Write failing tests for tree-mode rendering

**Files:**
- Modify: `ui/sidebar_test.go`

**Step 1: Add rendering tests**

Append these tests to `ui/sidebar_test.go`. They call `String()` and assert on rendered output. They will fail because `String()` doesn't render tree rows yet.

```go
func TestSidebarTreeRender_UngroupedPlan(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "bugfix.md", Status: "ready"}},
		nil,
	)

	output := s.String()
	// Should contain the plan name and ready glyph
	assert.Contains(t, output, "bugfix")
	assert.Contains(t, output, "○")
}

func TestSidebarTreeRender_TopicWithChevron(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "in_progress"},
			}},
		},
		nil, nil,
	)

	output := s.String()
	// Topic should be visible with collapsed chevron
	assert.Contains(t, output, "auth")
	assert.Contains(t, output, "▸")
}

func TestSidebarTreeRender_ExpandedTopicShowsPlans(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "in_progress"},
			}},
		},
		nil, nil,
	)

	// Expand topic
	s.SelectByID(SidebarTopicPrefix + "auth")
	s.ToggleSelectedExpand()

	output := s.String()
	assert.Contains(t, output, "auth")
	assert.Contains(t, output, "tokens")
	assert.Contains(t, output, "●") // in_progress glyph
}

func TestSidebarTreeRender_ExpandedPlanStages(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "fix.md", Status: "in_progress"}},
		nil,
	)

	// Expand plan to show stages
	s.SelectByID(SidebarPlanPrefix + "fix.md")
	s.ToggleSelectedExpand()

	output := s.String()
	assert.Contains(t, output, "Plan")
	assert.Contains(t, output, "Implement")
	assert.Contains(t, output, "Review")
	assert.Contains(t, output, "Finished")
}

func TestSidebarTreeRender_SelectedRowHighlighted(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetFocused(true)
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{
			{Filename: "a.md", Status: "ready"},
			{Filename: "b.md", Status: "ready"},
		},
		nil,
	)

	// Select second plan
	s.Down()
	output := s.String()
	// Both plans should be present
	assert.Contains(t, output, "a")
	assert.Contains(t, output, "b")
}

func TestSidebarTreeRender_HistoryToggle(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		nil,
		[]PlanDisplay{{Filename: "active.md", Status: "ready"}},
		[]PlanDisplay{{Filename: "old.md", Status: "done"}},
	)

	output := s.String()
	assert.Contains(t, output, "History")
}

func TestSidebarTreeRender_CancelledPlan(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		nil, nil, nil,
		[]PlanDisplay{{Filename: "dropped.md", Status: "cancelled"}},
	)

	output := s.String()
	assert.Contains(t, output, "dropped")
	assert.Contains(t, output, "✕")
}

func TestSidebarTreeRender_TopicAggregateStatus(t *testing.T) {
	s := NewSidebar()
	s.SetSize(40, 20)
	s.SetTopicsAndPlans(
		[]TopicDisplay{
			{Name: "auth", Plans: []PlanDisplay{
				{Filename: "tokens.md", Status: "in_progress"},
				{Filename: "session.md", Status: "ready"},
			}},
		},
		nil, nil,
	)

	output := s.String()
	// Topic should show running indicator because child is in_progress
	assert.Contains(t, output, "●")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/ -run TestSidebarTreeRender -v`
Expected: FAIL — `String()` renders flat `s.items` (which is empty/default when only tree data is set), so the tree content won't appear.

**Step 3: Commit**

```bash
git add ui/sidebar_test.go
git commit -m "test(ui): add failing tests for tree-mode sidebar rendering"
```

---

### Task 4: Implement per-kind render functions

**Files:**
- Modify: `ui/sidebar.go` (add new methods before `String()`)

**Step 1: Add the per-kind render functions**

Add these methods before the `String()` function (before line 662):

```go
// renderTopicRow renders a topic header row.
// Layout: [cursor 1][chevron 1][space 1][label...][gap...][status 2]
func (s *Sidebar) renderTopicRow(row sidebarRow, idx, contentWidth int) string {
	chevron := "▸"
	if !row.Collapsed {
		chevron = "▾"
	}

	cursor := " "
	if idx == s.selectedIdx {
		cursor = "▸"
	}

	// Aggregate status glyph
	var statusGlyph string
	var statusStyle lipgloss.Style
	if row.HasNotification {
		statusGlyph = "◉"
		statusStyle = sidebarNotifyStyle
	} else if row.HasRunning {
		statusGlyph = "●"
		statusStyle = sidebarRunningStyle
	}

	label := row.Label
	const fixedW = 3 // cursor(1) + chevron(1) + space(1)
	trailW := 0
	if statusGlyph != "" {
		trailW = 2 // " ●"
	}
	maxLabel := contentWidth - fixedW - trailW
	if maxLabel < 3 {
		maxLabel = 3
	}
	if runewidth.StringWidth(label) > maxLabel {
		label = runewidth.Truncate(label, maxLabel-1, "…")
	}

	text := topicLabelStyle.Render(label)
	usedW := fixedW + runewidth.StringWidth(label) + trailW
	gap := contentWidth - usedW
	if gap < 0 {
		gap = 0
	}

	trail := ""
	if statusGlyph != "" {
		trail = " " + statusStyle.Render(statusGlyph)
	}
	return cursor + chevron + " " + text + strings.Repeat(" ", gap) + trail
}

// renderPlanRow renders a plan row.
// Layout: [indent][cursor 1][label...][gap...][chevron 1][space 1][status 1]
func (s *Sidebar) renderPlanRow(row sidebarRow, idx, contentWidth int) string {
	indent := strings.Repeat(" ", row.Indent)

	chevron := "▸"
	if !row.Collapsed {
		chevron = "▾"
	}

	cursor := " "
	if idx == s.selectedIdx {
		cursor = "▸"
	}

	// Status glyph
	var statusGlyph string
	var statusStyle lipgloss.Style
	switch {
	case row.HasNotification:
		statusGlyph = "◉"
		statusStyle = sidebarNotifyStyle
	case row.HasRunning:
		statusGlyph = "●"
		statusStyle = sidebarRunningStyle
	default:
		statusGlyph = "○"
		statusStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	}

	label := row.Label
	indentW := row.Indent
	const cursorW = 1
	const chevronW = 2 // chevron + space
	const trailW = 1   // status glyph
	maxLabel := contentWidth - indentW - cursorW - chevronW - trailW
	if maxLabel < 3 {
		maxLabel = 3
	}
	if runewidth.StringWidth(label) > maxLabel {
		label = runewidth.Truncate(label, maxLabel-1, "…")
	}

	usedW := indentW + cursorW + runewidth.StringWidth(label) + chevronW + trailW
	gap := contentWidth - usedW
	if gap < 0 {
		gap = 0
	}

	return indent + cursor + label + strings.Repeat(" ", gap) + chevron + " " + statusStyle.Render(statusGlyph)
}

// renderStageRow renders a plan lifecycle stage row.
// Layout: [indent][indicator 1][space 1][label]
func (s *Sidebar) renderStageRow(row sidebarRow) string {
	indent := strings.Repeat(" ", row.Indent)

	var indicator string
	var indStyle lipgloss.Style
	switch {
	case row.Done:
		indicator = "✓"
		indStyle = stageCheckStyle
	case row.Active:
		indicator = "▸"
		indStyle = stageActiveStyle
	default:
		indicator = "○"
		indStyle = stageLockedStyle
	}

	return indent + indStyle.Render(indicator) + " " + row.Label
}

// renderHistoryToggleRow renders the history section divider.
func (s *Sidebar) renderHistoryToggleRow(contentWidth int) string {
	label := "── History ──"
	return historyToggleStyle.Render(label)
}

// renderCancelledRow renders a cancelled plan with strikethrough.
// Layout: [cursor 1][label...][gap...][space 1][✕ 1]
func (s *Sidebar) renderCancelledRow(row sidebarRow, idx, contentWidth int) string {
	cursor := " "
	if idx == s.selectedIdx {
		cursor = "▸"
	}

	label := row.Label
	const trailW = 2 // " ✕"
	maxLabel := contentWidth - 1 - trailW
	if maxLabel < 3 {
		maxLabel = 3
	}
	if runewidth.StringWidth(label) > maxLabel {
		label = runewidth.Truncate(label, maxLabel-1, "…")
	}

	usedW := 1 + runewidth.StringWidth(label) + trailW
	gap := contentWidth - usedW
	if gap < 0 {
		gap = 0
	}

	return cursor + sidebarCancelledStyle.Render(label) + strings.Repeat(" ", gap) + " " + sidebarCancelledStyle.Render("✕")
}
```

**Step 2: Add the `renderTreeRows` dispatcher**

Add this method right before the per-kind functions:

```go
// renderTreeRows writes the tree-mode sidebar content into b.
// Each row is rendered according to its Kind via per-kind render functions.
func (s *Sidebar) renderTreeRows(b *strings.Builder, itemWidth int) {
	contentWidth := itemWidth - 2 // account for Padding(0,1) in item styles

	for i, row := range s.rows {
		var line string

		switch row.Kind {
		case rowKindTopic:
			line = s.renderTopicRow(row, i, contentWidth)
		case rowKindPlan:
			line = s.renderPlanRow(row, i, contentWidth)
		case rowKindStage:
			line = s.renderStageRow(row)
		case rowKindHistoryToggle:
			line = s.renderHistoryToggleRow(contentWidth)
		case rowKindCancelled:
			line = s.renderCancelledRow(row, i, contentWidth)
		}

		// Apply selection styling
		if i == s.selectedIdx && s.focused {
			b.WriteString(selectedTopicStyle.Width(itemWidth).Render(line))
		} else if i == s.selectedIdx && !s.focused {
			b.WriteString(activeTopicStyle.Width(itemWidth).Render(line))
		} else {
			b.WriteString(topicItemStyle.Width(itemWidth).Render(line))
		}
		b.WriteString("\n")
	}
}
```

**Step 3: Run tests (still failing — not wired into String yet)**

Run: `go build ./ui/`
Expected: Compiles cleanly.

**Step 4: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(ui): implement per-kind render functions for tree-mode sidebar"
```

---

### Task 5: Wire `renderTreeRows` into `String()`

**Files:**
- Modify: `ui/sidebar.go` — the `String()` method

**Step 1: Add tree-mode branch**

In `String()`, replace the flat items loop (the block starting at `for i, item := range s.items {` through the closing `}` of that loop) by wrapping it in a conditional:

Find this block (starts after `itemWidth` calculation):
```go
	for i, item := range s.items {
```

Replace the entire flat loop with:

```go
	if s.useTreeMode {
		s.renderTreeRows(&b, itemWidth)
	} else {
		for i, item := range s.items {
			// ... entire existing flat rendering loop unchanged ...
		}
	}
```

Keep the entire existing flat rendering loop body inside the `else` block, unchanged.

**Step 2: Run tree render tests**

Run: `go test ./ui/ -run TestSidebarTreeRender -v`
Expected: ALL PASS

**Step 3: Run full test suite**

Run: `go test ./ui/ -v`
Expected: ALL PASS — flat-mode tests still work via the `else` branch.

**Step 4: Build check**

Run: `go build ./...`
Expected: Clean build, no errors.

**Step 5: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(ui): wire tree-mode rendering into Sidebar.String()"
```

---

### Task 6: Verify and clean up

**Step 1: Run full project tests**

Run: `go test ./... 2>&1 | tail -30`
Expected: All packages pass.

**Step 2: Run go vet**

Run: `go vet ./...`
Expected: Clean.

**Step 3: Final commit (if any cleanup needed)**

Only commit if cleanup was required. Otherwise, done.
