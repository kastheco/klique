# CLI Monitoring & Observability Commands

**Goal:** Add CLI commands for querying the audit log, managing orphaned tmux sessions, and viewing instance status summaries. Closes Category 5 (Monitoring & Observability) in the UI/CLI feature parity report.

**Order:** 3 of 4 in CLI parity series. Independent of `cli-signal-processing.md` — CAN run in parallel with it (and with `cli-clickup-integration.md`). Only requires `cli-plan-lifecycle.md` to be done (for shared CLI infrastructure patterns).

**Depends on:** `cli-plan-lifecycle.md`

**Architecture:** Three independent subcommand groups:
- `kas audit` — queries the existing SQLite audit log (`config/auditlog/`). Data already exists, just needs a CLI reader.
- `kas tmux` — wraps tmux CLI to discover orphaned `kas_*` sessions, adopt them into the instance list, or kill them. Reuses logic from `app_actions.go:handleTmuxBrowserAction`.
- `kas instance status` — adds a summary subcommand showing running/ready/paused/killed counts.

Each is self-contained and testable independently.

**Tech Stack:** Go, cobra, `config/auditlog`, `session/tmux`, `config.State`

**Size:** Small-Medium (estimated ~3 hours, 3 tasks, 1 wave — all tasks are independent)

---

## Proposed Commands

```
kas audit list [--limit=N] [--event=X]      # query audit log entries
kas tmux list                               # list orphaned kas_ tmux sessions
kas tmux adopt <session-name> <title>       # adopt orphan into instance list
kas tmux kill <session-name>                # kill orphan session
kas instance status                         # summary: running/ready/paused counts
```

## Key Gaps Being Closed

| TUI Feature | TUI Location | CLI Equivalent |
|-------------|-------------|----------------|
| Audit log viewing | `config/auditlog/` SQLite | `kas audit list` |
| Tmux session browser | `app_actions.go:handleTmuxBrowserAction` | `kas tmux list` |
| Adopt orphan session | `app_actions.go:BrowserAdopt` | `kas tmux adopt` |
| Permission prompt detection | `app.go:statePermission` | Future (out of scope) |
| Instance metadata summary | `nav.GetInstances()` aggregate | `kas instance status` |
