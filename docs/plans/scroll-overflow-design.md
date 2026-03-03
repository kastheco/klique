# Scroll Overflow Design

**Date:** 2026-02-25  
**Status:** approved

## Problem

Two UI panels overflow their bounds when content exceeds the available height:

1. **Center column (instance list)** — cards overflow past the bottom of the terminal when many instances are running
2. **Left sidebar** — history rows overflow when expanded with many completed plans

Both panels currently render all content into a string buffer and rely on lipgloss `.Height()` to clip — but clipped content is unreachable, making the UI unusable with many items.

## Approach: Manual Scroll Offset

Add a `scrollOffset int` to both `List` and `Sidebar`. Render all rows into a buffer, then slice to only the visible window before wrapping in the border. `Down()`/`Up()` adjust `selectedIdx` and clamp `scrollOffset` to keep the selection visible. No new dependencies; both types stay as pure `String()`-returning structs.

## List (center column)

**New field:** `scrollOffset int` on `List` — line count from top of content area.

**`String()` changes:**
1. Render all items into a `strings.Builder` as today
2. Split result by `\n`
3. Compute `availLines = innerHeight - headerLines` (header = tabs row + blank = 2 lines; `innerHeight = l.height - borderV`)
4. Slice `lines[scrollOffset : min(scrollOffset+availLines, len(lines))]`
5. Join and pass to the border renderer (no longer sets `.Height()` since content is pre-sliced)

**`ensureSelectedVisible()`** — new private method called at the end of `Down()` and `Up()`:
- Compute `itemStartLine(idx)` = sum of `itemHeight(i)+1` for `i < idx` (the `+1` is the blank gap between items)
- Compute `itemEndLine = itemStartLine + itemHeight(idx) - 1`
- If `itemStartLine < scrollOffset` → `scrollOffset = itemStartLine`
- If `itemEndLine >= scrollOffset+availLines` → `scrollOffset = itemEndLine - availLines + 1`

`itemHeight()` already exists on `List`.

**`SetSize()` change:** after updating height, call `ensureSelectedVisible()` so resize keeps selection in view.

## Sidebar

**New field:** `scrollOffset int` on `Sidebar` — row index of the first visible row.

Each sidebar row renders as exactly 1 line.

**`renderTreeRows()` changes:**
- Compute `availRows = innerHeight - headerLines` (header = search bar 3 lines + 2 blank lines = 5; `innerHeight = s.height - borderAndPadding`)
- Only render rows `s.rows[scrollOffset : min(scrollOffset+availRows, len(s.rows))]`

**Scroll clamping in `Down()`/`Up()`** (sidebar row navigation):
- After updating `selectedIdx`, clamp: if `selectedIdx < scrollOffset` → `scrollOffset = selectedIdx`; if `selectedIdx >= scrollOffset+availRows` → `scrollOffset = selectedIdx - availRows + 1`

**`SetSize()` change:** recompute `availRows` and clamp `scrollOffset` after resize.

## What Doesn't Change

- No new dependencies
- `List` and `Sidebar` remain pure `String()`-returning structs (no `Update`/`tea.Cmd`)
- Key routing in `app_input.go` unchanged
- Search, highlight, filter, and topic-collapse logic unaffected
- `scrollOffset` resets to 0 when the instance list or plan list is rebuilt (via `rebuildFilteredItems()` and `rebuildTree()`)

## Testing

- Table-driven unit tests for `ensureSelectedVisible()` covering: scroll down past bottom, scroll up past top, resize smaller than content, resize larger than content
- Table-driven unit tests for sidebar scroll clamping: same scenarios
- Existing `List` and `Sidebar` tests must still pass
