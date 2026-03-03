# Rewrite Git Layer Implementation Plan

**Goal:** Clean-room rewrite all upstream-derived code in `session/git/` (worktree.go, worktree_ops.go, worktree_git.go, diff.go) to remove AGPL-tainted lines. The rewrite preserves identical public API and behavior while replacing every line tracing back to the fork point.

**Architecture:** Four files rewritten in-place. The `GitWorktree` struct, all exported methods, and the `DiffStats` type keep their exact signatures. Internal helpers (`runGitCommand`, `getWorktreeDirectory`, `cleanupExistingBranch`, `syncBranchWithRemote`) are reimplemented from functional specs. Files that are 100% original (util.go, plan_lifecycle.go, worktree_branch.go) are untouched. Existing tests in `worktree_ops_test.go`, `util_test.go`, and `plan_lifecycle_test.go` serve as the regression suite.

**Tech Stack:** Go 1.24, go-git/v5, os/exec (git CLI), testify

**Size:** Medium (estimated ~2.5 hours, 4 tasks, 1 wave)

---

## Wave 1: Git Worktree Subsystem

All four files are independent at the rewrite level — they share the same package but each task rewrites a single file. No task depends on another task's output within this wave.

### Task 1: Rewrite worktree.go — Struct and Constructors

**Files:**
- Modify: `session/git/worktree.go`
- Test: `session/git/worktree_ops_test.go` (existing, no changes needed)

**Step 1: write the failing test**

No new tests needed. Existing tests exercise `NewGitWorktree`, `NewGitWorktreeFromStorage`, `NewGitWorktreeOnBranch`, and all accessors.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/git/worktree.go` from scratch based on this spec:

- `getWorktreeDirectory(repoPath)` — return `<repoPath>/.worktrees`, error if empty
- `GitWorktree` struct — fields: repoPath, worktreePath, sessionName, branchName, baseCommitSHA
- `NewGitWorktreeFromStorage(repoPath, worktreePath, sessionName, branchName, baseCommitSHA)` — direct constructor
- `NewGitWorktree(repoPath, sessionName)` — load config for branch prefix, sanitize branch name, resolve absolute path, find git root, generate worktree path with hex timestamp suffix
- `NewGitWorktreeOnBranch(repoPath, sessionName, branch)` — like NewGitWorktree but uses exact branch name
- Accessors: `GetWorktreePath()`, `GetBranchName()`, `GetRepoPath()`, `GetRepoName()`, `GetBaseCommitSHA()`

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
- Test: `session/git/worktree_ops_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `Setup`, `Cleanup`, `Remove`, `Prune`, `CleanupWorktrees`, and `syncBranchWithRemote`.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/git/... -run "TestSetup|TestCleanup|TestRemove|TestPrune" -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/git/worktree_ops.go` from scratch:

- `Setup()` — parallel mkdir + branch existence check, dispatch to setupFromExistingBranch or setupNewWorktree
- `setupFromExistingBranch()` — remove stale worktree, sync with remote, create worktree from branch, resolve base commit via merge-base
- `syncBranchWithRemote()` — fetch, compare SHAs, fast-forward if possible, rebase if diverged, abort rebase on conflict
- `setupNewWorktree()` — open repo, cleanup existing branch, get HEAD, create worktree with new branch from HEAD
- `Cleanup()` — remove worktree, delete branch ref, prune
- `Remove()` — remove worktree only (keep branch)
- `Prune()` — `git worktree prune`
- `CleanupWorktrees(repoPath)` — enumerate .worktrees/ entries, remove each worktree + associated branch, prune

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
- Test: `session/git/worktree_ops_test.go` (existing)

**Step 1: write the failing test**

Existing tests cover `runGitCommand`, `PushChanges`, `CommitChanges`, `IsDirty`, `IsBranchCheckedOut`. PR creation is tested via integration.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/git/worktree_git.go` from scratch:

- `runGitCommand(path, args...)` — execute `git -C <path> <args>`, return combined output
- `PushChanges(commitMessage, open)` — check gh CLI, commit changes, push
- `Push(open)` — `git push -u origin <branch>`, optionally open browser
- `GeneratePRBody()` — assemble PR body from changed files, commit log, diff stats
- `CreatePR(title, body, commitMsg)` — push changes, `gh pr create`, handle existing PR
- `CommitChanges(commitMessage)` — check dirty, `git add .`, `git commit -m --no-verify`
- `IsDirty()` — `git status --porcelain`
- `IsBranchCheckedOut()` — `git branch --show-current`
- `OpenBranchURL()` — `gh browse --branch`

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
- Test: `session/git/worktree_ops_test.go` (existing)

**Step 1: write the failing test**

Existing tests exercise `Diff()` and `DiffStats.IsEmpty()`.

**Step 2: run test to verify baseline passes**

```bash
go test ./session/git/... -v -count=1
```

expected: PASS

**Step 3: rewrite implementation**

Rewrite `session/git/diff.go` from scratch:

- `DiffStats` struct — Content string, Added int, Removed int, Error error
- `IsEmpty()` — true when Added==0, Removed==0, Content==""
- `Diff()` — check worktree path exists, get base commit, run `git --no-pager diff <base>`, count +/- lines (excluding +++ and --- headers)

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
