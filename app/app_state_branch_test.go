package app

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentBranch(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo with a commit (needed for branch to exist).
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}

	run("git", "init", "-b", "release/2.0")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	run("git", "commit", "--allow-empty", "-m", "init")

	// Should return the actual branch name.
	branch := currentBranch(dir)
	assert.Equal(t, "release/2.0", branch)
}

func TestCurrentBranch_Fallback(t *testing.T) {
	// Non-existent directory should fall back to "main".
	branch := currentBranch("/nonexistent/path")
	assert.Equal(t, "main", branch)
}
