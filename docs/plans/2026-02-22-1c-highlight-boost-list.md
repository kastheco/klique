# Highlight-Boost Instance List

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the hide-filter instance list with a highlight-boost model: all instances always visible, matching instances highlighted and sorted to the top.

**Architecture:** Add a `highlightFilter` field to `List` that holds the current sidebar selection context (topic name or plan filename). On rebuild, instances are partitioned into matched/unmatched, each sorted independently by the current sort mode, then concatenated. The renderer dims unmatched instances when a filter is active.

**Tech Stack:** Go, lipgloss, bubbletea

**Design doc:** `docs/plans/2026-02-22-tree-mode-sidebar-design.md`

---

### Task 1: Write failing tests for highlight-boost sorting

**Files:**
- Create: `ui/list_highlight_test.go`

**Step 1: Write tests**

```go
package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestInstance(title, planFile string) *session.Instance {
	return &session.Instance{
		Title:    title,
		PlanFile: planFile,
		Status:   session.Running,
	}
}

func TestListHighlightFilter_MatchedFirst(t *testing.T) {
	s := spinner.New()
	l := NewList(&s, false)

	a := makeTestInstance("alpha", "plan-a.md")
	b := makeTestInstance("bravo", "plan-b.md")
	c := makeTestInstance("charlie", "plan-a.md")

	l.AddInstance(a)
	l.AddInstance(b)
	l.AddInstance(c)

	l.SetHighlightFilter("plan", "plan-a.md")

	items := l.GetInstances()
	require.Len(t, items, 3)
	// Matched instances (plan-a.md) should come first
	assert.Equal(t, "plan-a.md", items[0].PlanFile)
	assert.Equal(t, "plan-a.md", items[1].PlanFile)
	// Unmatched last
	assert.Equal(t, "plan-b.md", items[2].PlanFile)
}

func TestListHighlightFilter_EmptyShowsAll(t *testing.T) {
	s := spinner.New()
	l := NewList(&s, false)

	a := makeTestInstance("alpha", "plan-a.md")
	b := makeTestInstance("bravo", "plan-b.md")

	l.AddInstance(a)
	l.AddInstance(b)

	l.SetHighlightFilter("", "")

	items := l.GetInstances()
	assert.Len(t, items, 2)
}

func TestListHighlightFilter_TopicMatch(t *testing.T) {
	s := spinner.New()
	l := NewList(&s, false)

	a := makeTestInstance("alpha", "plan-a.md")
	a.Topic = "auth"
	b := makeTestInstance("bravo", "plan-b.md")
	b.Topic = "deploy"
	c := makeTestInstance("charlie", "plan-c.md")
	c.Topic = "auth"

	l.AddInstance(a)
	l.AddInstance(b)
	l.AddInstance(c)

	l.SetHighlightFilter("topic", "auth")

	items := l.GetInstances()
	require.Len(t, items, 3)
	assert.Equal(t, "auth", items[0].Topic)
	assert.Equal(t, "auth", items[1].Topic)
	assert.Equal(t, "deploy", items[2].Topic)
}

func TestListIsHighlighted(t *testing.T) {
	s := spinner.New()
	l := NewList(&s, false)

	a := makeTestInstance("alpha", "plan-a.md")
	b := makeTestInstance("bravo", "plan-b.md")

	l.AddInstance(a)
	l.AddInstance(b)

	l.SetHighlightFilter("plan", "plan-a.md")

	assert.True(t, l.IsHighlighted(a))
	assert.False(t, l.IsHighlighted(b))
}

func TestListIsHighlighted_NoFilter(t *testing.T) {
	s := spinner.New()
	l := NewList(&s, false)

	a := makeTestInstance("alpha", "plan-a.md")
	l.AddInstance(a)

	l.SetHighlightFilter("", "")
	// No filter active — everything is "highlighted" (normal rendering)
	assert.True(t, l.IsHighlighted(a))
}
```

**Step 2: Check if `Instance.Topic` field exists**

Run: `rg -n 'Topic.*string' session/instance.go | head -5`

If `Topic` field doesn't exist on Instance, add it:

```go
// Topic is the topic/group this instance belongs to (from plan-state).
Topic string
```

**Step 3: Run tests to verify they fail**

Run: `go test ./ui/ -run TestListHighlight -v`
Expected: FAIL — `SetHighlightFilter` and `IsHighlighted` don't exist.

**Step 4: Commit**

```bash
git add ui/list_highlight_test.go
git commit -m "test(ui): add failing tests for highlight-boost instance list"
```

---

