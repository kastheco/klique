# AGENTS.md

## Read this first

This repository already carries agent-specific guidance in a few places:

- `CLAUDE.md` — project-wide rules and UI conventions.
- `.codex/AGENTS.md` — role-specific notes for coder/reviewer/planner agents.
- `.agents/`, `.claude/`, and `.opencode/` — scaffolded harness skills/config used by `kasmos setup` and audited by `kas check`.

Key rules repeated from those files because they affect code changes here:

- Keep TUI user-facing labels lowercase.
- In overlays, prefer arrow-key navigation (`↑`/`↓`), not `j`/`k`, and let letter keys type into active search/filter inputs.
- Keep Bubble Tea I/O asynchronous: do blocking work in `tea.Cmd` goroutines and feed results back as `tea.Msg`.
- Treat task state as data in the task store, not as ad hoc files.

## What this repo is

`kasmos` is a Go application with several surfaces:

- a Bubble Tea/Lip Gloss terminal UI (`app/`, `ui/`)
- a Cobra CLI (`main.go`, `cmd/`, `check.go`)
- a multi-repo background daemon with a Unix-socket control API (`daemon/`)
- a SQLite/HTTP-backed task store (`config/taskstore/`, `config/taskstate/`)
- orchestration and prompt generation for planner/coder/reviewer agents (`orchestration/`)
- an optional Next.js site in `web/`

The main Go module is `github.com/kastheco/kasmos` (`go.mod`). The codebase uses Go 1.24.4, Bubble Tea v2, Lip Gloss v2, Cobra, `modernc.org/sqlite`, and `testify`.

## Essential commands

### Go build/test/dev

Observed wrappers:

- `make build` → `go build -o kasmos .`
- `make test` → `go test ./... -v`
- `make run ARGS='...'` → run the built binary
- `just build` → `go build -o kasmos .`
- `just test` → `go test ./...`
- `just lint` → `go vet ./...`
- `just run ...` → `go run . ...`
- `just install` → `go install .` and symlink `kas` / `kms`
- `just setup` → `kas setup --force`

Observed direct commands in docs/CI:

- `go build -o kasmos .`
- `go test ./...`
- `go test -v ./...`
- `go vet ./...`
- `gofmt -w .` (`CONTRIBUTING.md`)
- CI formatting check: `gofmt -l .` (and `gofmt -d .` on failure)

### Web app (`web/`)

Observed scripts:

- `cd web && npm ci`
- `cd web && npm run dev`
- `cd web && npm run build`
- `cd web && npm run start`
- `cd web && npm run lint`

`web/package-lock.json` exists, and GitHub Pages CI uses `npm ci`.

### Useful repo-specific CLI commands

The Cobra root command is `kas`, even though the built binary is named `kasmos` by Make/Just. Current root commands are registered in `main.go` and include:

- `kas setup`
- `kas check`
- `kas task ...`
- `kas serve`
- `kas instance ...`
- `kas audit ...`
- `kas tmux ...`
- `kas signal ...`
- `kas daemon ...`
- `kas monitor ...`
- `kas reset`
- `kas debug`
- `kas version`

Useful task/daemon commands observed in code and docs:

- `kas task list`
- `kas task show <plan-file>`
- `kas task create <name>`
- `kas task register <plan-file>`
- `kas task update-content <plan-file>` (reads from stdin)
- `kas task transition <plan-file> <event>`
- `kas task set-status <plan-file> <status> --force`
- `kas task implement <plan-file>`
- `kas serve --port 7433 --db <path>`
- `kas daemon start --foreground --config <path>`
- `kas daemon status`
- `kas monitor`
- `kas monitor status`
- `kas check -v`

## CI and release expectations

Observed CI workflows:

- `.github/workflows/build.yml` runs `go test -v ./...` and cross-builds for `linux`/`darwin` on `amd64` and `arm64`.
- `.github/workflows/lint.yml` runs `golangci-lint` and fails on `gofmt -l .` output.
- `.github/workflows/deploy-pages.yml` builds the Next.js site under `web/` and uploads `web/out`.
- `.github/workflows/release.yml` requires the pushed tag version to match `version` in `main.go`, then runs GoReleaser.

