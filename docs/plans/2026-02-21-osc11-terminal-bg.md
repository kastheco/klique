# OSC 11 Terminal Background Fix — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Set the terminal's default background to `#232136` via OSC 11 on startup so every ANSI reset and unstyled cell falls back to the correct Rosé Pine Moon base color, then remove all 67 defensive `Background(ColorBase)` declarations that exist solely to fight black bleed-through.

**Architecture:** Emit `\033]11;#232136\033\\` before bubbletea starts, `\033]111\033\\` (reset to default) on exit. Then clean up all styles that only set `Background(ColorBase)` as a workaround. Simplify `FillBackground` to height-fill only.

**Tech Stack:** Go, bubbletea v1, lipgloss, ANSI OSC escape sequences

**Design doc:** `docs/plans/2026-02-21-osc11-terminal-bg-design.md`

---

### Task 1: Create `ui/termbg.go` — OSC 11 set/restore

**Files:**
- Create: `ui/termbg.go`
- Test: `ui/termbg_test.go`

**Step 1: Write the test**

```go
// ui/termbg_test.go
package ui

import (
	"bytes"
	"testing"
)

func TestSetTerminalBackground_EmitsOSC11(t *testing.T) {
	var buf bytes.Buffer
	restore := setTermBg(&buf, "#232136")
	got := buf.String()
	want := "\033]11;#232136\033\\"
	if got != want {
		t.Errorf("OSC 11 set: got %q, want %q", got, want)
	}

	buf.Reset()
	restore()
	got = buf.String()
	want = "\033]111\033\\"
	if got != want {
		t.Errorf("OSC 111 restore: got %q, want %q", got, want)
	}
}

func TestSetTerminalBackground_InvalidColor(t *testing.T) {
	var buf bytes.Buffer
	restore := setTermBg(&buf, "")
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty color, got %q", buf.String())
	}
	restore() // should not panic
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./ui/ -run TestSetTerminalBackground -v`
Expected: FAIL — `setTermBg` not defined

**Step 3: Write the implementation**

```go
// ui/termbg.go
package ui

import (
	"fmt"
	"io"
	"os"
)

// SetTerminalBackground emits OSC 11 to set the terminal's default background
// color. Returns a function that restores the original default via OSC 111.
// This makes every ANSI reset (\033[0m) fall back to the specified color
// instead of the terminal's configured default (usually black).
//
// Supported by: kitty, alacritty, foot, wezterm, ghostty, iTerm2,
// Windows Terminal, and most modern terminal emulators.
func SetTerminalBackground(hexColor string) func() {
	return setTermBg(os.Stdout, hexColor)
}

// setTermBg is the testable core — writes to the given writer instead of stdout.
func setTermBg(w io.Writer, hexColor string) func() {
	if hexColor == "" {
		return func() {}
	}
	// OSC 11 ; <color> ST — set default background color
	fmt.Fprintf(w, "\033]11;%s\033\\", hexColor)

	return func() {
		// OSC 111 ST — reset default background to terminal's configured value
		fmt.Fprint(w, "\033]111\033\\")
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./ui/ -run TestSetTerminalBackground -v`
Expected: PASS

**Step 5: Commit**

```
feat(ui): add OSC 11 terminal background set/restore

Emit OSC 11 to set the terminal's default background color so every
ANSI reset falls back to the theme color instead of black.
```

---

### Task 2: Wire OSC 11 into `app.Run()`

**Files:**
- Modify: `app/app.go:25-34` (the `Run` function)

**Step 1: Add the OSC 11 call before bubbletea starts**

Change `app/app.go` `Run()` from:

```go
func Run(ctx context.Context, program string, autoYes bool) error {
	zone.NewGlobal()
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)
	_, err := p.Run()
	return err
}
```

To:

