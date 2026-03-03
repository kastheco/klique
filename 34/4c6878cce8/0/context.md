# Session Context

## User Prompts

### Prompt 1

Implement docs/plans/01a-rewrite-tmux-layer.md using the `kasmos-coder` skill. Execute all tasks sequentially.

Reviewer feedback from previous round:
Round 1 — changes required.

## important

- `session/tmux/session.go:44` — `*TmuxSession` does not satisfy the `Session` interface because `NewReset` returns `*TmuxSession` but the interface declares `NewReset(...) Session`. The interface was introduced by this plan but is immediately broken. Fix: either (a) remove `NewReset` from the `Session...

