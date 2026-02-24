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
| `.kittify/` | Project constitution, missions, scripts, agent rules |
| `.claude/commands/` | spec-kitty workflow slash commands (specify → merge) |

## Standards

Non-negotiable rules are in `.kittify/memory/constitution.md`. Key points:
- Go 1.24+, bubbletea v1.3.x, lipgloss v1.1.x, bubbles v0.20+
- Tests: testify assertions, table-driven, no real tmux/git/network in tests
- Non-blocking I/O: all I/O in `tea.Cmd` goroutines, results as `tea.Msg`
- Config: dual TOML (`~/.klique/config.toml`) + JSON (`~/.klique/config.json`)

## Active Work

Feature status is tracked in auto memory. Two features are in progress:

- **F001 - Fork Baseline Audit**: 6 WPs updating deps, integrating upstream PRs, fixing code quality, adding test coverage. All WPs planned, none started.
- **F002 - spec-kitty Orchestration**: Integrating klique with spec-kitty task management (WP status in TUI, role-based agent switching). All WPs planned.

See `~/.claude/projects/-home-kas-dev-klique/memory/` for full WP breakdowns.

## Workflow

spec-kitty commands in `.claude/commands/spec-kitty.*` drive the development lifecycle:
`specify` → `plan` → `tasks` → `implement` → `review` → `accept` → `merge`

The `kitty-specs/` directory is created on-demand when starting a new feature.
