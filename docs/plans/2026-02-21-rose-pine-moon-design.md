# Rose Pine Moon Theme Fix + Focus Mode Glow — Design

## Problem

Klique's theme claims to be rose-pine-moon but deviates from the opencode reference in two ways:

1. **Missing highlight colors** — The official palette has three highlight tones (`highlightLow`, `highlightMed`, `highlightHigh`) that opencode uses for borders, hover states, and subtle differentiation. Klique uses `overlay` (#393552) for all unfocused borders, which is darker and less visible than opencode's `highlightMed` (#44415a).

2. **Focus mode is invisible** — When the user presses `i` to enter focus mode (typing directly into the agent pane), the only visual change is a thin border color shift from iris → foam. This is easy to miss, especially on large screens.

Additionally, the right arrow key from the instance list (panel 2) enters focus mode instead of wrapping back to the sidebar — conflating navigation with mode switching.

## Design

### Part 1: Complete the Palette

Add three missing rose-pine-moon highlight colors to `ui/theme.go`:

| Name             | Hex       | Role                                    |
|------------------|-----------|-----------------------------------------|
| ColorHighlightLow  | #2a283e | borderSubtle, diff backgrounds          |
| ColorHighlightMed  | #44415a | default unfocused borders               |
| ColorHighlightHigh | #56526e | hover states, elevated elements         |

Update all unfocused border usages from `ColorOverlay` → `ColorHighlightMed`:
- `sidebarBorderStyle` (unfocused state)
- `listBorderStyle`
- `windowStyle` / inactive tab borders
- `searchBarStyle`
- Repo button border
- `ui/overlay/theme.go` mirror

`ColorOverlay` remains used for background elements (even row alternation, active-unfocused selection bg).

### Part 2: Focus Mode Glow

When focus mode is active (`focusMode == true`), render gradient glow columns on either side of the center panel in the `View()` layout.

**Visual:**
```
sidebar │      ░▒▓█║ center panel ║█▓▒░      │ list
         ^^^^^^                        ^^^^^^
         gap (base bg)                 gap (base bg)
         then fade                     then fade
         toward foam                   toward foam
```

**Specification:**
- Glow columns are ~5 chars wide per side (10 total)
- Each column cell is a space with a truecolor background
- Gradient: brightest foam at center panel edge, fading through 4-5 interpolation steps to pure base at the outer edge
- The outermost 1-2 chars are base background — visual gap that sells "glow dissipating into darkness"
- Space is permanently reserved (always present as base-bg columns) so entering/exiting focus mode causes zero layout reflow
- New file `ui/glow.go` implements `RenderGlowColumn(height, glowWidth int, color, baseColor string, reverse bool) string`

**Width accounting in `updateHandleWindowSizeEvent`:**
- Subtract glow width (10 chars) from `tabsWidth` permanently
- The glow columns are always rendered — just invisible (base bg) when not in focus mode

### Part 3: Focus Mode Tab Label

In focus mode, the active tab text renders with foam foreground and appends a small mode indicator (e.g., "FOCUS") to reinforce insert-mode state.

### Part 4: Right Arrow Fix

Change right arrow from panel 2 to wrap to panel 0 (sidebar) instead of entering focus mode. Focus mode is exclusively `i`.

- `left`: panel 2 → 1 → 0, no-op at 0
- `right`: panel 0 → 1 → 2 → 0 (wrap)
