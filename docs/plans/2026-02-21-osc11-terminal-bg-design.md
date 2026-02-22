# OSC 11 Terminal Background Fix — Design

## Problem

Klique has **67 defensive `Background(ColorBase)` / `BorderBackground(ColorBase)` / `WithWhitespaceBackground(ColorBase)` declarations** across 7 UI files, all fighting the same root cause: the terminal's default background is black, so every ANSI `\033[0m` reset and every unstyled cell bleeds through as black.

This is unwinnable whack-a-mole because:
- lipgloss emits resets between styled segments
- Inline ANSI (gradient text, pre-styled glyphs) embeds `\033[0m` that clears background mid-line
- `FillBackground` can't fix intra-line gaps — only pads line endings
- `BorderBackground` is needed separately from `Background` for border cells

## Solution

Emit **OSC 11** (`\033]11;#232136\033\\`) to set the terminal's default background color to Rosé Pine Moon base (`#232136`) on startup. Restore the original on exit. This makes every ANSI reset and unstyled cell fall back to the correct color.

## Why OSC 11

| Approach | Fixes root cause | Complexity | Compatibility |
|----------|:---:|:---:|:---:|
| OSC 11 (set terminal default bg) | ✅ | ~40 lines | All modern terminals |
| Erase display with bg color | ❌ resets still bleed | ~20 lines | Wider |
| Custom renderer (rewrite resets) | ✅ | ~200 lines, fragile | Universal |

OSC 11 is used by neovim, lazygit, and other TUI apps. Supported by: kitty, alacritty, foot, wezterm, ghostty, iTerm2, Windows Terminal.

## Architecture

### New file: `ui/termbg.go`

Two exported functions:

- **`SetTerminalBackground(hexColor string) func()`** — writes OSC 11 to stdout, returns a restore closure
- Restore strategy: emit `OSC 111` (reset default bg to terminal's configured value) — simpler and more reliable than query/save/restore via `OSC 11;?`

### Integration point: `app.Run()`

```go
func Run(ctx context.Context, program string, autoYes bool) error {
    restore := ui.SetTerminalBackground("#232136")
    defer restore()
    zone.NewGlobal()
    p := tea.NewProgram(...)
    _, err := p.Run()
    return err
}
```

### Cleanup scope

**53 `Background(ColorBase)` removable** — these exist purely to fight black bleed-through. Styles that only set `Background(ColorBase)` + a foreground color can drop the background entirely.

**9 `BorderBackground(ColorBase)` removable** — border cells now fall back to `#232136` via the terminal default.

**5 `WithWhitespaceBackground(ColorBase)` removable** — whitespace in `lipgloss.Place()` calls is already the right color.

**`FillBackground`** — simplify to height-fill only (append blank lines). Width-padding becomes unnecessary.

### What stays unchanged

- All 8 `Foreground(ColorBase)` — semantic (inverted text on colored backgrounds)
- `Background(ColorSurface)` and other non-base backgrounds — intentional surface differentiation
- Overlay package `colorBase` references for shadow/foreground — semantic uses
- Overlay fade effect hardcoded colors — intentional dimming

### Files touched

| File | Changes |
|------|---------|
| `ui/termbg.go` | **New** — OSC 11 set/restore |
| `app/app.go` | Wire `SetTerminalBackground` in `Run()` |
| `ui/fill.go` | Simplify to height-fill only |
| `ui/sidebar.go` | Remove 15 `Background` + 4 `BorderBackground` + 1 `WithWhitespaceBackground` |
| `ui/list_styles.go` | Remove 12 `Background` + 1 `BorderBackground` |
| `ui/diff.go` | Remove 12 `Background` + 2 `BorderBackground` + 1 `WithWhitespaceBackground` |
| `ui/menu.go` | Remove 6 `Background` + 1 `WithWhitespaceBackground` |
| `ui/tabbed_window.go` | Remove 5 `Background` + 2 `BorderBackground` + 1 `WithWhitespaceBackground` |
| `ui/list_renderer.go` | Remove 2 `Background` + 1 `WithWhitespaceBackground` |
| `ui/preview.go` | Remove 1 `Background` |

### Risk

Low. OSC 11 is a well-established escape sequence. The restore function ensures the terminal returns to normal on exit. If a terminal doesn't support OSC 11, the worst case is the escape sequence is silently ignored and we're back to the current state (which already has all the defensive backgrounds as fallback — though we'll have removed them, so we should verify on target terminals first).

Mitigation: keep `FillBackground` height-fill as a safety net. The cleanup pass removes `Background(ColorBase)` from styles but doesn't remove the structural fill.
