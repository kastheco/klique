# Spring Load-In Animation — Design

## Goal

Add a spring-physics load-in animation for the KASMOS banner on launch using `charmbracelet/harmonica`. The banner unfolds vertically from its center (0 rows → 6 rows) with a satisfying spring bounce, then the CTA line appears beneath it. The full banner + CTA remains vertically centered throughout.

## Architecture

The `PreviewPane` gains a small spring-animation state machine that runs once on launch. Harmonica's `Spring.Update(pos, vel, target)` drives a single float representing the number of visible banner rows. The existing `previewTickMsg` (50ms / 20fps) already fires on every frame — we piggyback on it to advance the spring. No new tick loops or goroutines needed.

### Spring parameters

| Param | Value | Rationale |
|---|---|---|
| FPS | `FPS(20)` | Matches the existing 50ms preview tick |
| Angular frequency | `6.0` | Brisk unfold, settles in ~0.6s |
| Damping ratio | `0.5` | Under-damped — slight overshoot + bounce |

### Animation flow

```
Launch → first fallback render
  ├─ Spring pos=0.0, vel=0.0, target=6.0
  ├─ Each tick: pos,vel = spring.Update(pos, vel, 6.0)
  ├─ visibleRows = clamp(round(pos), 0, 6)
  ├─ Show center `visibleRows` of the 6-row banner
  ├─ Banner block is vertically centered in the pane
  ├─ When |pos - 6.0| < 0.01 && |vel| < 0.01 → settled
  └─ CTA line appears after settle
```

### Row slicing (center-out)

For `visibleRows = N` out of 6 total banner lines:
- `startRow = (6 - N) / 2`
- `endRow = startRow + N`
- Show `bannerLines[startRow:endRow]`

This means the reveal is symmetric — rows appear from the center outward.

### CTA behavior

The CTA message ("create [n]ew plan or [s]elect existing") appears only after the spring settles. During animation, only the expanding banner is visible. The CTA pops in cleanly — no separate spring needed (it would over-complicate for minimal payoff).

## Component changes

### `ui/spring.go` (new)

Small struct encapsulating the spring animation state:

```go
type SpringAnim struct {
    spring    harmonica.Spring
    pos, vel  float64
    target    float64
    settled   bool
}
```

Methods: `NewSpringAnim(target)`, `Tick() bool` (returns true while animating), `VisibleRows() int`, `Settled() bool`.

### `ui/preview.go` (modify)

- Add `springAnim *SpringAnim` field to `PreviewPane`
- Initialize it in `NewPreviewPane()`
- Add `TickSpring()` method (called from app tick loop, same path as `TickBanner()`)
- In `String()` fallback branch: when spring is not settled, slice the banner to `springAnim.VisibleRows()` from center and skip the CTA

### `ui/tabbed_window.go` (modify)

- Add `TickSpring()` passthrough to preview pane (mirrors existing `TickBanner()`)

### `app/app.go` (modify)

- Call `m.tabbedWindow.TickSpring()` in the `previewTickMsg` handler, every tick (not throttled like banner dots)

### `ui/consts.go` (modify)

- Export `BannerLines() []string` — returns the pre-split, gradient-rendered lines for the current frame, so `String()` can slice them

### `go.mod` (modify)

- Add `github.com/charmbracelet/harmonica` dependency

## What doesn't change

- The dot animation (`KASMOS.` → `KASMOS..` → `KASMOS...`) continues working after spring settles — it's driven by `bannerFrame` which is independent
- Config (`AnimateBanner`) only gates the dot animation, not the spring load-in — the spring always plays on launch
- Vertical centering logic in `String()` stays the same; it already centers whatever `fallbackText` block it receives
- No new message types, no new goroutines, no config changes
