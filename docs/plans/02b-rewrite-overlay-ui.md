# Rewrite Overlay UI Implementation Plan

**Goal:** Clean-room rewrite all upstream-derived code in `ui/overlay/` to remove AGPL-tainted lines. The overlay system provides modal dialogs (confirmations, text input, pickers, context menus, toasts) rendered on top of the main TUI. Note: `overlay.go` was adapted from lipgloss PR #102 (MIT-licensed charmbracelet code), which has different licensing implications than AGPL — but we rewrite it anyway for a clean baseline.

**Architecture:** Seven files rewritten in-place. All overlay types implement a common interface pattern (Init, Update, View from bubbletea). The `PlaceOverlay` function in overlay.go handles ANSI-aware text compositing. Files that are 100% original (formOverlay.go, tmuxBrowserOverlay.go, permissionOverlay.go, theme.go) are untouched. Existing tests (formOverlay_test.go, textInput_test.go, tmuxBrowserOverlay_test.go, toast_test.go) serve as the regression suite.

**Tech Stack:** Go 1.24, bubbletea v1.3.x, lipgloss v1.1.x, bubbles v0.20+, testify

**Size:** Medium (estimated ~3 hours, 4 tasks, 2 waves)

---

## Wave 1: Foundation Layer

### Task 1: Rewrite overlay.go — ANSI-Aware Text Compositing

**Files:**
- Modify: `ui/overlay/overlay.go`
- Test: `ui/overlay/formOverlay_test.go` (existing, exercises PlaceOverlay indirectly)

**Step 1: write the failing test**

Existing tests exercise `PlaceOverlay` through overlay rendering. No new tests needed — compile + visual regression is sufficient for a compositing function.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/overlay/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `ui/overlay/overlay.go` from scratch:

- `PlaceOverlay(x, y, fg, bg, center, shadow)` — composite foreground text onto background text at (x,y). Must handle ANSI escape sequences correctly: strip ANSI for width calculation, preserve ANSI in output. Optional centering, optional shadow effect.
- Helper functions for ANSI-aware string splitting, width measurement, and line padding
- Pre-compiled regex for ANSI color code replacement

The original was adapted from lipgloss PR #102 (MIT). The rewrite should use the same `ansi.PrintableRuneWidth` approach but with fresh implementation of the compositing algorithm.

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/overlay/overlay.go
git commit -m "feat(clean-room): rewrite ui/overlay/overlay.go from scratch"
```

### Task 2: Rewrite confirmationOverlay.go and textOverlay.go — Simple Overlays

**Files:**
- Modify: `ui/overlay/confirmationOverlay.go`
- Modify: `ui/overlay/textOverlay.go`
- Test: `ui/overlay/formOverlay_test.go` (existing)

**Step 1: write the failing test**

These are simple overlays (75 and 47 lines respectively). Existing tests cover them through overlay rendering. No new tests needed.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/overlay/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite both files from scratch:

**confirmationOverlay.go:**
- `ConfirmationOverlay` struct — title, message, onConfirm/onCancel callbacks, selected index
- `NewConfirmationOverlay(title, message, onConfirm, onCancel)` — constructor
- `Update(msg)` — handle left/right arrow keys for yes/no selection, enter to confirm
- `View()` — render bordered box with title, message, and yes/no buttons

**textOverlay.go:**
- `TextOverlay` struct — title, content, width/height
- `NewTextOverlay(title, content)` — constructor
- `Update(msg)` — handle escape/q to close
- `View()` — render bordered box with title and scrollable content

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/overlay/confirmationOverlay.go ui/overlay/textOverlay.go
git commit -m "feat(clean-room): rewrite confirmation and text overlays from scratch"
```

## Wave 2: Interactive Overlays

> **depends on wave 1:** Interactive overlays use `PlaceOverlay` from overlay.go (rewritten in wave 1) for rendering.

### Task 3: Rewrite textInput.go, pickerOverlay.go, contextMenu.go — Input Overlays

**Files:**
- Modify: `ui/overlay/textInput.go`
- Modify: `ui/overlay/pickerOverlay.go`
- Modify: `ui/overlay/contextMenu.go`
- Test: `ui/overlay/textInput_test.go` (existing)

**Step 1: write the failing test**

Existing `textInput_test.go` covers the text input overlay. Picker and context menu are tested through integration.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/overlay/... -run "TestTextInput" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite all three files from scratch:

**textInput.go:**
- `TextInputOverlay` struct — title, placeholder, bubbles textinput model, onSubmit/onCancel callbacks
- `NewTextInputOverlay(title, placeholder, onSubmit, onCancel)` — constructor
- `Update(msg)` — handle enter (submit), escape (cancel), delegate to textinput model
- `View()` — render bordered box with title and text input

**pickerOverlay.go:**
- `PickerOverlay` struct — title, items list, selected index, onSelect/onCancel callbacks, filter support
- `NewPickerOverlay(title, items, onSelect, onCancel)` — constructor
- `Update(msg)` — handle up/down navigation, enter (select), escape (cancel), typing for filter
- `View()` — render bordered list with highlighted selection

**contextMenu.go:**
- `ContextMenu` struct — items with labels/actions, selected index, position
- `NewContextMenu(items)` — constructor
- `Update(msg)` — handle up/down navigation, enter (select), escape (close)
- `View()` — render floating menu at position with highlighted selection

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/overlay/textInput.go ui/overlay/pickerOverlay.go ui/overlay/contextMenu.go
git commit -m "feat(clean-room): rewrite text input, picker, and context menu overlays from scratch"
```

### Task 4: Rewrite toast.go — Toast Notification System

**Files:**
- Modify: `ui/overlay/toast.go`
- Test: `ui/overlay/toast_test.go` (existing)

**Step 1: write the failing test**

Existing `toast_test.go` covers toast creation, rendering, timing, and dismissal.

**Step 2: run test to verify baseline passes**

```bash
go test ./ui/overlay/... -run "TestToast" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `ui/overlay/toast.go` from scratch:

- `ToastLevel` enum — Info, Success, Warning, Error
- `Toast` struct — message, level, duration, created time, dismissed flag
- `ToastManager` struct — queue of active toasts, max visible count
- `NewToastManager()` — constructor
- `Add(message, level, duration)` — enqueue toast
- `Tick()` — expire old toasts, return whether any changed
- `Dismiss()` — dismiss top toast
- `View(width)` — render visible toasts stacked, styled by level (color, icon)
- `HasToasts()` — check if any visible

**Step 4: run test to verify it passes**

```bash
go test ./ui/overlay/... -run "TestToast" -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add ui/overlay/toast.go
git commit -m "feat(clean-room): rewrite toast notification system from scratch"
```
