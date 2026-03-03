package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskBranchFromFile(t *testing.T) {
	got := TaskBranchFromFile("auth-refactor.md")
	want := "plan/auth-refactor"
	if got != want {
		t.Fatalf("TaskBranchFromFile() = %q, want %q", got, want)
	}
}

func TestTaskWorktreePath(t *testing.T) {
	repo := "/tmp/repo"
	branch := "plan/auth-refactor"
	got := TaskWorktreePath(repo, branch)
	want := filepath.Join(repo, ".worktrees", "plan-auth-refactor")
	if got != want {
		t.Fatalf("TaskWorktreePath() = %q, want %q", got, want)
	}
}

func TestNewSharedTaskWorktree(t *testing.T) {
	repo := "/tmp/repo"
	branch := "plan/auth-refactor"
	gt := NewSharedTaskWorktree(repo, branch)

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

func TestSetupFromExistingBranch_SetsBaseCommitSHA(t *testing.T) {
	repo := initTestRepo(t)

	cmd := exec.Command("git", "-C", repo, "branch", "plan/test-base")
	require.NoError(t, cmd.Run())

	gt := NewSharedTaskWorktree(repo, "plan/test-base")
	require.NoError(t, gt.Setup())
	t.Cleanup(func() { _ = gt.Cleanup() })

	assert.NotEmpty(t, gt.GetBaseCommitSHA(), "baseCommitSHA should be set after Setup")
}

func initTestRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, string(out))
	}

	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	readmePath := filepath.Join(repo, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("initial\n"), 0644))

	runGit("add", "README.md")
	runGit("commit", "-m", "initial commit")

	return repo
}
