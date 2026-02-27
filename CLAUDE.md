# klique

TUI-based multi-agent orchestration IDE. Manages concurrent AI agent sessions (claude, codex, gemini, amp, etc.) in isolated git worktrees + tmux sessions. Each task gets its own branch; the TUI provides unified control over all running agents.

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `app/` | TUI application logic (bubbletea model, input handling, state) |
| `config/` | Configuration management (TOML + JSON dual config, agent profiles) |
| `session/` | Instance lifecycle, git worktree ops, tmux session management |
| `ui/` | Rendering components (list, sidebar, preview, diff, overlay) |
| `cmd/` | CLI entry points (cobra commands) |
| `daemon/` | Background daemon for auto-accept mode |
| `internal/initcmd/` | `kq init` multi-harness setup wizard |
| `.claude/commands/` | Custom slash commands for agent workflows |

## Standards

Key points:
- Go 1.24+, bubbletea v1.3.x, lipgloss v1.1.x, bubbles v0.20+
- Tests: testify assertions, table-driven, no real tmux/git/network in tests
- Non-blocking I/O: all I/O in `tea.Cmd` goroutines, results as `tea.Msg`
- Config: dual TOML (`~/.klique/config.toml`) + JSON (`~/.klique/config.json`)
- **Lowercase labels**: all user-visible text (toasts, confirmations, overlay titles, instance list titles) must be lowercase to match the app's aesthetic. No title case or sentence case — e.g. "push changes from 'foo'?" not "Push changes from 'foo'?"
- **Arrow-key navigation in overlays**: use ↑↓ for navigation, not j/k vim bindings. Letter keys should always type into search/filter when present.

## Active Work

Feature status is tracked in auto memory. Two features are in progress:

- **F001 - Fork Baseline Audit**: 6 WPs updating deps, integrating upstream PRs, fixing code quality, adding test coverage. All WPs planned, none started.
- **F002 - Agent Orchestration**: WP status in TUI, role-based agent switching. All WPs planned.

See `~/.claude/projects/-home-kas-dev-klique/memory/` for full WP breakdowns.

## Workflow

Development follows a wave-based plan execution lifecycle. Each agent works only on the specific task it has been assigned — do not expand scope beyond your assigned work package. When `KASMOS_TASK` is set, you are one of several concurrent agents on a shared worktree. `KASMOS_WAVE` identifies your wave, `KASMOS_PEERS` the number of sibling agents. Implement only your assigned task — see your dynamic prompt for specific rules.
