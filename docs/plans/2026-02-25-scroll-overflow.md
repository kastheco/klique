# Scroll Overflow Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add scroll offset tracking to `List` and `Sidebar` so content that exceeds panel height is scrollable rather than clipped.

**Architecture:** Add `scrollOffset int` to both `List` (line-level offset) and `Sidebar` (row-level offset). `String()` renders all content then slices to the visible window. `Down()`/`Up()` call `ensureSelectedVisible()` after updating `selectedIdx` to keep the cursor in view. No new dependencies.

**Tech Stack:** Go, lipgloss, bubbles — no new imports needed.

---

### Task 1: List — add scrollOffset field and ensureSelectedVisible

**Files:**
- Modify: `ui/list.go` (struct + Down/Up/SetSize/rebuildFilteredItems)
- Modify: `ui/list_renderer.go` (String())
- Create: `ui/list_scroll_test.go`

**Step 1: Write the failing tests**

Create `ui/list_scroll_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/kastheco/kasmos/session"
	"github.com/stretchr/testify/assert"
)

func makeTestList(n int) *List {
	sp := spinner.New()
	l := NewList(&sp, false)
	l.SetSize(40, 30)
	for i := 0; i < n; i++ {
		inst := &session.Instance{}
		inst.Title = fmt.Sprintf("inst-%d", i)
		inst.MemMB = 100
		finalize := l.AddInstance(inst)
		finalize()
		inst.MarkStartedForTest()
	}
	return l
}

func TestListScrollOffset_DownScrolls(t *testing.T) {
	l := makeTestList(20)
	l.SetSize(40, 10) // very short — forces scrolling
	initial := l.scrollOffset
	for i := 0; i < 15; i++ {
		l.Down()
	}
	assert.Greater(t, l.scrollOffset, initial, "scrollOffset should increase when selection moves past bottom")
}

func TestListScrollOffset_UpScrollsBack(t *testing.T) {
	l := makeTestList(20)
	l.SetSize(40, 10)
	for i := 0; i < 15; i++ {
		l.Down()
	}
	offset := l.scrollOffset
	for i := 0; i < 15; i++ {
		l.Up()
	}
	assert.Less(t, l.scrollOffset, offset, "scrollOffset should decrease when selection moves back to top")
	assert.Equal(t, 0, l.scrollOffset, "scrollOffset should reset to 0 at top")
}

func TestListScrollOffset_ResizeClamps(t *testing.T) {
	l := makeTestList(20)
	l.SetSize(40, 10)
	for i := 0; i < 15; i++ {
		l.Down()
	}
	// Make the list taller so offset might be invalid
	l.SetSize(40, 60)
	assert.GreaterOrEqual(t, l.scrollOffset, 0, "scrollOffset must not go negative after resize")
}

func TestListString_DoesNotOverflowHeight(t *testing.T) {
	l := makeTestList(20)
	l.SetSize(40, 14)
	rendered := l.String()
	lines := strings.Split(rendered, "\n")
	assert.LessOrEqual(t, len(lines), 14, "rendered output must not exceed panel height")
}
```

**Step 2: Run tests to confirm they fail**

