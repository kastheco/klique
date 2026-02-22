# Tree-Mode Sidebar Renderer

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a tree-mode rendering branch to `Sidebar.String()` that renders `s.rows` with proper indentation, chevrons, and status glyphs.

**Architecture:** When `useTreeMode == true`, `String()` iterates `s.rows` instead of `s.items`, rendering each `sidebarRowKind` with the appropriate visual treatment. The flat `s.items` rendering path remains intact as fallback.

**Tech Stack:** Go, lipgloss, go-runewidth

**Design doc:** `docs/plans/2026-02-22-tree-mode-sidebar-design.md`

---

### Task 1: Add tree-mode row rendering styles

**Files:**
- Modify: `ui/sidebar.go` (styles section, lines 17-55)

**Step 1: Add new lipgloss styles for tree rows**

Add these styles after the existing `sidebarCancelledStyle` (line 127):

```go
// stageCheckStyle is for completed stage indicators.
var stageCheckStyle = lipgloss.NewStyle().Foreground(ColorFoam)

// stageActiveStyle is for the currently active stage indicator.
var stageActiveStyle = lipgloss.NewStyle().Foreground(ColorIris)

// stageLockedStyle is for locked/unreachable stage indicators.
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

### Task 2: Write failing tests for tree-mode rendering

**Files:**
- Modify: `ui/sidebar_test.go`

**Step 1: Write tests for renderTreeRows**

Add these tests to the end of `ui/sidebar_test.go`:

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
	// Should contain the plan name
	assert.Contains(t, output, "bugfix")
	// Should contain the ready glyph
	assert.Contains(t, output, "○")
}

func TestSidebarTreeRender_TopicWithPlans(t *testing.T) {
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
	// The output should contain both plans (basic sanity)
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
		nil,
		nil,
		nil,
		[]PlanDisplay{{Filename: "dropped.md", Status: "cancelled"}},
	)

	output := s.String()
	assert.Contains(t, output, "dropped")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/ -run TestSidebarTreeRender -v`
