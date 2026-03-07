package daemon

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/daemon/api"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemon_StartStop(t *testing.T) {
	cfg := &DaemonConfig{
		PollInterval: 100 * time.Millisecond,
	}
	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run(ctx)
	}()

	time.Sleep(250 * time.Millisecond)
	cancel()

	err = <-errCh
	assert.NoError(t, err)
}

func TestDaemon_ControlSocket(t *testing.T) {
	dir := t.TempDir()
	cfg := &DaemonConfig{
		PollInterval: 100 * time.Millisecond,
		SocketPath:   filepath.Join(dir, "kas.sock"),
	}
	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run(ctx)
	}()

	// Wait for socket to appear
	require.Eventually(t, func() bool {
		_, err := os.Stat(cfg.SocketPath)
		return err == nil
	}, 2*time.Second, 50*time.Millisecond)

	// Connect and query status
	client := NewSocketClient(cfg.SocketPath)
	status, err := client.Status()
	require.NoError(t, err)
	assert.True(t, status.Running)

	cancel()
	<-errCh
}

func TestDaemon_RunRejectsSecondInstanceForSameSocket(t *testing.T) {
	dir := t.TempDir()
	cfg := &DaemonConfig{
		PollInterval: 100 * time.Millisecond,
		SocketPath:   filepath.Join(dir, "kas.sock"),
	}

	first, err := NewDaemon(cfg)
	require.NoError(t, err)

	ctx1, cancel1 := context.WithCancel(context.Background())
	errCh1 := make(chan error, 1)
	go func() {
		errCh1 <- first.Run(ctx1)
	}()

	require.Eventually(t, func() bool {
		_, err := os.Stat(cfg.SocketPath)
		return err == nil
	}, 2*time.Second, 50*time.Millisecond)

	second, err := NewDaemon(cfg)
	require.NoError(t, err)
	err = second.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daemon already running")

	cancel1()
	assert.NoError(t, <-errCh1)
}

func TestDaemon_AddRepo(t *testing.T) {
	cfg := &DaemonConfig{
		PollInterval: 100 * time.Millisecond,
	}
	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = d.AddRepo(tmpDir)
	assert.NoError(t, err)

	repos := d.ListRepos()
	assert.Len(t, repos, 1)
}

func TestDaemon_GracefulShutdown_DrainsAgents(t *testing.T) {
	cfg := &DaemonConfig{
		PollInterval: 100 * time.Millisecond,
	}
	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	err = <-errCh
	assert.NoError(t, err)
	assert.Empty(t, d.spawner.RunningInstances())
}

func TestDaemon_RecoverOnRestart(t *testing.T) {
	cfg := &DaemonConfig{
		PollInterval: 100 * time.Millisecond,
	}
	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, d.AddRepo(dir))

	recovered, err := d.RecoverSessions()
	assert.NoError(t, err)
	assert.Equal(t, 0, recovered)
}

func TestDaemon_TickScansSharedWorktreeSignals(t *testing.T) {
	repo := t.TempDir()
	project := filepath.Base(repo)
	store := taskstore.NewTestStore(t)
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "plan.md",
		Status:   taskstore.StatusImplementing,
		Branch:   "plan/plan",
	}))

	d := &Daemon{
		repos:       NewRepoManager(),
		spawner:     NewTmuxSpawner(),
		logger:      slog.Default(),
		broadcaster: api.NewEventBroadcaster(),
	}
	d.repos.repos = []RepoEntry{{
		Path:      repo,
		Project:   project,
		Store:     store,
		Processor: loop.NewProcessor(loop.ProcessorConfig{Store: store, Project: project}),
	}}

	wtSignals := filepath.Join(repo, ".worktrees", "plan-plan", ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(wtSignals, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wtSignals, "implement-finished-plan.md"), nil, 0o644))

	d.tick(context.Background())

	entry, err := store.Get(project, "plan.md")
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusReviewing, entry.Status)
}

func TestCoderSpawnOpts_ForwardsFeedbackAsPrompt(t *testing.T) {
	repo := RepoEntry{Path: "/tmp/repo", Project: "repo"}
	opts := coderSpawnOpts(repo, "plan.md", "plan/plan", "apply requested fixes")

	assert.Equal(t, "apply requested fixes", opts.Prompt)
	assert.Equal(t, "apply requested fixes", opts.Feedback)
	assert.Equal(t, "/tmp/repo", opts.RepoPath)
	assert.Equal(t, "plan/plan", opts.Branch)
}

func TestSharedWorktreePaths(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".worktrees", "a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".worktrees", "b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".worktrees", "README"), nil, 0o644))

	paths := sharedWorktreePaths(repo)
	assert.ElementsMatch(t, []string{
		filepath.Join(repo, ".worktrees", "a"),
		filepath.Join(repo, ".worktrees", "b"),
	}, paths)
}

func TestTmuxSpawner_DiscoverOrphans(t *testing.T) {
	s := NewTmuxSpawner(TmuxSpawnerConfig{})
	orphans := s.DiscoverOrphanSessions()
	assert.NotNil(t, orphans)
}