Observed release tooling:

- `.goreleaser.yaml` packages `kasmos` for `darwin`/`linux` on `amd64` and `arm64`.
- Homebrew publishing is configured through the `kastheco/homebrew-tap` cask.
- `Justfile` has a `release <version>` helper that bumps `main.go`, commits, tags, and pushes.

## Code organization

High-value directories:

- `main.go` — app entrypoint, root Cobra wiring, interactive TUI startup, setup/reset/debug/version.
- `check.go` — `kas check`, used to audit skill sync across harnesses.
- `cmd/` — CLI subcommands for tasks, daemon, monitor, signal, tmux, instances, audit, serve.
- `app/` — main Bubble Tea model/state machine and TUI orchestration glue.
- `ui/` — rendering components such as navigation, status bar, info pane, overlays.
- `session/` — instance lifecycle, storage, execution backends, tmux/headless handling, notifications, git worktrees.
- `session/git/`, `session/tmux/`, `session/headless/` — backend-specific helpers.
- `orchestration/` and `orchestration/loop/` — prompt builders, wave/task orchestration, signal scanning.
- `config/` — config loading, TOML/JSON persistence, task parser/FSM/state/store.
- `config/taskstore/` — SQLite/HTTP task store and signal gateway.
- `daemon/` — multi-repo background orchestrator and socket/API plumbing.
- `internal/initcmd/` — setup wizard and scaffold templates.
- `web/` — Next.js App Router site.
- `.agents/`, `.claude/`, `.opencode/` — agent harness configs/skills that the app scaffolds and audits.

## Coding patterns and conventions

### Go style

Observed patterns across `cmd/`, `config/`, `session/`, and `ui/`:

- Exported types/functions usually have doc comments.
- Errors are wrapped with context using `fmt.Errorf(...: %w)`.
- External-process execution is abstracted behind interfaces where practical (example: `cmd.Executor`) to keep tests hermetic.
- Go formatting is standard `gofmt` style.

### TUI/Bubble Tea conventions

Observed in `CLAUDE.md` and `app/`/`ui/`:

- Lowercase TUI copy is intentional.
- Overlay/search interactions should favor arrow keys for navigation.
- Keep blocking I/O out of synchronous update paths; use `tea.Cmd`/`tea.Msg`.
- The UI is split into nav/audit/menu/statusbar/tabbed-window/overlay components rather than one giant renderer.

### Web conventions

Observed in `web/src/app/`:

- App Router layout under `web/src/app`.
- React function components with TypeScript.
- CSS modules are used heavily (`*.module.css`).
- Client-only components are explicit via `"use client"` and, in at least one case, `next/dynamic` with `ssr: false` (`ClientApp.tsx`).
- The checked-in TS/TSX style uses double quotes, semicolons, and 2-space indentation.

## Testing approach

Observed patterns:

- Go tests are widespread throughout the repo; most packages have `_test.go` files.
- Tests use `github.com/stretchr/testify/assert` and `require`.
- Table-driven tests are common (for example in parser/UI behavior tests).
- Tests use `t.TempDir()`, `t.Setenv()`, temporary working directories, and fake executors/state instead of real tmux/git/network where possible.
- Package `TestMain` helpers initialize logging (and bubblezone in `app/`) before tests run.
- `CLAUDE.md` explicitly says not to rely on real tmux/git/network in tests.

For web code, no frontend test files were found under `web/`; only build/lint scripts are currently configured there.

## Task store and orchestration gotchas

### The repo is mid-transition from “plan” wording to “task” wording

You will see both names.

- Code and CLI use `task` (`kas task ...`, `config/taskstate`, `config/taskstore`).
- Older comments/docs/prompts still say `plan` in places.

When in doubt, follow the current CLI and package names.

### Task state lives in the task store

