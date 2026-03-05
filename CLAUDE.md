# kasmos

TUI-based multi-agent orchestration IDE. Manages concurrent AI agent sessions (claude, codex, gemini, amp, etc.) in isolated git worktrees + tmux sessions. Each task gets its own branch; the TUI provides unified control over all running agents.

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `app/` | TUI application logic (bubbletea model, input handling, state) |
| `cmd/` | CLI entry points (cobra commands: `kas`, `kas task`, `kas instance`, `kas tmux`) |
| `config/` | Configuration management (TOML + JSON dual config, agent profiles) |
| `contracts/` | Shared interfaces and types |
| `daemon/` | Background daemon for auto-accept mode |
| `internal/` | Internal packages (check, clickup, initcmd, mcpclient, opencodesession, sentry) |
| `keys/` | Keybinding definitions |
| `log/` | Structured logging |
| `orchestration/` | Wave/task orchestration engine and prompt generation |
| `session/` | Instance lifecycle, storage, notifications; subpackages: `git/` (worktree ops), `tmux/` (session management) |
| `ui/` | Rendering components (navigation panel, info/audit panes, preview, statusbar, menus, overlays, theme) |
| `web/` | Web UI (public assets + source) |
| `.opencode/` | Agent configs, commands, plugins, skills for opencode harness |
| `.claude/` | Agent configs and skills for claude harness |
| `.agents/` | Shared agent skills (superpowers) |

## Standards

Key points:
- Go 1.24+, bubbletea/v2, lipgloss/v2, bubbles/v2
- Tests: testify assertions, table-driven, no real tmux/git/network in tests
- Non-blocking I/O: all I/O in `tea.Cmd` goroutines, results as `tea.Msg`
- Config: dual TOML (`<repo-root>/.kasmos/config.toml`) + JSON (`<repo-root>/.kasmos/config.json`)
- **Lowercase labels**: all user-visible text (toasts, confirmations, overlay titles, instance list titles) must be lowercase to match the app's aesthetic. No title case or sentence case — e.g. "push changes from 'foo'?" not "Push changes from 'foo'?"
- **Arrow-key navigation in overlays**: use ↑↓ for navigation, not j/k vim bindings. Letter keys should always type into search/filter when present.

## Workflow

Development follows a wave-based plan execution lifecycle. Each agent works only on the specific task it has been assigned — do not expand scope beyond your assigned work package. When `KASMOS_TASK` is set, you are one of several concurrent agents on a shared worktree. `KASMOS_WAVE` identifies your wave, `KASMOS_PEERS` the number of sibling agents. Implement only your assigned task — see your dynamic prompt for specific rules.
