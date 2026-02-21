# Rose Pine Moon Theme Fix + Focus Mode Glow — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix klique's theme to match opencode's rose-pine-moon, add a dramatic glow effect for focus mode, and fix right-arrow navigation.

**Architecture:** Add missing palette colors and update border references, create a glow column renderer, modify the main View() layout to inject glow columns, fix panel cycling.

**Tech Stack:** Go, lipgloss (truecolor backgrounds), bubbletea

---

### Task 1: Add Missing Highlight Colors to Palette

**Files:**
- Modify: `ui/theme.go`
- Modify: `ui/overlay/theme.go`

**Step 1: Add highlight colors to ui/theme.go**

Add after the `ColorText` line (line 14):

```go
// Highlight tones (from official rose-pine-moon palette)
ColorHighlightLow  = lipgloss.Color("#2a283e")
ColorHighlightMed  = lipgloss.Color("#44415a")
ColorHighlightHigh = lipgloss.Color("#56526e")
```

**Step 2: Mirror in overlay/theme.go**

Add to the overlay palette:

```go
colorHighlightMed = lipgloss.Color("#44415a")
```

**Step 3: Commit**

```bash
git add ui/theme.go ui/overlay/theme.go
git commit -m "feat(ui): add missing rose-pine-moon highlight colors"
```

---

### Task 2: Update Unfocused Borders to HighlightMed

**Files:**
- Modify: `ui/list_styles.go`
- Modify: `ui/sidebar.go`
- Modify: `ui/tabbed_window.go`

**Step 1: Update listBorderStyle in list_styles.go**

Change `BorderForeground(ColorOverlay)` to `BorderForeground(ColorHighlightMed)` in `listBorderStyle` (line 121).

**Step 2: Update sidebarBorderStyle in sidebar.go**

Change the unfocused `borderStyle.BorderForeground(ColorOverlay)` (line 319) to `borderStyle.BorderForeground(ColorHighlightMed)`.

Also update `sidebarBorderStyle` default (line 23) from `BorderForeground(ColorOverlay)` to `BorderForeground(ColorHighlightMed)`.

**Step 3: Update searchBarStyle in sidebar.go**

Change `searchBarStyle` (line 49) from `BorderForeground(ColorOverlay)` to `BorderForeground(ColorHighlightMed)`.

**Step 4: Update tabbed window unfocused border in tabbed_window.go**

Change the `default` case in the border color switch (line 333) from `ColorOverlay` to `ColorHighlightMed`.

**Step 5: Update repo button border in sidebar.go**

Change the repo button's default `borderColor` (line 468) from referencing `ColorOverlay` to `ColorHighlightMed`.

**Step 6: Build and verify**

Run: `go build ./...`
Expected: compiles cleanly

**Step 7: Commit**

```bash
git add ui/list_styles.go ui/sidebar.go ui/tabbed_window.go
git commit -m "style(ui): use highlightMed for unfocused borders to match opencode"
```

---

### Task 3: Create Glow Column Renderer

**Files:**
- Create: `ui/glow.go`

**Step 1: Write the glow renderer**

Create `ui/glow.go` with:

```go
package ui

import (
	"fmt"
	"strings"
)

// glowSteps defines the number of gradient steps in a glow column.
const glowSteps = 5

// RenderGlowColumn renders a vertical column of spaces with gradient
// backgrounds that fade from glowHex to baseHex. Width is glowSteps chars.
// If reverse is true, the gradient runs baseHex→glowHex (left side of center).
// If reverse is false, the gradient runs glowHex→baseHex (right side of center).
func RenderGlowColumn(height int, glowHex, baseHex string, reverse bool) string {
	if height <= 0 {
		return ""
	}

	gr, gg, gb := parseHex(glowHex)
	br, bg, bb := parseHex(baseHex)

	// Build one row of gradient-background spaces
	var rowBuilder strings.Builder
	for i := 0; i < glowSteps; i++ {
		// t=0 is the bright end (touching center panel), t=1 is the dark end (outer)
		t := float64(i) / float64(glowSteps-1)
		if reverse {
			t = 1.0 - t
		}
		r := lerpByte(gr, br, t)
		g := lerpByte(gg, bg, t)
		b := lerpByte(gb, bb, t)
		rowBuilder.WriteString(fmt.Sprintf("\033[48;2;%d;%d;%dm ", r, g, b))
	}
	rowBuilder.WriteString("\033[0m")
	row := rowBuilder.String()

	// Stack rows vertically
	rows := make([]string, height)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

// RenderEmptyColumn renders a vertical column of spaces with base background.
// Used when glow is inactive to reserve the same layout width.
func RenderEmptyColumn(height, width int) string {
	if height <= 0 || width <= 0 {
		return ""
	}
	row := strings.Repeat(" ", width)
	rows := make([]string, height)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}
```

