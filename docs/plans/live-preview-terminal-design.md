# Live Preview Terminal Design

## Problem

The preview pane renders agent output by polling `tmux capture-pane` every 50ms via the metadata tick. This causes visible latency, stale-frame flickering on input, and broken animations (e.g. OpenCode's loading spinner). Focus mode bypasses this entirely with an `EmbeddedTerminal` (PTY → VT emulator → render cache) and is smooth — but requires explicitly entering focus mode.

## Decision

**Option A — Single trailing EmbeddedTerminal.** One `previewTerminal` lives on the `home` struct for the currently selected instance. The existing `embeddedTerminal` (focus mode) field is eliminated; focus mode reuses `previewTerminal` by forwarding keys to it.

Rejected alternatives:
- **Pre-warm pool**: keeps N+1 attach processes alive, complex lifecycle for marginal gain.
- **Lazy attach with capture-pane fallback**: keeps dead code alive, adds pending/live/closed state machine.

## Architecture

### Single `previewTerminal` field

A `previewTerminal *session.EmbeddedTerminal` replaces `embeddedTerminal`. It is the sole terminal for both preview rendering and focus mode input.

- **Attach**: On selection change to a running instance, spawn `previewTerminal` asynchronously in a `tea.Cmd`. Preview shows a spinner until `previewTerminalReadyMsg` arrives.
- **Focus mode (`i`)**: Sets `stateFocusAgent`, starts forwarding keys to `previewTerminal.SendKey()`. No new process, no reattach — zero-latency transition.
- **Exit focus**: Stops forwarding keys. `previewTerminal` stays alive, preview keeps rendering.
- **Selection change**: Close old terminal, spawn new one. Stale ready messages discarded by title check.
- **Instance dies/pauses**: Close terminal, show fallback content.
- **Resize**: `previewTerminal.Resize()` on `WindowSizeMsg`.

### Render loop unification

The `previewTickMsg` handler switches from `capture-pane` to `previewTerminal.Render()`. Uses `WaitForRender(50ms)` for event-driven wakeup (same as focus mode today). `focusPreviewTickMsg` is eliminated — both normal and focus mode share one render path. The only difference is key forwarding.

### Selection change lifecycle

Track `previewTerminalInstance string` (title of attached instance). On `instanceChanged()`:

1. Compare current selection title to `previewTerminalInstance`.
2. If different: close old terminal, nil the pointer (spinner shows), fire async spawn cmd.
3. `previewTerminalReadyMsg{term, title}` handler: check title matches current selection (discard stale), store terminal.
4. Next render tick picks it up immediately.

Rapid switching is safe — each switch closes the previous and starts a new attach. Stale ready messages are discarded.

### Dead code removal

With `previewTerminal` providing live content, the following capture-pane preview paths become dead code and are removed:

- `PreviewPane.UpdateContent()` — the `capture-pane` → content path
- `Instance.PreviewCached()` / `CachedContent` field usage for preview rendering
- `TabbedWindow.UpdatePreview()` for agent tab content
- The `focusPreviewTickMsg` type and its handler

The metadata tick continues running for status detection, activity hashing, and other non-preview purposes. `capture-pane` calls in `tmux_io.go` stay since they serve those use cases, but the preview pane no longer depends on them.

## Constraints

- Read-only in normal mode. Input only forwarded in `stateFocusAgent`.
- One `tmux attach-session` process per selected instance (not per running instance).
- Async spawn — UI never blocks on attach (~100-200ms).
- All terminal I/O in goroutines, `Render()` holds mutex for microseconds only.
