package git

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kastheco/kasmos/config/taskstate"
)

// TaskBranchFromFile derives the git branch name from a plan filename.
// "auth-refactor" → "plan/auth-refactor"
func TaskBranchFromFile(planFile string) string {
	name := taskstate.DisplayName(planFile)
	name = sanitizeBranchName(name)
	if name == "" {
		name = "plan"
	}
	return "plan/" + name
}

// TaskWorktreePath returns the worktree path for a plan branch.
// The branch separator "/" is replaced with "-" to form a valid directory name.
func TaskWorktreePath(repoPath, branch string) string {
	safe := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(repoPath, ".worktrees", safe)
}

// NewSharedTaskWorktree constructs a GitWorktree for the shared plan worktree
// (used by coder and reviewer sessions that share the same branch).
func NewSharedTaskWorktree(repoPath, branch string) *GitWorktree {
	return NewGitWorktreeFromStorage(
		repoPath,
		TaskWorktreePath(repoPath, branch),
		"plan-shared",
		branch,
		"",
	)
}

// EnsureTaskBranch creates the plan branch off the current HEAD if it doesn't
// already exist. It is idempotent.
func EnsureTaskBranch(repoPath, branch string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}
	if _, err := gt.runGitCommand(repoPath, "rev-parse", "--verify", branch); err == nil {
		return nil // already exists
	}
	if _, err := gt.runGitCommand(repoPath, "branch", branch); err != nil {
		return fmt.Errorf("create plan branch %s: %w", branch, err)
	}
	return nil
}

// MergeTaskBranch merges the plan branch into the current branch (typically main),
// removes the worktree, and deletes the plan branch.
func MergeTaskBranch(repoPath, branch string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}
	worktreePath := TaskWorktreePath(repoPath, branch)

	// Remove worktree first so the branch isn't "checked out" elsewhere.
	_, _ = gt.runGitCommand(repoPath, "worktree", "remove", "-f", worktreePath)
	_, _ = gt.runGitCommand(repoPath, "worktree", "prune")

	// Ensure the local branch exists — worktree removal may have deleted it.
	// If a remote tracking branch exists, recreate the local one from it.
	if _, err := gt.runGitCommand(repoPath, "rev-parse", "--verify", branch); err != nil {
		remote := "origin/" + branch
		if _, remoteErr := gt.runGitCommand(repoPath, "rev-parse", "--verify", remote); remoteErr == nil {
			if _, brErr := gt.runGitCommand(repoPath, "branch", branch, remote); brErr != nil {
				return fmt.Errorf("recreate local branch %s from remote: %w", branch, brErr)
			}
		} else {
			return fmt.Errorf("branch %s not found locally or on remote", branch)
		}
	}

	// Merge the plan branch into the current branch.
	if _, err := gt.runGitCommand(repoPath, "merge", branch, "--no-ff", "-m",
		fmt.Sprintf("merge plan branch %s", branch)); err != nil {
		return fmt.Errorf("merge %s: %w", branch, err)
	}

	// Delete the plan branch after successful merge.
	if _, err := gt.runGitCommand(repoPath, "branch", "-d", branch); err != nil {
		// Non-fatal: merge succeeded, branch cleanup is best-effort.
		_ = err
	}

	return nil
}

// PreflightMergeTaskBranch checks whether the current branch can safely merge
// the task branch without clobbering local uncommitted changes in the repo
// worktree. It only blocks when dirty paths overlap files changed by the
// incoming branch.
func PreflightMergeTaskBranch(repoPath, branch string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}

	statusOut, err := gt.runGitCommand(repoPath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("preflight merge %s: check worktree status: %w", branch, err)
	}

	dirtyPaths := dirtyPathsFromPorcelain(statusOut)
	if len(dirtyPaths) == 0 {
		return nil
	}

	changedOut, err := gt.runGitCommand(repoPath, "diff", "--name-only", "HEAD..."+branch)
	if err != nil {
		return fmt.Errorf("preflight merge %s: list changed files: %w", branch, err)
	}

	changedPaths := pathSetFromLines(changedOut)
	if len(changedPaths) == 0 {
		return nil
	}

	overlap := intersectPathSets(dirtyPaths, changedPaths)
	if len(overlap) == 0 {
		return nil
	}

	preview := overlap
	if len(preview) > 5 {
		preview = preview[:5]
	}
	suffix := ""
	if len(overlap) > len(preview) {
		suffix = fmt.Sprintf(" (+%d more)", len(overlap)-len(preview))
	}

	return fmt.Errorf(
		"cannot merge %s: uncommitted changes overlap with incoming branch (%s%s); commit or stash first",
		branch,
		strings.Join(preview, ", "),
		suffix,
	)
}

func dirtyPathsFromPorcelain(statusOut string) map[string]struct{} {
	paths := make(map[string]struct{})
	for _, line := range strings.Split(statusOut, "\n") {
		if line == "" {
			continue
		}
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if arrow := strings.Index(path, " -> "); arrow >= 0 {
			path = path[arrow+4:]
		}
		if path != "" {
			paths[path] = struct{}{}
		}
	}
	return paths
}

func pathSetFromLines(out string) map[string]struct{} {
	paths := make(map[string]struct{})
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths[line] = struct{}{}
		}
	}
	return paths
}

func intersectPathSets(a, b map[string]struct{}) []string {
	var overlap []string
	for path := range a {
		if _, ok := b[path]; ok {
			overlap = append(overlap, path)
		}
	}
	sort.Strings(overlap)
	return overlap
}

// ResetTaskBranch removes the plan worktree (if any), deletes the branch, and
// recreates it from the current HEAD. Used by "start over".
func ResetTaskBranch(repoPath, branch string) error {
	gt := &GitWorktree{repoPath: repoPath, worktreePath: repoPath}
	worktreePath := TaskWorktreePath(repoPath, branch)
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
