# Spring Load-In Animation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a harmonica spring animation that unfolds the KASMOS banner vertically from center on launch, then reveals the CTA line.

**Architecture:** A `SpringAnim` struct in `ui/spring.go` wraps `harmonica.Spring` and tracks position/velocity/settled state. The existing 50ms preview tick drives `spring.Update()` each frame. `PreviewPane.String()` slices the banner to the spring's current visible row count, centering the partial banner vertically.

**Tech Stack:** Go, charmbracelet/harmonica, charmbracelet/bubbletea, charmbracelet/lipgloss

---

## Wave 1

### Task 1: Add harmonica dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add the dependency**

Run: `go get github.com/charmbracelet/harmonica@latest`
Expected: go.mod updated with harmonica requirement, go.sum updated

**Step 2: Verify**

Run: `go build ./...`
Expected: Clean build, no errors

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add charmbracelet/harmonica for spring animation"
```

---

### Task 2: Create SpringAnim struct

**Files:**
- Create: `ui/spring.go`
- Create: `ui/spring_test.go`

**Step 1: Write the tests**

```go
package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSpringAnim(t *testing.T) {
	s := NewSpringAnim(6.0)
	require.NotNil(t, s)
	assert.Equal(t, 0, s.VisibleRows())
	assert.False(t, s.Settled())
}

func TestSpringAnim_ConvergesToTarget(t *testing.T) {
	s := NewSpringAnim(6.0)

	// Tick until settled (should settle within 30 ticks at 20fps = 1.5s max)
	for i := 0; i < 60; i++ {
		if s.Settled() {
			break
		}
		s.Tick()
	}

	assert.True(t, s.Settled(), "spring should settle within 60 ticks")
	assert.Equal(t, 6, s.VisibleRows(), "should converge to target")
}

func TestSpringAnim_VisibleRowsClamped(t *testing.T) {
	s := NewSpringAnim(6.0)

	// Tick a bunch — visible rows should never exceed 6 or go below 0
	for i := 0; i < 60; i++ {
		rows := s.VisibleRows()
		assert.GreaterOrEqual(t, rows, 0, "rows should not be negative")
		assert.LessOrEqual(t, rows, 6, "rows should not exceed target")
		s.Tick()
	}
}

