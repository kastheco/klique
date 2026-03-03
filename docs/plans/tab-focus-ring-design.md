# Tab Focus Ring Design

## Problem

Arrow key navigation (`h/l/←/→`) between panels has a trap: once you enter the sidebar, there's no clean escape because `h/l` are overloaded (tree expand/collapse vs panel switch). The instance list has no rightward exit. The center pane tabs (Agent/Diff/Git) require separate keybindings to switch and a separate focus mode (`i`) to interact with.

## Solution: Flat Focus Ring

Replace the 3-panel focus model (`focusedPanel int`, 0-2) with a 5-slot focus ring (`focusSlot int`, 0-4). Tab cycles forward, Shift+Tab cycles backward. Each slot captures arrow keys for in-pane navigation.

### Focus Slots

| Slot | Pane | up/down | h/l |
|------|------|---------|-----|
| 0 | Sidebar | Navigate items | Tree expand/collapse |
| 1 | Agent tab | Scroll preview | No-op (future) |
| 2 | Diff tab | Scroll diff | Collapse/expand file sections |
| 3 | Git tab | → lazygit PTY | → lazygit PTY |
| 4 | Instance list | Navigate instances | No-op (future) |

### Navigation Keys

| Key | Action |
|-----|--------|
| Tab | Next slot (wraps 4→0) |
| Shift+Tab | Previous slot (wraps 0→4) |
| `!` (Shift+1) | Jump to slot 1 (Agent) |
| `@` (Shift+2) | Jump to slot 2 (Diff) |
| `#` (Shift+3) | Jump to slot 3 (Git) |
| `s` | Jump to slot 0 (Sidebar) |

### Removed Bindings

- `h/l/←/→` no longer switch panel focus. Repurposed for in-pane use only.
- `Shift+Up/Down` for center pane scrolling removed (use Tab-focus + up/down instead).
- `F1/F2/F3` tab switching removed (replaced by `!/@/#`).

### Two Tiers of Focus

1. **Tab-focus** (lightweight): Arrow keys captured by focused pane. Global letter keybindings (`n`, `D`, `q`, etc.) still work.
2. **Insert mode** (`i`/Enter → `stateFocusAgent`): Full keystroke forwarding to PTY. Only Ctrl+Space escapes.

### Visual Indicators

Existing Rosé Pine Moon palette, no new colors or border types:

- **Focused pane borders:** `ColorIris` (#c4a7e7). Sidebar/list use double border. Center pane uses iris border color.
- **Unfocused pane borders:** `ColorOverlay` (#393552).
- **Insert mode:** `ColorFoam` (#9ccfd8) borders (unchanged).
- **Focused center tab label:** Foam→Iris gradient (`#9ccfd8` → `#c4a7e7`).
- **Unfocused center tab labels:** `ColorMuted` (#6e6a86).

Gradient constants updated globally: `GradientStart` = `#9ccfd8` (foam), `GradientEnd` = `#c4a7e7` (iris). Affects banner animation and help title as well.

### Edge Cases

- **Startup:** `focusSlot = 4` (Instance List), same effective default as today.
- **Hidden sidebar:** Tab skips slot 0 when sidebar is hidden. Cycles 1→2→3→4→1. `s` key auto-shows sidebar and focuses it.
- **Git tab lifecycle:** Tab-focusing slot 3 spawns lazygit if not running. Leaving slot 3 kills lazygit. Same lifecycle as today.
- **Insert mode:** Tab is forwarded to PTY while in insert mode. Ctrl+Space exits to the slot you were on before entering.
- **`h/l` routing:** Slot 0 = tree ops, slot 3 = lazygit PTY, slots 1/2/4 = no-op.