**Step 2: Build and verify**

Run: `go build ./...`
Expected: compiles cleanly

**Step 3: Commit**

```bash
git add ui/glow.go
git commit -m "feat(ui): add glow column renderer for focus mode halo effect"
```

---

### Task 4: Integrate Glow Columns into Layout

**Files:**
- Modify: `app/app.go` (View function and updateHandleWindowSizeEvent)

**Step 1: Update width calculation in updateHandleWindowSizeEvent**

Subtract glow width from tabsWidth. After the existing width calculations (around line 272):

```go
glowWidth := ui.GlowColumnWidth() // returns glowSteps * 2 (both sides)
tabsWidth := msg.Width - sidebarWidth - listWidth - glowWidth
```

Store `glowWidth` on the home struct or compute inline.

**Step 2: Add TabbedWindow focusMode accessor**

The `View()` function needs to check if focus mode is active. `TabbedWindow` already has `IsFocusMode()` — use that.

**Step 3: Modify View() to inject glow columns**

Replace the current JoinHorizontal (line 520):

```go
// Determine glow state
glowActive := m.tabbedWindow.IsFocusMode()

var leftGlow, rightGlow string
glowHeight := m.contentHeight + 1 // match colStyle height
if glowActive {
	leftGlow = ui.RenderGlowColumn(glowHeight, "#9ccfd8", "#232136", true)   // fade base→foam (left of center)
	rightGlow = ui.RenderGlowColumn(glowHeight, "#9ccfd8", "#232136", false) // fade foam→base (right of center)
} else {
	leftGlow = ui.RenderEmptyColumn(glowHeight, ui.GlowSteps)
	rightGlow = ui.RenderEmptyColumn(glowHeight, ui.GlowSteps)
}

listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top,
	sidebarView, leftGlow, previewWithPadding, rightGlow, listWithPadding)
```

**Step 4: Export GlowSteps constant**

In `ui/glow.go`, export `GlowSteps` (rename from `glowSteps`) and add a helper:

```go
const GlowSteps = 5

func GlowColumnWidth() int {
	return GlowSteps * 2 // both sides
}
```

**Step 5: Build and verify**

Run: `go build ./...`
Expected: compiles cleanly

**Step 6: Commit**

```bash
git add app/app.go ui/glow.go
git commit -m "feat(ui): integrate focus mode glow columns into main layout"
```

---

### Task 5: Focus Mode Tab Label

**Files:**
- Modify: `ui/tabbed_window.go`

**Step 1: Add FOCUS indicator to active tab in focus mode**

In `TabbedWindow.String()`, around line 361, the current code skips gradient text in focus mode and just renders plain text. Change this to render with foam color and append a mode indicator:

```go
if isActive && w.focusMode {
	focusLabel := lipgloss.NewStyle().Foreground(ColorFoam).Bold(true).Render(t + "  FOCUS")
	renderedTabs = append(renderedTabs, style.Render(focusLabel))
} else if isActive {
	renderedTabs = append(renderedTabs, style.Render(GradientText(t, GradientStart, GradientEnd)))
} else {
	renderedTabs = append(renderedTabs, style.Render(t))
}
```

**Step 2: Build and verify**

Run: `go build ./...`
Expected: compiles cleanly

**Step 3: Commit**

```bash
git add ui/tabbed_window.go
git commit -m "style(ui): show FOCUS label on active tab during focus mode"
```

---

### Task 6: Fix Right Arrow Navigation

**Files:**
- Modify: `app/app_input.go`

**Step 1: Change right arrow from panel 2 to wrap to sidebar**

In `app_input.go` around line 1117, replace the focus mode entry block:

```go
case keys.KeyRight:
	// Cycle right: sidebar(0) → preview(1) → list(2) → sidebar(0) (wrap).
	next := (m.focusedPanel + 1) % 3
	m.setFocus(next)
	return m, nil
```

This removes the focus-mode entry from right arrow entirely. Focus mode remains exclusively `i`.

**Step 2: Build and verify**

Run: `go build ./...`
Expected: compiles cleanly

**Step 3: Commit**

```bash
git add app/app_input.go
git commit -m "fix(ui): right arrow wraps to sidebar instead of entering focus mode"
```

---

### Task 7: Final Verification

**Step 1: Full build**

Run: `go build ./...`

**Step 2: Run tests**

Run: `go test ./...`

**Step 3: Visual check**

Run the app and verify:
- Unfocused borders are slightly brighter (highlightMed)
- Pressing `i` on center panel shows foam glow radiating from center panel edges
- Pressing `ctrl+q` / escape exits focus mode and glow disappears
- Right arrow from instance list wraps to sidebar
- Left arrow cycles normally: list → center → sidebar
