# Detect & Respond to OpenCode Permission Prompts — Design

## Problem

When opencode requests permission to access a resource (external directory, tool, etc.), it shows a "Permission required" dialog with three choices: Allow once, Allow always, Reject. The agent is blocked until the user responds. Currently kasmos has no way to detect or respond to this — the user must attach to the tmux session and manually interact with the opencode TUI.

## Solution

Detect the permission prompt in opencode's pane content, show a kasmos modal overlay mirroring the three choices (defaulting to "Allow always"), send the corresponding key sequence to the tmux pane, and cache "Allow always" decisions locally so repeat permissions auto-approve without interrupting the user.

## Detection

The metadata tick already captures pane content via `HasUpdatedWithContent()`. For opencode sessions, we parse the last ~30 lines of ANSI-stripped content looking for `"Permission required"`. When found:

1. Extract the **description** from the line after "Permission required" (strip leading `"← "`)
2. Extract the **pattern** from the first line starting with `"- "` after `"Patterns"`

These two fields are returned as a `PermissionPrompt` struct on `InstanceMetadata`.

Detection is performed in a new `session.ParsePermissionPrompt(content, program)` function alongside the existing `ParseActivity()` — same pattern, same location.

## Permission Cache

File: `~/.config/kasmos/permission-cache.json`

```json
{
  "/opt/*": "allow_always",
  "/tmp/*": "allow_always"
}
```

- Keyed by **pattern** string (stable, scoped, what opencode uses internally)
- Only `"allow_always"` entries are stored — the cache is purely an auto-approve list
- Loaded once at app startup into the `home` struct
- Written to disk on each new cache entry
- Lives in `config/` package as `PermissionCache` with `Load()` / `Save()` / `Lookup()` / `Remember()` methods

## Modal Overlay

New file: `ui/overlay/permissionOverlay.go`

```
╭──────────────────────────────────────────────╮
│  △ permission required                       │
│  access external directory /opt              │
│  pattern: /opt/*                             │
│                                              │
│  ▸ allow always    allow once    reject      │
│                                              │
│  ←→ select · enter confirm · esc dismiss     │
╰──────────────────────────────────────────────╯
```

- Three horizontal options, arrow keys navigate, Enter confirms, Esc dismisses
- Default selection: **allow always** (index 0 in the overlay, maps to the middle option in opencode)
- Border color: `colorGold` (#f6c177) — warning semantic
- Title color: `colorGold`, option highlight: `colorFoam` background
- Shows the instance title in the overlay so the user knows which agent is asking

## App State

- New state: `statePermission`
- New fields on `home`:
  - `permissionOverlay *overlay.PermissionOverlay`
  - `pendingPermissionInstance *session.Instance`
  - `permissionCache *config.PermissionCache`

## Key Sequences

Sent to the tmux pane via `tmux send-keys`:

| Choice | Keys | Rationale |
|--------|------|-----------|
| Allow always | `Right` `Enter` `Enter` | Navigate from default "Allow once" to "Allow always", confirm |
| Allow once | `Enter` | "Allow once" is already selected in opencode |
| Reject | `Right` `Right` `Enter` | Navigate past both allow options to "Reject", confirm |

New helper: `TmuxSession.SendPermissionResponse(choice)` — composes the right sequence of `tmux send-keys` calls with small delays between them.

## Flow

1. **Metadata tick** → `ParsePermissionPrompt()` finds prompt → sets `InstanceMetadata.PermissionPrompt`
2. **app.go Update** → checks `PermissionPrompt` on each metadata result:
   - **Cache hit** (pattern in cache as `allow_always`): auto-send "Allow always" keys via async `tea.Cmd`. No modal.
   - **Cache miss + `stateDefault`**: create `PermissionOverlay` with extracted description/pattern, set `statePermission`, store pending instance.
   - **Cache miss + modal already showing**: skip (don't stack modals). The permission will re-detect on next tick.
3. **User responds**:
   - **Allow always**: send keys, add pattern to cache, save cache to disk
   - **Allow once**: send keys only
   - **Reject**: send keys only
   - **Esc**: dismiss modal, do nothing. Permission re-detects on next tick, re-prompts.

## Edge Cases

- **Multiple instances hitting permissions simultaneously**: first one wins the modal; others queue implicitly (re-detected on next tick after modal dismisses)
- **Permission prompt disappears before user responds** (e.g. opencode times out): Esc the modal, detection stops since string is gone
- **Same pattern requested again after "Allow once"**: shows modal again (not cached)
- **Cache file doesn't exist**: created on first write, empty map on load failure