```go
func Run(ctx context.Context, program string, autoYes bool) error {
	// Set the terminal's default background to the theme base color so every
	// ANSI reset and unstyled cell falls back to #232136 instead of black.
	restore := ui.SetTerminalBackground(string(ui.ColorBase))
	defer restore()

	zone.NewGlobal()
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)
	_, err := p.Run()
	return err
}
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: success

**Step 3: Manual smoke test**

Run: `go run . 2>/dev/null` (in a git repo)
Expected: the background behind all panels, gaps, and reset points should be `#232136` (Rosé Pine Moon base) instead of black. The sidebar plan glyph text, right edge strip, and overlay should all have correct backgrounds.

**Step 4: Commit**

```
feat(ui): wire OSC 11 terminal background into app startup

Sets terminal default bg to #232136 before bubbletea starts and
restores on exit. This is the root fix for all background bleed-through.
```

---

### Task 3: Remove defensive `Background(ColorBase)` from `app/app.go`

**Files:**
- Modify: `app/app.go:529`

**Step 1: Remove `Background(ColorBase)` from `colStyle`**

Change line 529 from:
```go
colStyle := lipgloss.NewStyle().PaddingTop(1).Height(m.contentHeight + 1).Background(ui.ColorBase)
```
To:
```go
colStyle := lipgloss.NewStyle().PaddingTop(1).Height(m.contentHeight + 1)
```

**Step 2: Verify it compiles**

Run: `go build ./...`

**Step 3: Commit**

```
refactor(ui): remove defensive Background(ColorBase) from app colStyle

OSC 11 handles the terminal default background now.
```

---

### Task 4: Clean up `ui/sidebar.go` styles

**Files:**
- Modify: `ui/sidebar.go`

**Step 1: Remove `Background(ColorBase)` and `BorderBackground(ColorBase)` from sidebar styles**

