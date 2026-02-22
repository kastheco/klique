package git

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kastheco/klique/config/planstate"
)

// PlanBranchFromFile derives the git branch name from a plan filename.
// "2026-02-21-auth-refactor.md" → "plan/auth-refactor"
func PlanBranchFromFile(planFile string) string {
	name := planstate.DisplayName(planFile)
	name = sanitizeBranchName(name)
	if name == "" {
		name = "plan"
	}
	return "plan/" + name
}

// PlanWorktreePath returns the worktree path for a plan branch.
// The branch separator "/" is replaced with "-" to form a valid directory name.
func PlanWorktreePath(repoPath, branch string) string {
	safe := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(repoPath, ".worktrees", safe)
}

// NewSharedPlanWorktree constructs a GitWorktree for the shared plan worktree
// (used by coder and reviewer sessions that share the same branch).
func NewSharedPlanWorktree(repoPath, branch string) *GitWorktree {
	return NewGitWorktreeFromStorage(
		repoPath,
		PlanWorktreePath(repoPath, branch),
		"plan-shared",
		branch,
		"",
	)
}

// CommitPlanScaffoldOnMain stages the plan stub file and plan-state.json on the
// main branch (repoPath is the main worktree) and creates a commit.
func CommitPlanScaffoldOnMain(repoPath, planFile string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}
	planPath := filepath.Join("docs", "plans", planFile)
	statePath := filepath.Join("docs", "plans", "plan-state.json")
	if _, err := gt.runGitCommand(repoPath, "add", planPath, statePath); err != nil {
		return fmt.Errorf("stage plan scaffold: %w", err)
	}
	if _, err := gt.runGitCommand(repoPath, "commit", "-m", "feat(plan): add "+planstate.DisplayName(planFile)+" scaffold"); err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("commit plan scaffold: %w", err)
	}
	return nil
}

// EnsurePlanBranch creates the plan branch off the current HEAD if it doesn't
// already exist. It is idempotent.
func EnsurePlanBranch(repoPath, branch string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}
	if _, err := gt.runGitCommand(repoPath, "rev-parse", "--verify", branch); err == nil {
		return nil // already exists
	}
	if _, err := gt.runGitCommand(repoPath, "branch", branch); err != nil {
		return fmt.Errorf("create plan branch %s: %w", branch, err)
	}
	return nil
}

// ResetPlanBranch removes the plan worktree (if any), deletes the branch, and
// recreates it from the current HEAD. Used by "start over".
func ResetPlanBranch(repoPath, branch string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}
	worktreePath := PlanWorktreePath(repoPath, branch)
	_, _ = gt.runGitCommand(repoPath, "worktree", "remove", "-f", worktreePath)
	_, _ = gt.runGitCommand(repoPath, "branch", "-D", branch)
	if _, err := gt.runGitCommand(repoPath, "branch", branch); err != nil {
		return fmt.Errorf("recreate plan branch %s: %w", branch, err)
	}
	if _, err := gt.runGitCommand(repoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("prune worktrees: %w", err)
	}
	return nil
}