Observed in `.codex/AGENTS.md`, `cmd/task.go`, and `config/taskstate/taskstate.go`:

- Do not edit task state directly.
- Use `kas task ...` commands to create/register/show/update/transition work.
- `taskstate.Load` expects a store-backed world; task/task-topic metadata comes from the task store.
- Lifecycle statuses observed in code are `ready`, `planning`, `implementing`, `reviewing`, `done`, and `cancelled`.

### Config/task store paths are anchored to the main repo root

Observed in `config/config.go` and `config/taskstore/factory.go`:

- Project-local config lives under `<repo-root>/.kasmos/`.
- `GetConfigDir()` resolves the main repository root even when running inside a git worktree.
- The default local DB path is `<repo-root>/.kasmos/taskstore.db`.

### Plan/task filenames are normalized

Observed in `cmd/task.go` and `config/taskstore/sqlite.go`:

- `register` and `update-content` trim a trailing `.md` suffix.
- There is a migration that strips `.md` suffixes from stored task/subtask filenames.
- Be careful when mixing filesystem markdown files with store identifiers.

### Wave/task markdown shape matters

Observed in `config/taskparser/taskparser.go`:

- Task content is parsed from markdown.
- Header metadata uses fields like `**Goal:**`, `**Architecture:**`, and `**Tech Stack:**`.
- Implementation requires wave headers: `## Wave N` or `### Wave N`.
- Tasks are parsed from `### Task N: Title` or `#### Task N: Title`.
- The parser accepts `:`, `—`, `–`, and `-` after `Task N`.
- If there are no wave headers, parsing fails with a “no wave headers found” error.

### Topics are scheduling boundaries

Observed in `README.md` and `config/taskstate/taskstate.go`:

- Topics group related plans/tasks.
- Only one implementing plan per topic should be active at a time.

### Signals have both legacy filesystem and newer DB-backed paths

Observed in `cmd/signal.go`, `config/taskstore/signal*.go`, `orchestration/loop/gateway_scanner.go`, and `daemon/daemon.go`:

- Legacy sentinel files still exist under `.kasmos/signals`.
- There is also a SQLite-backed `signals` table with `pending` / `processing` / `done` / `failed` states.
- The daemon can bridge filesystem sentinels into the DB-backed gateway, claim them atomically, and mark them processed.
- When touching signal code, check whether you are changing the legacy file-scanner path, the DB gateway path, or both.

## Other important gotchas

- `main.go` refuses to launch the interactive app unless the current directory is inside a git repository.
- `go.mod` replaces `github.com/charmbracelet/x/vt` with the local patch at `./_patches/vt`; do not remove or “clean up” that replace directive casually.
- `web/next.config.ts` uses `output: "export"` and sets `basePath` to `/kasmos` in production; this matters for asset paths and GitHub Pages behavior.
- `kas check` audits harness skill sync from `~/.agents/skills` and project `.agents/skills` into per-harness directories; use it if you touch scaffolded skills/harness config.
- Service files for daemon/taskstore deployment live in `contrib/`.

## Practical workflow for future agents

1. Read `CLAUDE.md` and `.codex/AGENTS.md` first.
2. Use the direct Go commands or the `make`/`just` wrappers above.
3. For Go changes, run targeted tests first, then `go test ./...`, and keep `gofmt`/`go vet`/CI lint expectations in mind.
4. For web changes, use `cd web && npm run build` and `cd web && npm run lint`.
5. If your change touches task lifecycle behavior, inspect all three layers:
   - CLI (`cmd/task.go`, `cmd/signal.go`)
   - store/state/parser/FSM (`config/task*`)
   - orchestration/daemon (`orchestration/`, `daemon/`)
6. If your change touches harness skills or setup scaffolding, inspect `.agents/`, `.claude/`, `.opencode/`, and `internal/initcmd/`, then run `kas check -v`.
## Operating posture

Act like a kasmos agent in this repository: stay in role, avoid scope creep, investigate before changing code, and verify results before claiming success.

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
