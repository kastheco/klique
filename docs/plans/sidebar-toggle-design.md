# Sidebar Toggle (Ctrl+S)

## Summary

Add a `ctrl+s` keybind to show/hide the left sidebar. When hidden, the center tabbed panel expands to absorb the freed space. Two-step arrow-key reveal prevents disorientation.

## State

- `sidebarHidden bool` on the `home` struct
- Not persisted — sidebar starts visible on every launch

## Layout

In `updateHandleWindowSizeEvent`, when `sidebarHidden`:
- `sidebarWidth = 0`
- `tabsWidth = msg.Width - listWidth` (preview absorbs freed space)
- Still call `sidebar.SetSize()` for internal consistency

In `View()`, conditionally include the sidebar column in `lipgloss.JoinHorizontal`.

## Keybinding

`ctrl+s` maps to `KeyToggleSidebar` in `keys/keys.go`.

Behavior in `handleKeyPress`:
- Sidebar **visible + focused** (panel 0): hide sidebar, `setFocus(1)`
- Sidebar **visible + not focused**: hide sidebar, keep current focus
- Sidebar **hidden**: show sidebar, keep current focus

## Edge Cases

### Arrow key reveal (two-step)

When `focusedPanel == 1` and `sidebarHidden == true`, pressing `left`/`h`:
1. First press: reveal sidebar (`sidebarHidden = false`), fire `tea.WindowSize()` to recalculate layout, do **not** change focus
2. Second press: normal navigation, `setFocus(0)`

### `s` key (KeyFocusSidebar)

Same two-step behavior: when hidden, first press reveals without focusing. Next press navigates.

### Mouse

When hidden, `sidebarWidth == 0` so `x < m.sidebarWidth` is never true — clicks naturally fall through to the preview panel.

### ctrl+s during overlays/modal states

`ctrl+s` is not in the `GlobalKeyStringsMap` overlay bypass list, so it's ignored during all modal states (statePrompt, stateHelp, stateConfirm, etc.) via the existing early-return guard.

## Menu

Add `KeyToggleSidebar` to the system group so it's discoverable. Help text: `ctrl+s` / `toggle sidebar`.
