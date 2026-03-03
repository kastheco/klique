# Stepthrough Worktrees — Bug Fixes

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix 5 bugs found during git worktree/branch audit — incorrect commands, missing isolation, index mutation, naming divergence.

**Architecture:** All fixes are in `session/git/` (4 files) plus one line in `app/app_state.go`. No new types or interfaces. Each task is independent.

**Tech Stack:** Go, go-git/v5, os/exec git CLI

---

## Wave 1: Core git command fixes

### Task 1: Fix `CleanupWorktrees` — missing `-C`, wrong removal method

**Files:**
- Modify: `session/git/worktree_ops.go:182-247`
- Test: `session/git/worktree_ops_test.go` (create)

Three sub-issues in `CleanupWorktrees`:
- `git worktree list --porcelain` (line 194) runs without `-C repoPath` — executes against CWD
- `git branch -D` (line 225) runs without `-C repoPath`
- `os.RemoveAll` (line 235) bypasses git bookkeeping — should use `git worktree remove -f` first

**Step 1: Write failing test**

Create `session/git/worktree_ops_test.go`. Test that `CleanupWorktrees` properly removes worktrees and their branches in a temp git repo:

```go
package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("init\n"), 0644))
	cmd := exec.Command("git", "-C", repo, "add", ".")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", repo, "commit", "-m", "initial")
	require.NoError(t, cmd.Run())
	return repo
}

func TestCleanupWorktrees_RemovesWorktreeAndBranch(t *testing.T) {
	repo := initTestRepo(t)

	// Create a worktree + branch
	wtDir := filepath.Join(repo, ".worktrees", "test-branch")
	require.NoError(t, os.MkdirAll(filepath.Dir(wtDir), 0755))
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "-b", "test-branch", wtDir, "HEAD")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	// Verify branch exists
	cmd = exec.Command("git", "-C", repo, "branch", "--list", "test-branch")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(out), "test-branch")

	// Run cleanup
	err = CleanupWorktrees(repo)
	require.NoError(t, err)

	// Worktree dir should be gone
	_, err = os.Stat(wtDir)
	assert.True(t, os.IsNotExist(err), "worktree dir should be removed")

	// Branch should be gone
	cmd = exec.Command("git", "-C", repo, "branch", "--list", "test-branch")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(out)), "branch should be deleted")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./session/git/ -run TestCleanupWorktrees -v`
Expected: Test may pass or fail depending on CWD — the point is the code is wrong even if it accidentally works.

**Step 3: Rewrite `CleanupWorktrees`**

Replace the function body to use `-C repoPath` for all git commands and `git worktree remove -f` instead of `os.RemoveAll`:

```go
func CleanupWorktrees(repoPath string) error {
	worktreesDir, err := getWorktreeDirectory(repoPath)
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to clean
		}
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	// Helper to run git commands rooted at repoPath.
	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %v: %s (%w)", args, out, err)
		}
		return string(out), nil
	}

	// Build worktree→branch map from porcelain output.
	output, err := run("worktree", "list", "--porcelain")
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	worktreeBranches := make(map[string]string)
	var currentWT string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentWT = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
			if currentWT != "" {
				worktreeBranches[currentWT] = branch
			}
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		worktreePath := filepath.Join(worktreesDir, entry.Name())

		// Try proper git removal first.
		if _, err := run("worktree", "remove", "-f", worktreePath); err != nil {
			// Fallback: force-remove the directory if git doesn't recognise it.
			log.WarningLog.Printf("git worktree remove failed for %s, falling back to os.RemoveAll: %v", worktreePath, err)
			os.RemoveAll(worktreePath)
		}

		// Delete the associated branch.
		for path, branch := range worktreeBranches {
			if strings.Contains(path, entry.Name()) {
				if _, err := run("branch", "-D", branch); err != nil {
					log.ErrorLog.Printf("failed to delete branch %s: %v", branch, err)
				}
				break
			}
		}
	}

	_, err = run("worktree", "prune")
	if err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./session/git/ -run TestCleanupWorktrees -v`
Expected: PASS

**Step 5: Run full package tests**

Run: `go test ./session/git/ -v`
Expected: All pass

**Step 6: Commit**

```bash
git add session/git/worktree_ops.go session/git/worktree_ops_test.go
git commit -m "fix(git): CleanupWorktrees uses -C repoPath and git worktree remove"
```

---

### Task 2: Fix `PushChanges` — replace `gh repo sync` with `git push`

**Files:**
- Modify: `session/git/worktree_git.go:23-63`

`gh repo sync` is for fork synchronisation, not pushing feature branches. The second call without `--source` actually pulls from remote, risking overwrites. Replace with plain `git push`.

**Step 1: Write failing test**

No unit test needed — this is a command substitution. The existing integration path covers it. But verify the logic is correct by reading the current code.

**Step 2: Rewrite `PushChanges`**

Replace the `gh repo sync` + fallback logic with a single `git push -u origin <branch>`:

```go
func (g *GitWorktree) PushChanges(commitMessage string, open bool) error {
	if err := checkGHCLI(); err != nil {
		return err
	}

	if err := g.CommitChanges(commitMessage); err != nil {
		return err
	}

	// Push the branch to remote.
	if _, err := g.runGitCommand(g.worktreePath, "push", "-u", "origin", g.branchName); err != nil {
		return fmt.Errorf("failed to push branch %s: %w", g.branchName, err)
	}

	if open {
		if err := g.OpenBranchURL(); err != nil {
			log.ErrorLog.Printf("failed to open branch URL: %v", err)
		}
	}

	return nil
}
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: Clean build

**Step 4: Run tests**

Run: `go test ./session/git/ -v`
Expected: All pass

**Step 5: Commit**

```bash
git add session/git/worktree_git.go
git commit -m "fix(git): replace gh repo sync with git push for branch pushing"
```

---

### Task 3: Fix `NewSharedPlanWorktree` — populate `baseCommitSHA`

**Files:**
- Modify: `session/git/worktree_ops.go` (in `setupFromExistingBranch`)
- Modify: `session/git/plan_lifecycle.go` (minor — leave as-is since Setup resolves it)
- Test: `session/git/plan_lifecycle_test.go`

`NewSharedPlanWorktree` passes `""` for baseCommitSHA. The `setupFromExistingBranch` path also never sets it. This means `Diff()` always errors for coder/reviewer sessions.

Fix: resolve the merge-base in `setupFromExistingBranch` after the worktree is created.

**Step 1: Write failing test**

Add to `session/git/plan_lifecycle_test.go`:

```go
func TestSetupFromExistingBranch_SetsBaseCommitSHA(t *testing.T) {
	repo := initTestRepo(t)

	// Create a plan branch with a commit.
	cmd := exec.Command("git", "-C", repo, "branch", "plan/test-base")
	require.NoError(t, cmd.Run())

	gt := NewSharedPlanWorktree(repo, "plan/test-base")
	require.NoError(t, gt.Setup())
	t.Cleanup(func() { gt.Cleanup() })

	assert.NotEmpty(t, gt.GetBaseCommitSHA(), "baseCommitSHA should be set after Setup")
}
```

Note: this test uses `initTestRepo` from the new `worktree_ops_test.go` — if they're in the same package it's available.

**Step 2: Run test to verify it fails**

Run: `go test ./session/git/ -run TestSetupFromExistingBranch_SetsBaseCommitSHA -v`
Expected: FAIL — baseCommitSHA is empty

**Step 3: Fix `setupFromExistingBranch`**

Add base commit resolution after worktree creation:

```go
func (g *GitWorktree) setupFromExistingBranch() error {
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath)

	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		return fmt.Errorf("failed to create worktree from branch %s: %w", g.branchName, err)
	}

	// Resolve a base commit for diff computation. Try merge-base with HEAD
	// of the main worktree first; fall back to the branch's own HEAD.
	if g.baseCommitSHA == "" {
		if out, err := g.runGitCommand(g.repoPath, "merge-base", "HEAD", g.branchName); err == nil {
			g.baseCommitSHA = strings.TrimSpace(out)
		} else if out, err := g.runGitCommand(g.worktreePath, "rev-parse", "HEAD"); err == nil {
			g.baseCommitSHA = strings.TrimSpace(out)
		}
	}

	return nil
}
```

This needs `"strings"` imported — already imported in this file via `worktree_ops.go`.

**Step 4: Run test to verify it passes**

Run: `go test ./session/git/ -run TestSetupFromExistingBranch_SetsBaseCommitSHA -v`
Expected: PASS

**Step 5: Run full package tests**

Run: `go test ./session/git/ -v`
Expected: All pass

**Step 6: Commit**

```bash
git add session/git/worktree_ops.go session/git/plan_lifecycle_test.go
git commit -m "fix(git): resolve baseCommitSHA in setupFromExistingBranch for diff support"
```

---

## Wave 2: Index safety and naming consistency

### Task 4: Fix `Diff()` — stop mutating index with `git add -N`

**Files:**
- Modify: `session/git/diff.go:27-61`

`git add -N .` runs every metadata tick, mutating the worktree's index. This creates `index.lock` contention with agents actively staging files.

Replace with a read-only approach: diff tracked files against base, then count untracked files separately.

**Step 1: Rewrite `Diff()`**

```go
func (g *GitWorktree) Diff() *DiffStats {
	stats := &DiffStats{}

	if _, err := os.Stat(g.worktreePath); err != nil {
		stats.Error = fmt.Errorf("worktree path gone: %w", err)
		return stats
	}

	base := g.GetBaseCommitSHA()
	if base == "" {
		stats.Error = fmt.Errorf("no base commit SHA available")
		return stats
	}

	// Diff tracked changes (read-only, does not touch the index).
	content, err := g.runGitCommand(g.worktreePath, "--no-pager", "diff", base)
	if err != nil {
		stats.Error = err
		return stats
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.Added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.Removed++
		}
	}
	stats.Content = content

	return stats
}
```

This drops untracked files from the diff count — acceptable trade-off vs index corruption.

**Step 2: Verify build**

Run: `go build ./...`
Expected: Clean

**Step 3: Run tests**

Run: `go test ./session/git/ -v`
Expected: All pass

**Step 4: Commit**

```bash
git add session/git/diff.go
git commit -m "fix(git): remove git add -N from Diff to avoid index lock contention"
```

---

### Task 5: Fix ClickUp import branch naming

**Files:**
- Modify: `app/app_state.go:1105`

`importClickUpTask` manually constructs the branch as `"plan/" + strings.TrimSuffix(filename, ".md")` which includes the date prefix, diverging from `PlanBranchFromFile` which strips it.

**Step 1: Fix the branch construction**

Change line 1105 from:
```go
branch := "plan/" + strings.TrimSuffix(filename, ".md")
```
to:
```go
branch := gitpkg.PlanBranchFromFile(filename)
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: Clean

**Step 3: Run tests**

Run: `go test ./app/ -v -count=1`
Expected: All pass

**Step 4: Commit**

```bash
git add app/app_state.go
git commit -m "fix(clickup): use PlanBranchFromFile for consistent branch naming"
```
