package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	kaslog "github.com/kastheco/kasmos/log"
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

// TestSyncBranchWithRemote_DoesNotPoisonMainWorktree verifies that
// syncBranchWithRemote never changes the HEAD of the main worktree,
// even when local and remote branches have diverged.
func TestSyncBranchWithRemote_DoesNotPoisonMainWorktree(t *testing.T) {
	// Set up a "remote" bare repo and a "local" clone.
	bare := t.TempDir()
	local := t.TempDir()

	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v in %s: %s", args, dir, out)
		return strings.TrimSpace(string(out))
	}

	// Init bare remote with an initial commit.
	run(bare, "init", "--bare", "--initial-branch=main")
	staging := t.TempDir()
	run(staging, "clone", bare, ".")
	run(staging, "config", "user.email", "test@test.com")
	run(staging, "config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(staging, "README.md"), []byte("init\n"), 0644))
	run(staging, "add", ".")
	run(staging, "commit", "-m", "initial")
	run(staging, "push", "origin", "main")

	// Create a feature branch on the remote with one commit.
	run(staging, "checkout", "-b", "plan/feature")
	require.NoError(t, os.WriteFile(filepath.Join(staging, "remote.txt"), []byte("remote\n"), 0644))
	run(staging, "add", ".")
	run(staging, "commit", "-m", "remote commit")
	run(staging, "push", "origin", "plan/feature")

	// Clone into "local" — this is the main worktree kasmos would operate on.
	run(local, "clone", bare, ".")
	run(local, "config", "user.email", "test@test.com")
	run(local, "config", "user.name", "test")

	// Create a diverged local branch: checkout plan/feature, add a different commit.
	run(local, "checkout", "-b", "plan/feature", "origin/plan/feature~1")
	require.NoError(t, os.WriteFile(filepath.Join(local, "local.txt"), []byte("local\n"), 0644))
	run(local, "add", ".")
	run(local, "commit", "-m", "local diverged commit")

	// Switch back to main — this is the expected state.
	run(local, "checkout", "main")
	mainBranch := run(local, "branch", "--show-current")
	require.Equal(t, "main", mainBranch)

	// Ensure loggers are initialized for the code under test.
	kaslog.Initialize(false)
	defer kaslog.Close()

	// Run syncBranchWithRemote via a GitWorktree targeting the feature branch.
	gw := &GitWorktree{
		repoPath:   local,
		branchName: "plan/feature",
	}
	gw.syncBranchWithRemote()

	// The main worktree MUST still be on "main".
	afterBranch := run(local, "branch", "--show-current")
	assert.Equal(t, "main", afterBranch,
		"syncBranchWithRemote must not change the main worktree's checked-out branch")
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