### Task 2: Implement SetHighlightFilter and partition sort

**Files:**
- Modify: `ui/list.go`

**Step 1: Add highlight fields to List struct**

Add to the `List` struct:

```go
	highlightKind  string // "plan", "topic", or "" (no highlight)
	highlightValue string // plan filename or topic name
```

**Step 2: Add SetHighlightFilter method**

```go
// SetHighlightFilter sets the highlight context from sidebar selection.
// kind is "plan" or "topic", value is the plan filename or topic name.
// Empty kind means no highlight (all items render normally).
func (l *List) SetHighlightFilter(kind, value string) {
	l.highlightKind = kind
	l.highlightValue = value
	l.rebuildFilteredItems()
}
```

**Step 3: Add IsHighlighted method**

```go
// IsHighlighted returns true if the instance matches the current highlight filter.
// Returns true for all instances when no filter is active.
func (l *List) IsHighlighted(inst *session.Instance) bool {
	if l.highlightKind == "" || l.highlightValue == "" {
		return true
	}
	return l.matchesHighlight(inst)
}

func (l *List) matchesHighlight(inst *session.Instance) bool {
	switch l.highlightKind {
	case "plan":
		return inst.PlanFile == l.highlightValue
	case "topic":
		return inst.Topic == l.highlightValue
	}
	return false
}
```

**Step 4: Modify rebuildFilteredItems to partition-sort**

Replace the sort call at the end of `rebuildFilteredItems` with partition-aware sorting:

```go
func (l *List) rebuildFilteredItems() {
	// First apply status filter
	var filtered []*session.Instance
	if l.statusFilter == StatusFilterActive {
		for _, inst := range l.allItems {
			if !inst.Paused() {
				filtered = append(filtered, inst)
			}
		}
	} else {
		filtered = l.allItems
	}

	// Partition into matched/unmatched if highlight is active
	if l.highlightKind != "" && l.highlightValue != "" {
		var matched, unmatched []*session.Instance
		for _, inst := range filtered {
			if l.matchesHighlight(inst) {
				matched = append(matched, inst)
			} else {
				unmatched = append(unmatched, inst)
			}
		}
		l.items = matched
		l.sortItems()
		sortedMatched := make([]*session.Instance, len(l.items))
		copy(sortedMatched, l.items)

		l.items = unmatched
		l.sortItems()

		l.items = append(sortedMatched, l.items...)
	} else {
		l.items = filtered
		l.sortItems()
	}

	if l.selectedIdx >= len(l.items) {
		l.selectedIdx = len(l.items) - 1
	}
	if l.selectedIdx < 0 {
		l.selectedIdx = 0
	}
}
```

**Step 5: Run tests**

Run: `go test ./ui/ -run TestListHighlight -v`
Expected: PASS

Run: `go test ./ui/ -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add ui/list.go
git commit -m "feat(ui): implement highlight-boost partition sort for instance list"
```

---

### Task 3: Add dimmed rendering for non-highlighted instances

**Files:**
- Modify: `ui/list_renderer.go`

**Step 1: Add dimmed styles**

Add after the existing style definitions (around line 15):

```go
// dimmedTitleStyle is for non-highlighted instances when a filter is active.
var dimmedTitleStyle = lipgloss.NewStyle().
	Foreground(ColorMuted)

// dimmedDescStyle matches dimmedTitleStyle for the description line.
var dimmedDescStyle = lipgloss.NewStyle().
	Foreground(ColorMuted)
```

**Step 2: Modify Render signature to accept highlighted flag**

Change the `Render` method signature:

```go
func (r *InstanceRenderer) Render(i *session.Instance, selected bool, focused bool, hasMultipleRepos bool, rowIndex int, highlighted bool) string {
```

At the top of the method, after the existing style selection block (lines 31-47), add:

```go
	// Dim non-highlighted instances when a highlight filter is active
	if !highlighted && !selected {
		titleS = dimmedTitleStyle
		descS = dimmedDescStyle
	}
```

**Step 3: Update all callers of Render**

Search for calls to `r.Render` or `renderer.Render` and add the `highlighted` parameter. The caller is in the list rendering code — find it and pass `l.IsHighlighted(inst)`.

Run: `rg -n '\.Render\(' ui/list_renderer.go ui/list.go` to find the call site.

Update the call to pass the highlighted flag.

**Step 4: Build and test**

Run: `go build ./...`
Expected: Clean build

Run: `go test ./ui/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add ui/list_renderer.go ui/list.go
git commit -m "feat(ui): dim non-highlighted instances in list renderer"
```
