# Disable Cache When Focused — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Stop the normal preview tick from overwriting the embedded terminal's live output with stale cached content during focus mode.

**Architecture:** Guard `TabbedWindow.UpdatePreview()` so it no-ops when `focusMode` is true. The embedded terminal's `focusPreviewTickMsg` tick becomes the sole writer to the preview pane during focus mode. On exit, the normal tick resumes automatically.

**Tech Stack:** Go, bubbletea

---

## Context

Two concurrent tickers fight over the preview pane when focus mode is active:

1. **`focusPreviewTickMsg`** (event-driven, ≤50ms) — reads fresh content from the VT emulator via `EmbeddedTerminal.Render()` → `SetPreviewContent()` → `SetRawContent()`
2. **`previewTickMsg`** (every 50ms, always running) — calls `instanceChanged()` → `UpdatePreview()` → `UpdateContent()` → `PreviewCached()` which overwrites with **stale 500ms-old** `CachedContent` from the metadata tick

The display alternates between fresh VT output and stale cached content every ~50ms → visible stutter/flicker.

---

### Task 1: Guard UpdatePreview in focus mode

**Files:**
- Modify: `ui/tabbed_window.go:135-140`
- Test: `ui/tabbed_window_test.go` (create)

**Step 1: Write the failing test**

Create `ui/tabbed_window_test.go`:

```go
package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdatePreview_SkipsWhenFocusMode(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(80, 24)
	diff := NewDiffPane()
	git := NewGitPane()
	tw := NewTabbedWindow(preview, diff, git)
	tw.SetActiveTab(PreviewTab)

	// Set focus mode — simulates embedded terminal owning the pane.
	tw.SetFocusMode(true)

	// Attempt to update preview with nil instance.
	// Without the guard this would overwrite the preview state.
	// With the guard it should be a no-op (return nil, state unchanged).
	err := tw.UpdatePreview(nil)
	assert.NoError(t, err)

	// Preview should still have its default zero state, not the fallback
	// banner that UpdateContent(nil) would set.
	assert.False(t, preview.previewState.fallback,
		"UpdatePreview should be a no-op when focusMode is true")
}

func TestUpdatePreview_WorksWhenNotFocusMode(t *testing.T) {
	preview := NewPreviewPane()
	preview.SetSize(80, 24)
	diff := NewDiffPane()
	git := NewGitPane()
	tw := NewTabbedWindow(preview, diff, git)
	tw.SetActiveTab(PreviewTab)

	// Focus mode OFF — normal path.
	tw.SetFocusMode(false)

	err := tw.UpdatePreview(nil)
	assert.NoError(t, err)

	// With nil instance, UpdateContent sets fallback state.
	assert.True(t, preview.previewState.fallback,
		"UpdatePreview should update content when focusMode is false")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./ui/ -run TestUpdatePreview -v`
Expected: `TestUpdatePreview_SkipsWhenFocusMode` FAILS — the fallback state gets set because `UpdatePreview` does not check `focusMode`.

**Step 3: Add the focusMode guard to UpdatePreview**

In `ui/tabbed_window.go`, change `UpdatePreview`:

```go
// UpdatePreview updates the content of the preview pane. instance may be nil.
// No-op when focusMode is true — the embedded terminal owns the pane.
func (w *TabbedWindow) UpdatePreview(instance *session.Instance) error {
	if w.activeTab != PreviewTab || w.focusMode {
		return nil
	}
	return w.preview.UpdateContent(instance)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./ui/ -run TestUpdatePreview -v`
Expected: PASS — both subtests green.

**Step 5: Run full test suite**

Run: `go test ./... -count=1`
Expected: All tests pass, no regressions.

**Step 6: Commit**

```bash
git add ui/tabbed_window.go ui/tabbed_window_test.go
git commit -m "fix(ui): skip preview update in focus mode to prevent stutter

The normal previewTickMsg (50ms) was overwriting the embedded terminal's
live VT output with stale CachedContent from the metadata tick (500ms),
causing visible flicker. Guard UpdatePreview with focusMode check so the
embedded terminal is the sole pane writer during focus mode."
```
