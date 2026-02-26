package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initCleanupTestRepo(t *testing.T) string {
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
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git add: %s", out)

	cmd = exec.Command("git", "-C", repo, "commit", "-m", "initial")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "git commit: %s", out)

	return repo
}

func TestCleanupWorktrees_RemovesWorktreeAndBranch(t *testing.T) {
	repo := initCleanupTestRepo(t)

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(oldWD))
	})
	require.NoError(t, os.Chdir(t.TempDir()))

	wtDir := filepath.Join(repo, ".worktrees", "test-branch")
	require.NoError(t, os.MkdirAll(filepath.Dir(wtDir), 0755))

	cmd := exec.Command("git", "-C", repo, "worktree", "add", "-b", "test-branch", wtDir, "HEAD")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git worktree add: %s", out)

	cmd = exec.Command("git", "-C", repo, "branch", "--list", "test-branch")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(out), "test-branch")

	err = CleanupWorktrees(repo)
	require.NoError(t, err)

	_, err = os.Stat(wtDir)
	assert.True(t, os.IsNotExist(err), "worktree dir should be removed")

	cmd = exec.Command("git", "-C", repo, "branch", "--list", "test-branch")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(out)), "branch should be deleted")
}
