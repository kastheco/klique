You are a code reviewer performing a license compliance and correctness audit on the `release/2.0` branch of the kasmos repo.

## Context

This branch contains a **clean-room rewrite** of the entire codebase to remove all AGPL-licensed code. The original codebase was forked from an AGPL project; every `.go` file touched in this branch was supposed to be rewritten from scratch under a new license. The rewrite was done across 6 plans merged in order:

1. **01a-rewrite-tmux-layer** — `session/tmux/` (adapters, monitors, pane I/O, session management)
2. **01b-rewrite-git-layer** — `session/git/` (worktree, diff, git operations)
3. **02a-rewrite-session-core** — `session/` top-level (instance, lifecycle, storage, activity, permissions, notifications)
4. **02b-rewrite-overlay-ui** — `ui/overlay/` (overlay manager, all overlay types, theme)
5. **03-rewrite-ui-panels** — `ui/` top-level (navigation, preview, diff, menu, statusbar, info/audit panes, tabbed window)
6. **04-rewrite-plumbing** — `cmd/`, `config/`, `daemon/` (CLI entry, config management, daemon)

Total: 59 commits, 89 files changed, +8004/-4662 lines.

## Your Two Tasks

### Task 1: AGPL Contamination Audit

The goal of the rewrite was to produce code that is **not derivative** of the original AGPL source. Check for:

- **Verbatim code survival**: Compare `main..release/2.0` diffs. Look for hunks where old code survived unchanged (lines that should have been rewritten but weren't touched). Focus on logic blocks, not imports/package declarations.
- **AGPL headers**: Search all `.go` files on `release/2.0` for any remaining AGPL license headers, copyright notices referencing the original project, or SPDX identifiers mentioning AGPL.
- **Structural copying**: Look for functions/methods that appear to be trivial renames of the original rather than genuine rewrites — same control flow, same variable names, same error messages.

To do this effectively:
```bash
# Get the full diff
git diff main..release/2.0

# Check for AGPL references
rg -i 'agpl|affero|gnu affero' --type go

# Check for old license headers
rg -i 'copyright.*original-author-or-project' --type go

# Look at files that had minimal changes (suspicious — should have been fully rewritten)
git diff --numstat main..release/2.0 | sort -n -k1
```

### Task 2: Correctness Review

For each rewritten file, verify:

- **API contract preservation**: Public types, methods, and interfaces must remain compatible with callers. Check that nothing was accidentally dropped or renamed in a way that breaks the app.
- **Test coverage**: New tests were added (especially in `ui/overlay/`). Verify they actually test meaningful behavior, not just compile.
- **Build verification**: Run `go build ./...` and `go test ./...` to confirm the branch compiles and tests pass.
- **Dead code**: Flag any obviously dead code, unused exports, or TODO/FIXME comments left behind.

## Output Format

Produce a structured report:

```
## AGPL Contamination Audit
- [ ] No AGPL headers found / Found in: ...
- [ ] No verbatim code blocks survived / Suspicious files: ...
- [ ] No structural copies detected / Flagged functions: ...

## Correctness Review  
- [ ] Build passes
- [ ] Tests pass (N passed, M failed)
- [ ] API compatibility preserved / Breaking changes: ...
- [ ] Dead code flagged: ...

## Verdict
PASS / FAIL with summary
```