func TestSpringAnim_TickReturnsFalseWhenSettled(t *testing.T) {
	s := NewSpringAnim(6.0)

	// Should return true while animating
	assert.True(t, s.Tick(), "should return true while animating")

	// Tick until settled
	for i := 0; i < 60; i++ {
		if !s.Tick() {
			break
		}
	}

	// After settling, Tick returns false
	assert.False(t, s.Tick(), "should return false after settling")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/ -run TestSpringAnim -v`
Expected: FAIL — `NewSpringAnim` not defined

**Step 3: Write the implementation**

```go
package ui

import (
	"math"

	"github.com/charmbracelet/harmonica"
)

// SpringAnim drives a spring-physics animation from 0 to a target value.
// Used for the banner load-in: target is the number of banner rows (6).
type SpringAnim struct {
	spring  harmonica.Spring
	pos     float64
	vel     float64
	target  float64
	settled bool
}

// NewSpringAnim creates a spring animation targeting the given value.
// Tuned for 20fps (50ms tick), under-damped for a satisfying bounce.
func NewSpringAnim(target float64) *SpringAnim {
	return &SpringAnim{
		spring: harmonica.NewSpring(harmonica.FPS(20), 6.0, 0.5),
		target: target,
	}
}

// Tick advances the spring by one frame. Returns true while still animating,
// false once settled.
func (s *SpringAnim) Tick() bool {
	if s.settled {
		return false
	}
	s.pos, s.vel = s.spring.Update(s.pos, s.vel, s.target)

	// Settled when close to target with negligible velocity.
	if math.Abs(s.pos-s.target) < 0.01 && math.Abs(s.vel) < 0.01 {
		s.pos = s.target
		s.vel = 0
		s.settled = true
		return false
	}
	return true
}

// VisibleRows returns the current number of visible rows, clamped to [0, target].
func (s *SpringAnim) VisibleRows() int {
	rows := int(math.Round(s.pos))
	if rows < 0 {
		return 0
	}
	maxRows := int(s.target)
	if rows > maxRows {
		return maxRows
	}
	return rows
}

// Settled returns true once the spring has reached equilibrium.
func (s *SpringAnim) Settled() bool {
	return s.settled
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./ui/ -run TestSpringAnim -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add ui/spring.go ui/spring_test.go
git commit -m "feat: add SpringAnim struct wrapping harmonica spring"
```

---

### Task 3: Export banner lines for slicing

**Files:**
- Modify: `ui/consts.go`
- Create or modify: `ui/consts_test.go` (if it exists)

**Step 1: Write the test**

Add a test that `BannerLines` returns the expected number of lines for frame 0:

```go
func TestBannerLines_ReturnsCorrectRowCount(t *testing.T) {
	lines := BannerLines(0)
	assert.Equal(t, 6, len(lines), "banner should have 6 rows")
}

func TestBannerLines_FrameWraps(t *testing.T) {
	// Should not panic on any frame index
	for i := 0; i < 20; i++ {
		lines := BannerLines(i)
		assert.Equal(t, 6, len(lines))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./ui/ -run TestBannerLines -v`
Expected: FAIL — `BannerLines` not defined

**Step 3: Implement BannerLines**

Add to `ui/consts.go`:

```go
// BannerLines returns the pre-rendered gradient banner as individual lines
// for the given animation frame. Always returns exactly 6 lines.
func BannerLines(frame int) []string {
	banner := bannerFrames[frame%len(bannerFrames)]
	return strings.Split(banner, "\n")
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./ui/ -run TestBannerLines -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add ui/consts.go ui/consts_test.go
git commit -m "feat: export BannerLines for row-level banner slicing"
```

---

## Wave 2

### Task 4: Wire spring into PreviewPane

**Files:**
- Modify: `ui/preview.go`
- Modify: `ui/tabbed_window.go`

**Step 1: Add spring field and tick method to PreviewPane**

In `ui/preview.go`, add `springAnim *SpringAnim` field to the `PreviewPane` struct.

Initialize in `NewPreviewPane()`:

```go
func NewPreviewPane() *PreviewPane {
	return &PreviewPane{
		viewport:   viewport.New(0, 0),
		springAnim: NewSpringAnim(6.0),
	}
}
```

Add passthrough method:

```go
// TickSpring advances the spring load-in animation by one frame.
func (p *PreviewPane) TickSpring() {
	if p.springAnim != nil {
		p.springAnim.Tick()
	}
}
```

**Step 2: Add passthrough in TabbedWindow**

In `ui/tabbed_window.go`, add next to the existing `TickBanner` method:

```go
// TickSpring advances the preview pane's spring load-in animation.
func (w *TabbedWindow) TickSpring() {
	w.preview.TickSpring()
}
```

**Step 3: Run full test suite to verify nothing breaks**

Run: `go test ./ui/ -v`
Expected: All PASS

**Step 4: Commit**

```bash
git add ui/preview.go ui/tabbed_window.go
git commit -m "feat: wire spring animation into PreviewPane and TabbedWindow"
```

---

### Task 5: Render spring-animated banner in String()

**Files:**
- Modify: `ui/preview.go`

This is the core rendering change. In `PreviewPane.String()`, the fallback branch currently builds the full banner. We modify it to slice the banner when the spring hasn't settled.

**Step 1: Modify the fallback rendering in String()**

Replace the banner-mode block inside `if p.previewState.fallbackMsg != ""` with spring-aware logic:

```go
if p.previewState.fallbackMsg != "" {
	bannerLines := BannerLines(p.bannerFrame)

	// Spring load-in: show center N rows during animation
	if p.springAnim != nil && !p.springAnim.Settled() {
		visibleRows := p.springAnim.VisibleRows()
		if visibleRows <= 0 {
			// Nothing visible yet — show empty space
			fallbackText = ""
		} else {
			totalRows := len(bannerLines)
			startRow := (totalRows - visibleRows) / 2
			endRow := startRow + visibleRows
			if startRow < 0 {
				startRow = 0
			}
			if endRow > totalRows {
				endRow = totalRows
			}
			fallbackText = strings.Join(bannerLines[startRow:endRow], "\n")
		}
	} else {
		// Spring settled (or nil) — full banner with CTA
		banner := strings.Join(bannerLines, "\n")
		bannerWidth := lipgloss.Width(bannerLines[0])
		ctaWidth := lipgloss.Width(p.previewState.fallbackMsg)
		ctaPad := (bannerWidth - ctaWidth) / 2
		if ctaPad < 0 {
			ctaPad = 0
		}
		centeredCTA := strings.Repeat(" ", ctaPad) + p.previewState.fallbackMsg
		fallbackText = lipgloss.JoinVertical(lipgloss.Left, banner, "", centeredCTA)
	}
}
```

Also update the bare `else` branch (no fallbackMsg, no text) similarly — when the spring hasn't settled, show partial banner without going through `FallBackText`:

```go
} else {
	bannerLines := BannerLines(p.bannerFrame)
	if p.springAnim != nil && !p.springAnim.Settled() {
		visibleRows := p.springAnim.VisibleRows()
		if visibleRows <= 0 {
			fallbackText = ""
		} else {
			totalRows := len(bannerLines)
			startRow := (totalRows - visibleRows) / 2
			endRow := startRow + visibleRows
			if startRow < 0 {
				startRow = 0
			}
			if endRow > totalRows {
				endRow = totalRows
			}
			fallbackText = strings.Join(bannerLines[startRow:endRow], "\n")
		}
	} else {
		fallbackText = FallBackText(p.bannerFrame)
	}
}
```

**Step 2: Run tests**

Run: `go test ./ui/ -v`
Expected: All PASS

**Step 3: Run the app to verify visually**

Run: `go build -o kasmos . && ./kasmos`
Expected: On launch, the banner expands from center (1 row → 6 rows) with a spring bounce, then the CTA appears below it. Vertically centered throughout.

**Step 4: Commit**

```bash
git add ui/preview.go
git commit -m "feat: render spring-animated banner unfold from center"
```

---

### Task 6: Drive spring from app tick loop

**Files:**
- Modify: `app/app.go`

**Step 1: Add TickSpring call in previewTickMsg handler**

In `app/app.go`, inside the `case previewTickMsg:` handler (around line 416), add a call to `TickSpring()` — this should run every tick (not throttled like the banner dots):

```go
case previewTickMsg:
	cmd := m.instanceChanged()
	// Advance spring animation every tick (20fps)
	m.tabbedWindow.TickSpring()
	// Advance banner dot animation every 20 ticks (~1s per frame at 50ms tick)
	m.previewTickCount++
	if m.previewTickCount%20 == 0 {
		m.tabbedWindow.TickBanner()
	}
```

**Step 2: Build and verify**

Run: `go build -o kasmos . && ./kasmos`
Expected: Spring animation plays on launch — banner unfolds from center with bounce, CTA appears after settling.

**Step 3: Run full test suite**

Run: `go test ./... -count=1`
Expected: All PASS

**Step 4: Commit**

```bash
git add app/app.go
git commit -m "feat: drive spring animation from preview tick loop"
```

---

## Wave 3

### Task 7: Verify and clean up

**Step 1: Run full build + tests**

Run: `go build ./... && go test ./... -count=1`
Expected: Clean build, all tests pass

**Step 2: Run typos check**

Run: `typos ui/spring.go ui/preview.go ui/consts.go app/app.go`
Expected: No typos

**Step 3: Visual verification**

Run: `go build -o kasmos . && ./kasmos`
Expected:
- Banner unfolds from center with spring bounce on launch (~0.6s)
- CTA line appears after banner settles
- Banner + CTA are vertically centered in the preview pane
- Dot animation works normally after spring settles (if `animate_banner` is enabled)
- Selecting an instance clears the banner as before
- Returning to no-selection shows the full banner (no re-animation — spring already settled)

**Step 4: Final commit if any cleanup was needed**

```bash
git add -A
git commit -m "chore: spring animation cleanup"
```
