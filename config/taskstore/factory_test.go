package taskstore

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStoreFromConfig_HTTP(t *testing.T) {
	backend := newTestStore(t)
	srv := httptest.NewServer(NewHandler(backend))
	defer srv.Close()

	store, err := NewStoreFromConfig(srv.URL, "test-project")
	require.NoError(t, err)
	require.NoError(t, store.Ping())
}

func TestNewStoreFromConfig_Empty(t *testing.T) {
	store, err := NewStoreFromConfig("", "test-project")
	require.NoError(t, err)
	// Returns nil store — caller should fall back to legacy behavior
	assert.Nil(t, store)
}

func TestNewStoreFromConfig_Unreachable(t *testing.T) {
	store, err := NewStoreFromConfig("http://127.0.0.1:1", "test-project")
	// Factory succeeds (lazy connect) but Ping fails
	require.NoError(t, err)
	require.Error(t, store.Ping())
}

func TestResolvedDBPath(t *testing.T) {
	runGit := func(t *testing.T, repo string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, string(out))
	}

	t.Run("returns taskstore.db under .kasmos in working directory", func(t *testing.T) {
		projectDir := t.TempDir()
		t.Chdir(projectDir)

		dbPath := ResolvedDBPath()

		assert.Equal(t, filepath.Join(projectDir, ".kasmos", "taskstore.db"), dbPath)
	})

	t.Run("returns taskstore.db under main repo root from worktree", func(t *testing.T) {
		repoDir := t.TempDir()
		t.Setenv("HOME", t.TempDir())

		runGit(t, repoDir, "init", "-b", "main")
		runGit(t, repoDir, "config", "user.email", "test@example.com")
		runGit(t, repoDir, "config", "user.name", "test")
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("init\n"), 0o644))
		runGit(t, repoDir, "add", ".")
		runGit(t, repoDir, "commit", "-m", "initial")

		runGit(t, repoDir, "branch", "plan/worktree-db")
		worktreeParent := t.TempDir()
		worktreeDir := filepath.Join(worktreeParent, "worktree-db")
		runGit(t, repoDir, "worktree", "add", worktreeDir, "plan/worktree-db")
		t.Chdir(worktreeDir)

		dbPath := ResolvedDBPath()
		assert.Equal(t, filepath.Join(repoDir, ".kasmos", "taskstore.db"), dbPath)
	})
}
