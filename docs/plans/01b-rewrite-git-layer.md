# Rewrite Git Layer Implementation Plan

**Goal:** Clean-room rewrite all upstream-derived code in `session/git/` to remove AGPL-tainted lines while preserving identical public API, behavior, and passing all existing tests.

**Architecture:** Four files rewritten in-place: `worktree.go` (struct + constructors), `worktree_ops.go` (setup/cleanup/sync lifecycle), `worktree_git.go` (git CLI commands, push, PR ops), and `diff.go` (diff computation + stats). Each file is deleted and rewritten from its functional spec — no copy-paste from the original. The `GitWorktree` struct, all exported methods, and the `DiffStats` type keep their exact signatures so callers (`session/instance.go`, `session/instance_lifecycle.go`, `app/app_actions.go`, `app/app_state.go`, `main.go`) require zero changes. Files that are 100% original (`util.go`, `task_lifecycle.go`, `worktree_branch.go`) and all test files are untouched. Existing tests in `worktree_ops_test.go`, `util_test.go`, and `task_lifecycle_test.go` serve as the regression suite.

**Tech Stack:** Go 1.24, `go-git/v5` (PlainOpen, plumbing refs), `os/exec` (git + gh CLI), testify

**Size:** Medium (estimated ~2.5 hours, 4 tasks, 1 wave)

---

## Wave 1: Git Worktree Subsystem

All four files are independent at the rewrite level — they share the same package but each task rewrites a single file from its functional specification. No task depends on another task's output within this wave.

### Task 1: Rewrite worktree.go — Struct and Constructors

**Files:**
- Modify: `session/git/worktree.go`
- Test: `session/git/task_lifecycle_test.go` (existing — exercises `NewGitWorktreeFromStorage` via `NewSharedTaskWorktree`)

**Step 1: write the failing test**

No new tests needed. Existing tests exercise `NewGitWorktreeFromStorage` (via `NewSharedTaskWorktree`), and all accessors (`GetRepoPath`, `GetWorktreePath`, `GetBranchName`, `GetBaseCommitSHA`). The `NewGitWorktree` and `NewGitWorktreeOnBranch` constructors are integration-tested through `Setup()` in `TestSetupFromExistingBranch_SetsBaseCommitSHA`.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS — all existing tests green before rewrite

**Step 3: rewrite implementation**

Delete `session/git/worktree.go` and rewrite from scratch based on this spec:

- **`getWorktreeDirectory(repoPath string) (string, error)`** — returns `filepath.Join(repoPath, ".worktrees")`. Errors if `repoPath` is empty.
- **`GitWorktree` struct** — five unexported fields: `repoPath`, `worktreePath`, `sessionName`, `branchName`, `baseCommitSHA` (all strings).
- **`NewGitWorktreeFromStorage(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) *GitWorktree`** — direct field assignment constructor. Used when restoring from persisted state.
- **`NewGitWorktree(repoPath, sessionName string) (*GitWorktree, string, error)`** — loads `config.LoadConfig()` for `BranchPrefix`, concatenates prefix + sessionName, sanitizes via `sanitizeBranchName`, resolves `repoPath` to absolute via `filepath.Abs` (falls back to original on error), finds git root via `findGitRepoRoot`, gets worktree dir, builds worktree path as `<worktreeDir>/<branchName>_<hex-nanosecond-timestamp>`.
- **`NewGitWorktreeOnBranch(repoPath, sessionName, branch string) (*GitWorktree, string, error)`** — like `NewGitWorktree` but uses the provided `branch` (sanitized) instead of generating one from config prefix.
- **Accessors:** `GetWorktreePath() string`, `GetBranchName() string`, `GetRepoPath() string`, `GetRepoName() string` (returns `filepath.Base(repoPath)`), `GetBaseCommitSHA() string`.

Imports: `fmt`, `path/filepath`, `time`, `github.com/kastheco/kasmos/config`, `github.com/kastheco/kasmos/log`.

**Step 4: run test to verify it passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/git/worktree.go
git commit -m "feat(clean-room): rewrite session/git/worktree.go from scratch"
```

### Task 2: Rewrite worktree_ops.go — Setup, Cleanup, and Sync

**Files:**
- Modify: `session/git/worktree_ops.go`
- Test: `session/git/worktree_ops_test.go` (existing), `session/git/task_lifecycle_test.go` (existing)

**Step 1: write the failing test**

No new tests needed. Existing tests cover:
- `TestCleanupWorktrees_RemovesWorktreeAndBranch` — exercises `CleanupWorktrees`
- `TestSetupFromExistingBranch_SetsBaseCommitSHA` — exercises `Setup()` → `setupFromExistingBranch()` path

**Step 2: run test to verify baseline passes**

```bash
go test ./session/git/... -run "TestCleanup|TestSetup" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete `session/git/worktree_ops.go` and rewrite from scratch:

