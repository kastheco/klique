# Session Context

## User Prompts

### Prompt 1

Implement docs/plans/01a-rewrite-tmux-layer.md using the `kasmos-coder` skill. Execute all tasks sequentially.

Reviewer feedback from previous round:
Round 1 — changes required.

## critical

- `session/tmux/attach.go:84-89` — detach key detection scans all bytes in buffer instead of requiring single-byte read. The old code used `nr == 1 && (buf[0] == 17 || buf[0] == 0)` which only detached when the detach key was the sole byte read. The new code `for _, b := range buf[:n]` scans every byte,...