Expected: FAIL — `String()` currently renders flat `s.items`, not tree rows. The tree data is set but `DisableTreeMode()` is never called here (tests don't go through app layer), so `useTreeMode` is true but `String()` ignores it.

**Step 3: Commit**

```bash
git add ui/sidebar_test.go
git commit -m "test(ui): add failing tests for tree-mode sidebar rendering"
```

---

### Task 3: Implement `renderTreeRows` helper

**Files:**
- Modify: `ui/sidebar.go`

**Step 1: Add the `renderTreeRows` method**

Add this method before the existing `String()` function (before line 662). This is a helper that renders all tree rows into a string builder, used by `String()` when `useTreeMode == true`.

```go
// renderTreeRows writes the tree-mode sidebar content into b.
// Each row is rendered according to its Kind with appropriate
// indentation, chevrons, and status glyphs.
func (s *Sidebar) renderTreeRows(b *strings.Builder, itemWidth int) {
	contentWidth := itemWidth - 2 // account for Padding(0,1) in item styles

	for i, row := range s.rows {
		var line string

		switch row.Kind {
		case rowKindTopic:
			chevron := "▸"
			if !row.Collapsed {
				chevron = "▾"
			}
			cursor := " "
			if i == s.selectedIdx {
				cursor = "▸"
			}

			// Aggregate status from child plans
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
			const cursorW = 1
			const chevronW = 2 // "▸ " or "▾ "
			trailW := 0
			if statusGlyph != "" {
				trailW = 2 // " ●"
			}
			maxLabel := contentWidth - cursorW - chevronW - trailW
			if maxLabel < 3 {
				maxLabel = 3
			}
			if runewidth.StringWidth(label) > maxLabel {
				label = runewidth.Truncate(label, maxLabel-1, "…")
			}

			text := topicLabelStyle.Render(label)
			usedW := cursorW + chevronW + runewidth.StringWidth(label) + trailW
			gap := contentWidth - usedW
			if gap < 0 {
				gap = 0
			}

			trail := ""
			if statusGlyph != "" {
				trail = " " + statusStyle.Render(statusGlyph)
			}
			line = cursor + chevron + " " + text + strings.Repeat(" ", gap) + trail

		case rowKindPlan:
			// Determine indentation: plans under a topic get 2-char indent
			indent := ""
			if s.planIsUnderTopic(i) {
				indent = "  "
			}

			chevron := ""
			if !row.Collapsed {
				chevron = "▾"
			} else {
				chevron = "▸"
			}

			cursor := " "
			if i == s.selectedIdx {
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
			indentW := runewidth.StringWidth(indent)
			const cursorW = 1
			const chevronW = 2 // "▸ "
			const trailW = 2   // " ○"
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

			line = indent + cursor + label + strings.Repeat(" ", gap) + chevron + " " + statusStyle.Render(statusGlyph)

		case rowKindStage:
			// Determine indent: parent plan indent + 2
			indent := "  "
			if s.planIsUnderTopic(s.findParentPlan(i)) {
				indent = "    "
			}

			var indicator string
			var indStyle lipgloss.Style
			switch {
			case row.Done:
				indicator = "✓"
				indStyle = stageCheckStyle
			case row.Active:
				indicator = "▸"
				indStyle = stageActiveStyle
			default: // locked
				indicator = "○"
				indStyle = stageLockedStyle
			}

			label := row.Label
			line = indent + indStyle.Render(indicator) + " " + label

		case rowKindHistoryToggle:
			line = historyToggleStyle.Render("── History ──")

		case rowKindCancelled:
			cursor := " "
			if i == s.selectedIdx {
				cursor = "▸"
			}
			label := row.Label
			maxLabel := contentWidth - 1 - 2 // cursor + " ✕"
			if maxLabel < 3 {
				maxLabel = 3
			}
			if runewidth.StringWidth(label) > maxLabel {
				label = runewidth.Truncate(label, maxLabel-1, "…")
			}
			usedW := 1 + runewidth.StringWidth(label) + 2
			gap := contentWidth - usedW
			if gap < 0 {
				gap = 0
			}
			line = cursor + sidebarCancelledStyle.Render(label) + strings.Repeat(" ", gap) + " " + sidebarCancelledStyle.Render("✕")
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

**Step 2: Add helper methods**

Add these helpers after `renderTreeRows`:

```go
// planIsUnderTopic returns true if the plan at rowIdx is a child of a topic row.
func (s *Sidebar) planIsUnderTopic(rowIdx int) bool {
	for i := rowIdx - 1; i >= 0; i-- {
		switch s.rows[i].Kind {
		case rowKindTopic:
			return true
		case rowKindPlan:
			// Hit another plan at same or higher level — not under a topic
			return s.planIsUnderTopic(i)
		}
	}
	return false
}

// findParentPlan returns the row index of the nearest plan row above rowIdx.
func (s *Sidebar) findParentPlan(rowIdx int) int {
	for i := rowIdx - 1; i >= 0; i-- {
		if s.rows[i].Kind == rowKindPlan {
			return i
		}
	}
	return 0
}
```

**Step 3: Run tests**

Run: `go test ./ui/ -run TestSidebarTreeRender -v`
Expected: Still FAIL — `String()` doesn't call `renderTreeRows` yet.

**Step 4: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(ui): implement renderTreeRows helper for tree-mode sidebar"
```

---

### Task 4: Wire renderTreeRows into String()

**Files:**
- Modify: `ui/sidebar.go` — the `String()` method (line 662)

**Step 1: Add tree-mode branch to String()**

In `String()`, after the search bar rendering (after `b.WriteString("\n\n")` on line 692), add a tree-mode branch before the existing flat items loop:

Replace the existing items rendering block (lines 694-846) with:

```go
	// Items
	itemWidth := innerWidth - 2 // item padding
	if itemWidth < 4 {
		itemWidth = 4
	}

	if s.useTreeMode {
		s.renderTreeRows(&b, itemWidth)
	} else {
		// --- Flat mode (legacy) ---
		for i, item := range s.items {
			// ... (entire existing flat rendering loop unchanged)
		}
	}
```

Keep the entire existing flat rendering loop inside the `else` block unchanged.

**Step 2: Run tests**

Run: `go test ./ui/ -run TestSidebarTreeRender -v`
Expected: PASS — all tree render tests should pass now.

Run: `go test ./ui/ -v`
Expected: PASS — all existing tests still pass too.

**Step 3: Commit**

```bash
git add ui/sidebar.go
git commit -m "feat(ui): wire tree-mode rendering into Sidebar.String()"
```

---

### Task 5: Add SetFocused and SetSize if missing, verify full test suite

**Files:**
- Modify: `ui/sidebar.go` (if needed)

**Step 1: Verify `SetFocused` and `SetSize` exist**

Check that `Sidebar` has `SetFocused(bool)` and `SetSize(w, h int)` methods. These are used by the tests. If they don't exist, add them:

```go
func (s *Sidebar) SetFocused(focused bool) {
	s.focused = focused
}

func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height
}
```

**Step 2: Run full test suite**

Run: `go test ./ui/ -v`
Expected: ALL PASS

Run: `go build ./...`
Expected: Clean build

**Step 3: Commit**

```bash
git add ui/sidebar.go
git commit -m "test(ui): ensure sidebar helpers exist for tree-mode tests"
```