These styles need `Background(ColorBase)` removed (it's now the terminal default):

| Line | Style | Remove |
|------|-------|--------|
| 25-26 | `sidebarBorderStyle` | `BorderBackground(ColorBase)` and `Background(ColorBase)` |
| 31 | `topicItemStyle` | `Background(ColorBase)` |
| 48 | `sectionHeaderStyle` | `Background(ColorBase)` |
| 54-55 | `searchBarStyle` | `BorderBackground(ColorBase)` and `Background(ColorBase)` |
| 61-62 | `searchActiveBarStyle` | `BorderBackground(ColorBase)` and `Background(ColorBase)` |
| 80 | `dimmedTopicStyle` | `Background(ColorBase)` |
| 84 | `sidebarRunningStyle` | `Background(ColorBase)` |
| 88 | `sidebarReadyStyle` | `Background(ColorBase)` |
| 92 | `sidebarNotifyStyle` | `Background(ColorBase)` |

Also remove from dynamic styles:
| Line | Context | Remove |
|------|---------|--------|
| 519-520 | `btnStyle` in `String()` | `BorderBackground(ColorBase)` and `Background(ColorBase)` |
| 551 | `lipgloss.Place()` | `lipgloss.WithWhitespaceBackground(ColorBase)` arg |

**Keep unchanged:**
- Line 19: `Foreground(ColorBase)` on `sidebarTitleStyle` — semantic (inverted text)
- Line 38: `Foreground(ColorBase)` on `selectedTopicStyle` — semantic
- Lines 37, 43: `Background(ColorIris)`, `Background(ColorOverlay)` — non-base backgrounds

**Step 2: Verify it compiles and tests pass**

Run: `go build ./... && go test ./ui/ -v`

**Step 3: Commit**

```
refactor(ui): remove 20 defensive Background(ColorBase) from sidebar

OSC 11 sets the terminal default background; these are no longer needed.
```

---

### Task 5: Clean up `ui/list_styles.go`

**Files:**
- Modify: `ui/list_styles.go`

**Step 1: Remove `Background(ColorBase)` from list styles**

Remove from these styles:

| Line | Style | Remove |
|------|-------|--------|
| 11 | `readyStyle` | `Background(ColorBase)` |
| 15 | `notifyStyle` | `Background(ColorBase)` |
| 19 | `addedLinesStyle` | `Background(ColorBase)` |
| 23 | `removedLinesStyle` | `Background(ColorBase)` |
| 27 | `pausedStyle` | `Background(ColorBase)` |
| 32 | `titleStyle` | `Background(ColorBase)` |
| 37 | `listDescStyle` | `Background(ColorBase)` |
| 80 | `resourceStyle` | `Background(ColorBase)` |
| 84 | `activityStyle` | `Background(ColorBase)` |
| 124 | `sortDropdownStyle` | `Background(ColorBase)` |
| 132-133 | `listBorderStyle` | `BorderBackground(ColorBase)` and `Background(ColorBase)` |

**Keep unchanged:**
- Lines 42, 47: `Background(ColorSurface)` — intentional alternate row color
- Lines 52, 57: `Background(ColorIris)` + `Foreground(ColorBase)` — selected item inversion
- Lines 63, 68: `Background(ColorOverlay)` — active unfocused style
- Lines 72, 77: `Background(ColorIris/ColorGold)` + `Foreground(ColorBase)` — badge styles
- Lines 89, 94: `Background(ColorIris/ColorOverlay)` — filter tab styles

**Step 2: Verify**

Run: `go build ./... && go test ./ui/ -v`

**Step 3: Commit**

```
refactor(ui): remove 13 defensive Background(ColorBase) from list styles

OSC 11 handles terminal default background.
```

---

### Task 6: Clean up `ui/diff.go`

**Files:**
- Modify: `ui/diff.go`

**Step 1: Remove `Background(ColorBase)` from diff styles**

| Line | Style | Remove |
|------|-------|--------|
| 15 | `AdditionStyle` | `Background(ColorBase)` |
| 16 | `DeletionStyle` | `Background(ColorBase)` |
| 17 | `HunkStyle` | `Background(ColorBase)` |
| 20 | `fileItemStyle` | `Background(ColorBase)` |
| 27 | `fileItemDimStyle` | `Background(ColorBase)` |
| 32-33 | `filePanelBorderStyle` | `BorderBackground(ColorBase)` and `Background(ColorBase)` |
| 37-38 | `filePanelBorderFocusedStyle` | `BorderBackground(ColorBase)` and `Background(ColorBase)` |
| 40 | `diffHeaderStyle` | `Background(ColorBase)` |
| 44 | `diffHintStyle` | `Background(ColorBase)` |
| 175 | `lipgloss.Place()` | `lipgloss.WithWhitespaceBackground(ColorBase)` arg |

**Keep unchanged:**
- Line 24: `Foreground(ColorBase)` on `fileItemSelectedStyle` — semantic

**Step 2: Verify**

Run: `go build ./... && go test ./ui/ -v`

**Step 3: Commit**

```
refactor(ui): remove 15 defensive Background(ColorBase) from diff styles

OSC 11 handles terminal default background.
```

---

### Task 7: Clean up `ui/menu.go`

**Files:**
- Modify: `ui/menu.go`

**Step 1: Remove `Background(ColorBase)` from menu styles**

| Line | Style | Remove |
|------|-------|--------|
| 12 | `keyStyle` | `Background(ColorBase)` |
| 14 | `descStyle` | `Background(ColorBase)` |
| 16 | `sepStyle` | `Background(ColorBase)` |
| 18 | `actionGroupStyle` | `Background(ColorBase)` |
| 24 | `menuStyle` | `Background(ColorBase)` |
| 243 | `lipgloss.Place()` | `lipgloss.WithWhitespaceBackground(ColorBase)` arg |

**Step 2: Verify**

Run: `go build ./... && go test ./ui/ -v`

**Step 3: Commit**

```
refactor(ui): remove 7 defensive Background(ColorBase) from menu styles

OSC 11 handles terminal default background.
```

---

### Task 8: Clean up `ui/tabbed_window.go`

**Files:**
- Modify: `ui/tabbed_window.go`

**Step 1: Remove `Background(ColorBase)` and `BorderBackground(ColorBase)` from tab/window styles**

| Line | Style | Remove |
|------|-------|--------|
| 23-24 | `inactiveTabStyle` | `BorderBackground(ColorBase)` and `Background(ColorBase)` |
| 32-33 | `windowStyle` | `BorderBackground(ColorBase)` and `Background(ColorBase)` |
| 394 | `lipgloss.Place()` in `String()` | `lipgloss.WithWhitespaceBackground(ColorBase)` arg |

Note: `activeTabStyle` inherits from `inactiveTabStyle` (line 26), so cleaning `inactiveTabStyle` fixes both.

**Step 2: Verify**

Run: `go build ./... && go test ./ui/ -v`

**Step 3: Commit**

```
refactor(ui): remove 5 defensive Background(ColorBase) from tabbed window

OSC 11 handles terminal default background.
```

---

### Task 9: Clean up `ui/list_renderer.go` and `ui/preview.go`

**Files:**
- Modify: `ui/list_renderer.go:244,288`
- Modify: `ui/preview.go:13`

**Step 1: Clean up list_renderer.go**

Line 244 — change:
```go
gapFill := lipgloss.NewStyle().Background(ColorBase)
```
To:
```go
gapFill := lipgloss.NewStyle()
```

Line 288 — remove `lipgloss.WithWhitespaceBackground(ColorBase)` from the `lipgloss.Place()` call:
```go
return lipgloss.Place(l.width, l.height, lipgloss.Right, lipgloss.Top, bordered)
```

**Step 2: Clean up preview.go**

Line 12-14 — change:
```go
var previewPaneStyle = lipgloss.NewStyle().
	Background(ColorBase).
	Foreground(ColorText)
```
To:
```go
var previewPaneStyle = lipgloss.NewStyle().
	Foreground(ColorText)
```

**Step 3: Verify**

Run: `go build ./... && go test ./ui/ -v`

**Step 4: Commit**

```
refactor(ui): remove remaining Background(ColorBase) from list renderer and preview

OSC 11 handles terminal default background.
```

---

### Task 10: Simplify `ui/fill.go`

**Files:**
- Modify: `ui/fill.go`

**Step 1: Simplify FillBackground to height-fill only**

The width-padding logic is no longer needed since unstyled spaces are already `#232136`. Keep height-fill as a structural safety net (bubbletea needs the right number of lines to avoid scrolling artifacts).

Change `ui/fill.go` to:

```go
package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// FillBackground ensures the output has at least `height` lines so bubbletea's
// alt-screen renderer doesn't leave stale content below the rendered view.
// Width-padding is no longer needed because OSC 11 sets the terminal's default
// background to the theme base color — unstyled cells are already correct.
func FillBackground(s string, width, height int, bg lipgloss.TerminalColor) string {
	if height <= 0 {
		return s
	}

	lines := strings.Split(s, "\n")

	// Extend to target height with blank lines.
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
```

Note: keep the function signature unchanged (`width` and `bg` params) so callers don't need updating. They're just unused now.

**Step 2: Verify**

Run: `go build ./... && go test ./... -v`

**Step 3: Commit**

```
refactor(ui): simplify FillBackground to height-fill only

Width-padding is no longer needed now that OSC 11 sets the terminal
default background to the theme base color.
```

---

### Task 11: Final verification and smoke test

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: all pass

**Step 2: Run linter**

Run: `go vet ./...`
Expected: clean

**Step 3: Manual smoke test checklist**

Run: `go build -o kq . && ./kq`

Verify these previously-broken areas:
- [ ] Sidebar plan name text — no black behind characters after status glyphs (○/●/◉)
- [ ] Right edge — no black strip at far right of terminal
- [ ] Repo switcher overlay — correct background (no black bleed)
- [ ] Menu bar — no black between separator dots
- [ ] Tab borders — no black in border cells
- [ ] Diff pane — no black behind diff text
- [ ] Resize terminal — background stays correct
- [ ] Exit klique — terminal background restores to normal

**Step 4: Commit (if any fixups needed)**

```
fix(ui): address smoke test findings from OSC 11 migration
```
