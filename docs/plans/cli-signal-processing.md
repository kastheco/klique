# CLI Signal Processing Daemon

**Goal:** Extract the signal processing state machine from the TUI's `Update()` loop into a standalone package and expose it as a `kas signal` CLI command tree. This is the architectural linchpin for headless operation — without it, no autonomous agent lifecycle is possible outside the TUI. Closes 100% of Category 4 (Signal Processing) in the UI/CLI feature parity report.

**Order:** 2 of 4 in CLI parity series. Depends on `cli-plan-lifecycle.md` (plan lifecycle primitives must exist). Can NOT run in parallel with plan lifecycle. CAN run in parallel with `cli-monitoring.md` and `cli-clickup-integration.md` once plan lifecycle is done.

**Depends on:** `cli-plan-lifecycle.md`

**Architecture:** The TUI's `app.go:metadataResultMsg` handler is a ~300-line state machine that processes agent signals (planner-finished, implement-finished, review-approved, review-changes), spawns follow-up agents, manages wave orchestration, and handles permission prompts. This plan extracts that logic into a shared `orchestrator` or `engine` package that both the TUI and CLI can use. The CLI exposes it as:

- A one-shot processor (`kas signal process --once`) for cron/scripted use
- A long-running daemon (`kas signal process`) that polls for signals
- A listing command (`kas signal list`) for observability

**Tech Stack:** Go, cobra, extracted orchestrator package, `session` package for agent spawning

**Size:** Large (estimated ~8 hours, 4-5 tasks, 3 waves)

---

## Proposed Commands

```
kas signal process [--once]     # scan + process signals (loop or one-shot)
kas signal list                 # show pending signals
```

## Key Gaps Being Closed

| TUI Feature | TUI Location | CLI Equivalent |
|-------------|-------------|----------------|
| Process agent signals | `app.go:metadataResultMsg` signal loop | `kas signal process` |
| Process wave signals | `app.go:metadataResultMsg` wave signal loop | `kas signal process` |
| Auto-spawn reviewer on implement-finished | `app.go` signal handler | `kas signal process` |
| Auto-spawn coder on review-changes | `app.go` signal handler | `kas signal process` |
| List pending signals | N/A (implicit in TUI tick) | `kas signal list` |

## Architectural Notes

1. The current daemon (`daemon.go`) only does auto-yes (pressing enter on prompts). It needs to either be extended or replaced by the signal processor.
2. Concurrent access with the TUI needs coordination — if both TUI and daemon run simultaneously, they'll race on signal files. Consider file locking or a "headless mode" flag.
3. The `session` package is already CLI-ready — `Instance`, `git.GitWorktree`, and `tmux.TmuxSession` are all usable without the TUI. The extraction is primarily about the orchestration logic in `app.go`.
