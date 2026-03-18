package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/daemon/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalRepoPath_ResolvesWorktreeToMainRepo(t *testing.T) {
	mainRepo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(mainRepo, ".git", "worktrees", "wt-plans"), 0o755))

	worktree := t.TempDir()
	gitFile := "gitdir: " + filepath.Join(mainRepo, ".git", "worktrees", "wt-plans") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte(gitFile), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(mainRepo, ".git", "worktrees", "wt-plans", "commondir"),
		[]byte("../..\n"),
		0o644,
	))

	assert.Equal(t, filepath.Clean(mainRepo), canonicalRepoPath(worktree))
}

func TestNewHome_UsesMainRepoRootWhenLaunchedFromWorktree(t *testing.T) {
	mainRepo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(mainRepo, ".git", "worktrees", "wt-plans"), 0o755))

	worktree := t.TempDir()
	gitFile := "gitdir: " + filepath.Join(mainRepo, ".git", "worktrees", "wt-plans") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte(gitFile), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(mainRepo, ".git", "worktrees", "wt-plans", "commondir"),
		[]byte("../..\n"),
		0o644,
	))

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(worktree))
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	h := newHome(context.Background(), "opencode", false, "test")
	t.Cleanup(func() {
		h.embeddedServer.Stop()
		h.auditLogger.Close()
	})

	assert.Equal(t, filepath.Clean(mainRepo), filepath.Clean(h.activeRepoPath))
}

func TestResolveTaskStoreProject_PrefersDaemonRegisteredProject(t *testing.T) {
	mainRepo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(mainRepo, ".git"), 0o755))
	aliasPath := filepath.Join(t.TempDir(), "cms")
	require.NoError(t, os.Symlink(mainRepo, aliasPath))

	old := listDaemonRepoStatuses
	listDaemonRepoStatuses = func() ([]api.RepoStatus, error) {
		return []api.RepoStatus{{Path: aliasPath, Project: "cms"}}, nil
	}
	t.Cleanup(func() {
		listDaemonRepoStatuses = old
	})

	assert.Equal(t, "cms", resolveTaskStoreProject(mainRepo))
}
