package git

import (
	"path/filepath"
	"testing"
)

func TestPlanBranchFromFile(t *testing.T) {
	got := PlanBranchFromFile("2026-02-21-auth-refactor.md")
	want := "plan/auth-refactor"
	if got != want {
		t.Fatalf("PlanBranchFromFile() = %q, want %q", got, want)
	}
}

func TestPlanWorktreePath(t *testing.T) {
	repo := "/tmp/repo"
	branch := "plan/auth-refactor"
	got := PlanWorktreePath(repo, branch)
	want := filepath.Join(repo, ".worktrees", "plan-auth-refactor")
	if got != want {
		t.Fatalf("PlanWorktreePath() = %q, want %q", got, want)
	}
}

func TestNewSharedPlanWorktree(t *testing.T) {
	repo := "/tmp/repo"
	branch := "plan/auth-refactor"
	gt := NewSharedPlanWorktree(repo, branch)

	if gt.GetRepoPath() != repo {
		t.Fatalf("repo = %q, want %q", gt.GetRepoPath(), repo)
	}
	if gt.GetWorktreePath() != filepath.Join(repo, ".worktrees", "plan-auth-refactor") {
		t.Fatalf("unexpected worktree path %q", gt.GetWorktreePath())
	}
	if gt.GetBranchName() != branch {
		t.Fatalf("branch = %q, want %q", gt.GetBranchName(), branch)
	}
}