- **`Setup() error`** — creates worktrees directory via `os.MkdirAll` and checks branch existence via `go-git` `PlainOpen` + `Reference` in parallel goroutines. Dispatches to `setupFromExistingBranch()` or `setupNewWorktree()`.
- **`setupFromExistingBranch() error`** — removes stale worktree (`git worktree remove -f`), calls `syncBranchWithRemote()`, creates worktree from existing branch (`git worktree add`), resolves `baseCommitSHA` via `merge-base HEAD <branch>` (falls back to `rev-parse HEAD` in worktree).
- **`syncBranchWithRemote()`** — fetches `origin/<branch>`, compares local/remote SHAs. If identical, returns. If local is ancestor of remote, fast-forwards via `git branch -f`. If diverged, attempts `git rebase --onto`, aborts on conflict. All errors are logged, not returned (best-effort sync).
- **`setupNewWorktree() error`** — opens repo via `go-git`, calls `cleanupExistingBranch`, gets HEAD via `rev-parse`, stores as `baseCommitSHA`, creates worktree with new branch from HEAD commit (`git worktree add -b <branch> <path> <commit>`). Detects empty repos (no HEAD) with a user-friendly error.
- **`Cleanup() error`** — removes worktree (if path exists on disk), opens repo via `go-git`, removes branch reference via `Storer.RemoveReference`, prunes. Collects all errors via `errors.Join`.
- **`Remove() error`** — `git worktree remove -f` (keeps branch).
- **`Prune() error`** — `git worktree prune`.
- **`CleanupWorktrees(repoPath string) error`** — reads `.worktrees/` directory entries, lists worktrees via `git worktree list --porcelain` to map paths→branches, removes each worktree + deletes associated branch, prunes. Falls back to `os.RemoveAll` if git removal fails.

Imports: `errors`, `fmt`, `os`, `os/exec`, `path/filepath`, `strings`, `github.com/kastheco/kasmos/log`, `github.com/go-git/go-git/v5`, `github.com/go-git/go-git/v5/plumbing`.

**Step 4: run test to verify it passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/git/worktree_ops.go
git commit -m "feat(clean-room): rewrite session/git/worktree_ops.go from scratch"
```

### Task 3: Rewrite worktree_git.go — Git Commands and PR Operations

**Files:**
- Modify: `session/git/worktree_git.go`
- Test: `session/git/worktree_ops_test.go` (existing — `Setup`/`Cleanup` tests exercise `runGitCommand` transitively)

**Step 1: write the failing test**

No new tests needed. `runGitCommand` is exercised transitively by every test that calls `Setup()` or `Cleanup()`. The push/PR/commit methods require network access and are not unit-tested (integration-tested via manual flows).

**Step 2: run test to verify baseline passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete `session/git/worktree_git.go` and rewrite from scratch:

- **`runGitCommand(path string, args ...string) (string, error)`** — builds `git -C <path> <args...>` via `exec.Command`, runs `CombinedOutput`, returns stdout/stderr as string. Wraps errors with the output text.
- **`PushChanges(commitMessage string, open bool) error`** — checks `gh` CLI via `checkGHCLI()`, calls `CommitChanges`, then calls `Push`.
- **`Push(open bool) error`** — runs `git push -u origin <branchName>`. If `open` is true, calls `OpenBranchURL` (logs error on failure, does not propagate).
- **`GeneratePRBody() (string, error)`** — requires non-empty `baseCommitSHA`. Assembles markdown sections: changed files (`git diff --name-only <base>`), commit log (`git log --oneline <base>..HEAD`), diff stats (`git diff --stat <base>`). Joins non-empty sections with double newlines.
- **`CreatePR(title, body, commitMsg string) error`** — calls `PushChanges` (without browser open), runs `gh pr create --title --body --head`. If PR already exists (output contains "already exists"), opens existing PR via `gh pr view --web`. On success, opens new PR via `gh pr view --web`.
- **`CommitChanges(commitMessage string) error`** — checks `IsDirty()`, if dirty: `git add .` then `git commit -m <msg> --no-verify`.
- **`IsDirty() (bool, error)`** — `git status --porcelain`, returns true if output is non-empty.
- **`IsBranchCheckedOut() (bool, error)`** — `git branch --show-current` in `repoPath`, compares trimmed output to `branchName`.
- **`OpenBranchURL() error`** — checks `gh` CLI, runs `gh browse --branch <branchName>` in worktree directory.

Imports: `fmt`, `os/exec`, `strings`, `github.com/kastheco/kasmos/log`.

**Step 4: run test to verify it passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/git/worktree_git.go
git commit -m "feat(clean-room): rewrite session/git/worktree_git.go from scratch"
```

### Task 4: Rewrite diff.go — Diff Computation

**Files:**
- Modify: `session/git/diff.go`
- Test: `session/git/worktree_ops_test.go` (existing — `DiffStats` type used transitively)

**Step 1: write the failing test**

No new tests needed. `DiffStats` is a simple data struct used by callers (`session/instance.go`, `app/app.go`). The `Diff()` method requires a real git worktree and is integration-tested.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Delete `session/git/diff.go` and rewrite from scratch:

- **`DiffStats` struct** — four fields: `Content string` (full diff output), `Added int` (added line count), `Removed int` (removed line count), `Error error` (propagated error without breaking caller flow).
- **`(d *DiffStats) IsEmpty() bool`** — returns true when `Added == 0 && Removed == 0 && Content == ""`.
- **`(g *GitWorktree) Diff() *DiffStats`** — allocates `DiffStats{}`. Checks worktree path exists on disk via `os.Stat` (sets `Error` and returns early if gone). Gets base commit via `GetBaseCommitSHA()` (sets `Error` if empty). Runs `git --no-pager diff <base>` in worktree. Splits output by newlines, counts lines starting with `+` (excluding `+++`) as Added and lines starting with `-` (excluding `---`) as Removed. Sets `Content` to full diff output.

Imports: `fmt`, `os`, `strings`.

**Step 4: run test to verify it passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS

**Step 5: commit**

```bash
git add session/git/diff.go
git commit -m "feat(clean-room): rewrite session/git/diff.go from scratch"
```
