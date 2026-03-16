# kasmos agents

Act like a kasmos agent in this repository: stay in role, avoid scope creep, investigate before changing code, and verify results before claiming success.

## Repository

- `kasmos` is a Go 1.24.4 TUI/CLI for orchestrating AI agents in isolated git worktrees and tmux sessions.
- Main areas: `app/` TUI state, `cmd/` CLI entry points, `config/`, `daemon/`, `internal/`, `orchestration/`, `session/`, `ui/`, and `web/`.

## Shared rules

- Prefer small, targeted changes that preserve existing architecture.
- Keep user-visible labels, toasts, confirmations, and overlay titles lowercase.
- In overlays, prefer arrow-key navigation; do not introduce j/k-only behavior where typing or arrows are expected.
- Keep Bubble Tea I/O non-blocking: run I/O in `tea.Cmd` goroutines and return results as messages.
- Use table-driven tests with `testify` where appropriate. Do not rely on real tmux, git, or network behavior in tests.
- When `KASMOS_TASK` or `KASMOS_PEERS` is set, assume shared-worktree constraints and avoid sweeping or destructive changes.
- Prefer modern repo tooling and guidance: `rg`, `fd`, `sd`, `difft`, `typos`, and `scc`.

## Validation

- Run `gofmt -w` on touched Go files.
- Run `go test ./...` for Go changes.
- Run `go build ./...` when build paths, startup flow, command wiring, packaging, or release flow changes.
- For `web/**` changes, use Node 20 and run `cd web && npm ci && npm run build`.
- For release work, ensure the `version` constant in `main.go` matches any `v*` tag.

## Reviewer posture

- Check correctness and spec compliance before style.
- Cite findings with `path:line` references.
- Treat missing validation, missing tests for new logic, scaffold drift, and clear scope creep as blocking issues.

## Fixer posture

- Gather evidence first: read the failure, reproduce it, inspect the changed boundary, and then propose the smallest root-cause fix.
- Avoid speculative rewrites and broad refactors.

## Scaffold-managed files

- If you edit skills, prompts, or scaffold-managed agent files, keep the scaffold source and live copy in sync.
- `.agents/skills/...` mirrors `internal/initcmd/scaffold/templates/skills/...`.
- `internal/initcmd/scaffold/templates/{opencode,claude}/agents/...` mirrors runtime prompt files such as `.opencode/agents/...` and `.claude/agents/...`.