```bash
cd /home/kas/dev/kasmos
go test ./ui/ -run TestListScroll -v
```
Expected: compile error (scrollOffset field doesn't exist yet).

**Step 3: Add scrollOffset to List struct**

In `ui/list.go`, add field after `focused bool`:

```go
scrollOffset int // line offset from top of rendered content
```

**Step 4: Add itemStartLine helper to list_renderer.go**

Add after `itemHeight()`:

```go
// itemStartLine returns the line offset (0-based) where item idx begins in the
// rendered content buffer (excluding the 2-line header).
func (l *List) itemStartLine(idx int) int {
	line := 0
	for i := 0; i < idx; i++ {
		line += l.itemHeight(i) + 1 // +1 for blank gap between items
	}
	return line
}

// availContentLines returns the number of lines available for item content
// inside the border, excluding the 2-line header (tabs + blank).
func (l *List) availContentLines() int {
	const borderV = 2
	const headerLines = 2
	avail := l.height - borderV - headerLines
	if avail < 1 {
		avail = 1
	}
	return avail
}

// ensureSelectedVisible adjusts scrollOffset so the selected item is fully visible.
func (l *List) ensureSelectedVisible() {
	if len(l.items) == 0 {
		l.scrollOffset = 0
		return
	}
	avail := l.availContentLines()
	start := l.itemStartLine(l.selectedIdx)
	end := start + l.itemHeight(l.selectedIdx) - 1

	if start < l.scrollOffset {
		l.scrollOffset = start
	}
	if end >= l.scrollOffset+avail {
		l.scrollOffset = end - avail + 1
	}
	if l.scrollOffset < 0 {
		l.scrollOffset = 0
	}
}
```

**Step 5: Call ensureSelectedVisible in Down, Up, SetSize, rebuildFilteredItems**

In `ui/list.go`:

`Down()` — append after `l.selectedIdx++`:
```go
l.ensureSelectedVisible()
```

`Up()` — append after `l.selectedIdx--`:
```go
l.ensureSelectedVisible()
```

`SetSize()` — append at end of function:
```go
l.ensureSelectedVisible()
```

`rebuildFilteredItems()` — append at end (after the selectedIdx clamp block):
```go
l.scrollOffset = 0
l.ensureSelectedVisible()
```

**Step 6: Slice content in String()**

In `ui/list_renderer.go`, replace the content rendering block (lines 290-308) with:

```go
	// Render the list.
	for i, item := range l.items {
		b.WriteString(l.renderer.Render(item, i == l.selectedIdx, l.focused, len(l.repos) > 1, i, l.IsHighlighted(item)))
		if i != len(l.items)-1 {
			b.WriteString("\n\n")
		}
	}

	// Slice to the visible window using scrollOffset.
	allLines := strings.Split(b.String(), "\n")
	avail := l.availContentLines()
	start := l.scrollOffset
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + avail
	if end > len(allLines) {
		end = len(allLines)
	}
	visibleContent := strings.Join(allLines[start:end], "\n")

	// Wrap in border matching the sidebar style.
	borderStyle := listBorderStyle
	if l.focused {
		borderStyle = borderStyle.Border(lipgloss.DoubleBorder()).BorderForeground(ColorIris)
	}
	innerHeight := l.height - borderV
	if innerHeight < 4 {
		innerHeight = 4
	}
	bordered := borderStyle.Width(innerWidth).Height(innerHeight).Render(visibleContent)
	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, bordered)
```

Note: remove the `b.String()` that was previously passed directly to `.Height(innerHeight).Render()`. The header (`tabs + \n\n`) is written first then excluded from the slice by computing `availContentLines` to exclude it — but the header is prepended BEFORE the slice. Need to adjust: write header to `headerStr`, items to `b`, then combine as `headerStr + strings.Join(visible, "\n")`.

Revised approach — split header and content:

```go
	// Header string (tabs row + blank line).
	var header strings.Builder
	// ... (move the existing tab-writing code here) ...
	header.WriteString("\n\n")
	headerStr := header.String()

	// Item content.
	var content strings.Builder
	for i, item := range l.items {
		content.WriteString(l.renderer.Render(item, i == l.selectedIdx, l.focused, len(l.repos) > 1, i, l.IsHighlighted(item)))
		if i != len(l.items)-1 {
			content.WriteString("\n\n")
		}
	}

	// Slice content to visible window.
	allLines := strings.Split(content.String(), "\n")
	avail := l.availContentLines()
	start := l.scrollOffset
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + avail
	if end > len(allLines) {
		end = len(allLines)
	}
	visible := strings.Join(allLines[start:end], "\n")

	borderStyle := listBorderStyle
	if l.focused {
		borderStyle = borderStyle.Border(lipgloss.DoubleBorder()).BorderForeground(ColorIris)
	}
	innerHeight := l.height - borderV
	if innerHeight < 4 {
		innerHeight = 4
	}
	bordered := borderStyle.Width(innerWidth).Height(innerHeight).Render(headerStr + visible)
	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, bordered)
```

**Step 7: Run tests**

```bash
go test ./ui/ -run TestListScroll -v
```
Expected: all 4 tests PASS.

**Step 8: Run full ui test suite**

```bash
go test ./ui/... -v
```
Expected: all existing tests still pass.

**Step 9: Build**

```bash
go build ./...
```

**Step 10: Commit**

```bash
git add ui/list.go ui/list_renderer.go ui/list_scroll_test.go
git commit -m "feat(ui): add scroll offset to instance list to prevent overflow"
```

---

### Task 2: Sidebar — add scrollOffset field and row clamping

**Files:**
- Modify: `ui/sidebar.go` (struct + Down/Up/SetSize/rebuildRows)
- Create: `ui/sidebar_scroll_test.go`

**Step 1: Write the failing tests**

Create `ui/sidebar_scroll_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeTestSidebar(nPlans int) *Sidebar {
	s := NewSidebar()
	s.SetSize(30, 15)
	topics := []TopicDisplay{{Name: "tools", ID: "tools"}}
	plans := make([]PlanDisplay, nPlans)
	for i := range plans {
		plans[i] = PlanDisplay{
			Name:   fmt.Sprintf("plan-%d", i),
			File:   fmt.Sprintf("2026-02-25-plan-%d.md", i),
			Status: "ready",
			Topic:  "tools",
		}
	}
	s.SetTopicsAndPlans(topics, nil, nil)
	return s
}

func TestSidebarScrollOffset_DownScrolls(t *testing.T) {
	s := makeTestSidebar(30)
	s.SetSize(30, 10) // short height forces scroll
	initial := s.scrollOffset
	for i := 0; i < 25; i++ {
		s.Down()
	}
	assert.Greater(t, s.scrollOffset, initial, "scrollOffset should grow as selection moves down")
}

func TestSidebarScrollOffset_UpScrollsBack(t *testing.T) {
	s := makeTestSidebar(30)
	s.SetSize(30, 10)
	for i := 0; i < 25; i++ {
		s.Down()
	}
	for i := 0; i < 25; i++ {
		s.Up()
	}
	assert.Equal(t, 0, s.scrollOffset, "scrollOffset should return to 0 at top")
}

func TestSidebarString_DoesNotOverflowHeight(t *testing.T) {
	s := makeTestSidebar(30)
	s.SetSize(30, 12)
	rendered := s.String()
	lines := strings.Split(rendered, "\n")
	assert.LessOrEqual(t, len(lines), 12, "rendered sidebar must not exceed panel height")
}
```

**Step 2: Run tests to confirm they fail**

```bash
go test ./ui/ -run TestSidebarScroll -v
```
Expected: compile error (scrollOffset doesn't exist).

**Step 3: Add scrollOffset to Sidebar struct**

In `ui/sidebar.go`, add field after `historyExpanded bool`:

```go
scrollOffset int // row index of first visible sidebar row
```

**Step 4: Add availSidebarRows helper**

Add after `SetSize()`:

```go
// availSidebarRows returns the number of sidebar rows that fit in the panel.
// Header = search bar (3 lines with border) + 2 blank lines = 5 lines.
// Border+padding = 4 lines (2 border + 2 padding).
func (s *Sidebar) availSidebarRows() int {
	const borderAndPadding = 4
	const headerLines = 5
	avail := s.height - borderAndPadding - headerLines
	if avail < 1 {
		avail = 1
	}
	return avail
}

// clampSidebarScroll adjusts scrollOffset so selectedIdx stays in the visible window.
func (s *Sidebar) clampSidebarScroll() {
	avail := s.availSidebarRows()
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+avail {
		s.scrollOffset = s.selectedIdx - avail + 1
	}
	if s.scrollOffset < 0 {
		s.scrollOffset = 0
	}
}
```

**Step 5: Call clampSidebarScroll in Up, Down, SetSize, rebuildRows**

`Up()` — in the `s.useTreeMode` branch, append after `s.selectedIdx--`:
```go
s.clampSidebarScroll()
```
Also add it to the non-tree branch, after `s.selectedIdx = i; return` — but since that returns early, add `s.clampSidebarScroll()` before each `return` in `Up()` and `Down()`.

Simpler: refactor `Up()` tree-mode branch to not early-return:

```go
func (s *Sidebar) Up() {
	if s.useTreeMode {
		if s.selectedIdx > 0 {
			s.selectedIdx--
		}
		s.clampSidebarScroll()
		return
	}
	for i := s.selectedIdx - 1; i >= 0; i-- {
		if !s.items[i].IsSection {
			s.selectedIdx = i
			s.clampSidebarScroll()
			return
		}
	}
}

func (s *Sidebar) Down() {
	if s.useTreeMode {
		if s.selectedIdx < len(s.rows)-1 {
			s.selectedIdx++
		}
		s.clampSidebarScroll()
		return
	}
	for i := s.selectedIdx + 1; i < len(s.items); i++ {
		if !s.items[i].IsSection {
			s.selectedIdx = i
			s.clampSidebarScroll()
			return
		}
	}
}
```

`SetSize()` — append at end:
```go
s.clampSidebarScroll()
```

`rebuildRows()` — append at end (it's called from SetTopicsAndPlans):
```go
s.scrollOffset = 0
s.clampSidebarScroll()
```

**Step 6: Slice rows in renderTreeRows()**

In `ui/sidebar.go`, replace the loop start in `renderTreeRows()`:

```go
func (s *Sidebar) renderTreeRows(b *strings.Builder, itemWidth int) {
	contentWidth := itemWidth - 2

	avail := s.availSidebarRows()
	startRow := s.scrollOffset
	endRow := startRow + avail
	if endRow > len(s.rows) {
		endRow = len(s.rows)
	}

	for i, row := range s.rows[startRow:endRow] {
		i = i + startRow // preserve absolute index for selection check
		// ... rest unchanged ...
	}
}
```

Wait — the loop variable `i` is used for `i == s.selectedIdx` on line 906. Need to keep absolute index. Use:

```go
	for relIdx, row := range s.rows[startRow:endRow] {
		i := relIdx + startRow
		var line string
		// ... switch on row.Kind (unchanged) ...
		if i == s.selectedIdx && s.focused {
		// ... (unchanged) ...
		}
	}
```

**Step 7: Run tests**

```bash
go test ./ui/ -run TestSidebarScroll -v
```
Expected: all 3 tests PASS.

**Step 8: Run full ui test suite**

```bash
go test ./ui/... -v
```
Expected: all existing tests pass.

**Step 9: Build**

```bash
go build ./...
```

**Step 10: Commit**

```bash
git add ui/sidebar.go ui/sidebar_scroll_test.go
git commit -m "feat(ui): add scroll offset to sidebar to prevent history overflow"
```

---

### Task 3: Verify full build and test suite

**Files:** none (verification only)

**Step 1: Full build**

```bash
go build ./...
```
Expected: clean compile.

**Step 2: Full test suite**

```bash
go test ./... -count=1
```
Expected: all pass.

**Step 3: Smoke check — run kasmos and verify scrolling works**

```bash
go run . &
```
Navigate with j/k past the visible area — cards should scroll smoothly. History section in sidebar should scroll when expanded with many entries.

**Step 4: Commit any fixups if needed**

If tests revealed issues, commit fixes now.
