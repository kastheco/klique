# Bubblezone Mouse Integration — Design

## Goal

Replace fragile positional mouse math with bubblezone-based hit detection, enable mouse-click escape from agent focus mode, and forward full SGR mouse events to the embedded PTY when focused.

## Current State

- Bubblezone is imported but barely used — only `ZoneRepoSwitch` for the repo button (1 zone in the whole UI)
- All other mouse handling uses positional math: hardcoded `contentY - 4` offsets for nav rows, `localX / tabWidth` for tab headers, `x < m.navWidth` for panel detection
- Focus mode (`stateFocusAgent`) ignores all mouse events — the `m.state != stateDefault` guard bails out
- Exit from focus mode is keyboard-only: `Ctrl+Space`, or `!`/`@`/`#` to jump to tab slots

## Design Decisions

1. **Zone-per-element approach** — mark every clickable element with `zone.Mark()`, replace positional math with `zone.Get(id).InBounds(msg)`
2. **Click outside agent pane exits focus AND performs the action atomically** — clicking the nav panel selects that panel and exits focus in one action
3. **Full SGR mouse forwarding to PTY** — left/middle/right clicks, scroll wheel, motion/drag. Makes vim, lazygit, helix etc. work fully inside the embedded terminal
4. **Position-aware scroll in focus mode** — scroll within agent pane forwards to PTY, scroll outside routes to kasmos UI without exiting focus

## Zone ID Namespace

### Static zones (always exist)

| Zone ID | Element |
|---------|---------|
| `zone-nav-panel` | Entire nav panel column |
| `zone-nav-search` | Search box |
| `zone-nav-repo` | Repo switch button (replaces `ZoneRepoSwitch`) |
| `zone-tab-agent` | Agent tab header |
| `zone-tab-diff` | Diff tab header |
| `zone-tab-info` | Info tab header |
| `zone-agent-pane` | Agent preview content area (critical for focus-mode routing) |

### Dynamic zones (per render)

| Zone ID pattern | Element |
|-----------------|---------|
| `zone-nav-row-{idx}` | Each visible row in the nav panel |

All constants in `ui/zones.go`. Dynamic IDs via `fmt.Sprintf`.

## Mouse Routing in Focus Mode

When `stateFocusAgent` is active, mouse events route based on zone hit-testing:

### Click/motion inside `zone-agent-pane`
Re-encode as SGR mouse sequence (`\x1b[<{button};{x};{y}{M|m}`), translate coordinates relative to the pane's top-left, write to `EmbeddedTerminal.SendKey()`.

Coordinate translation:
- `paneX = msg.X - zone.Get("zone-agent-pane").StartX()`
- `paneY = msg.Y - zone.Get("zone-agent-pane").StartY()`
- 1-indexed for SGR protocol

### Click outside `zone-agent-pane`
Exit focus mode, then fall through to normal click handling. One click does both.

### Scroll wheel inside `zone-agent-pane`
Forward to PTY as SGR scroll sequences (button 64/65 for wheel up/down).

### Scroll wheel outside `zone-agent-pane`
Route to kasmos UI (scroll nav, scroll preview) without exiting focus mode.

## SGR Encoding

bubbletea decodes mouse events into `tea.MouseMsg` with `Action`, `Button`, `X`, `Y`, `Shift`, `Alt`, `Ctrl`. Re-encode to SGR extended format:

```
\x1b[<{button};{col};{row}{M|m}
```

Where:
- `M` = press/motion, `m` = release
- Button byte = button number + modifier bits (shift=4, alt=8, ctrl=16)
- Button numbers: 0=left, 1=middle, 2=right, 64=wheel-up, 65=wheel-down
- Motion adds 32 to the button byte
- Coordinates are 1-indexed

Lives in `mouseToSGR(msg tea.MouseMsg, offsetX, offsetY int) []byte` in `app/mouse_sgr.go`.

## File Changes

### New files
- `ui/zones.go` — zone ID constants and `NavRowZoneID(idx int) string` helper
- `app/mouse_sgr.go` — SGR mouse encoder
- `app/mouse_sgr_test.go` — table-driven tests for SGR encoding

### Modified files

| File | Change |
|------|--------|
| `ui/navigation_panel.go` | Wrap each visible row with `zone.Mark()` in `String()`. Wrap search box and repo button. Wrap entire panel with `zone-nav-panel`. |
| `ui/tabbed_window.go` | Wrap each tab header with `zone.Mark()`. Wrap content area with `zone-agent-pane` when on PreviewTab. Remove `HandleTabClick` positional method. |
| `app/app_input.go` | Rewrite `handleMouse` with zone-based hit testing. Add focus-mode mouse routing before the state guard. PTY forwarding for in-pane clicks. Zone-aware scroll routing. |
| `app/app_input.go` | Replace `handleRightClick` positional math with zone-based row detection. |

### Not changed
- `session/terminal.go` — `SendKey([]byte)` already accepts arbitrary bytes
- Overlay code — modal overlays keep existing click handling
- Keyboard handling — completely untouched
