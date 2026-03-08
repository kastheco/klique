package daemon

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskfsm"
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
	require.NoError(t, taskfsm.EnsureSignalDirs(wtSignals))
	require.NoError(t, os.WriteFile(filepath.Join(wtSignals, "implement-finished-plan.md"), nil, 0o644))

	d.tick(context.Background())

	entry, err := store.Get(project, "plan.md")
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusReviewing, entry.Status)

	// The signal should not be stuck in processing after tick completes.
	_, err = os.Stat(filepath.Join(wtSignals, "processing", "implement-finished-plan.md"))
	assert.True(t, os.IsNotExist(err), "processing file should not exist after tick")
}

func TestDaemon_Tick_AtomicProcessing(t *testing.T) {
	repo := t.TempDir()
	project := filepath.Base(repo)
	store := taskstore.NewTestStore(t)
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "atomic-plan.md",
		Status:   taskstore.StatusPlanning,
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

	signalsDir := filepath.Join(repo, ".kasmos", "signals")
	require.NoError(t, taskfsm.EnsureSignalDirs(signalsDir))
	signalFile := "planner-finished-atomic-plan.md"
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, signalFile), nil, 0o644))

	d.tick(context.Background())

	// Base signal file must be gone.
	_, err := os.Stat(filepath.Join(signalsDir, signalFile))
	assert.True(t, os.IsNotExist(err), "base signal file should be gone after tick")

	// Processing file must be gone (consumed on success).
	_, err = os.Stat(filepath.Join(signalsDir, "processing", signalFile))
	assert.True(t, os.IsNotExist(err), "processing file should be gone after successful processing")

	// Failed file must be absent (signal was handled successfully).
	_, err = os.Stat(filepath.Join(signalsDir, "failed", signalFile))
	assert.True(t, os.IsNotExist(err), "failed file should not exist for a successfully processed signal")

	// Store status must have transitioned to ready.
	entry, err := store.Get(project, "atomic-plan.md")
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusReady, entry.Status)
}

func TestDaemon_RecoverInFlight_OnStartup(t *testing.T) {
	signalsDir := t.TempDir()
	require.NoError(t, taskfsm.EnsureSignalDirs(signalsDir))

	// Simulate a crash: place a file in processing/
	staleFile := "planner-finished-stale.md"
	processingPath := filepath.Join(signalsDir, "processing", staleFile)
	require.NoError(t, os.WriteFile(processingPath, nil, 0o644))

	n := taskfsm.RecoverInFlight(signalsDir)
	assert.Equal(t, 1, n, "should recover exactly one in-flight signal")

	// File must have moved back to the base signals dir.
	_, err := os.Stat(filepath.Join(signalsDir, staleFile))
	assert.NoError(t, err, "recovered signal should be in the base signals dir")

	// Processing file must be gone.
	_, err = os.Stat(processingPath)
	assert.True(t, os.IsNotExist(err), "processing file should be gone after recovery")
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

func TestDaemon_TickRepoUsesGateway(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, ".kasmos", "signals")
	require.NoError(t, os.MkdirAll(signalsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(signalsDir, "planner-finished-gw-plan"), nil, 0o644))

	store := taskstore.NewTestStore(t)
	require.NoError(t, store.Create("test-project", taskstore.TaskEntry{Filename: "gw-plan", Status: taskstore.StatusPlanning}))

	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = gw.Close() })

	entry := RepoEntry{
		Path:          dir,
		Project:       "test-project",
		Store:         store,
		SignalsDir:    signalsDir,
		Processor:     loop.NewProcessor(loop.ProcessorConfig{Store: store, Project: "test-project"}),
		SignalGateway: gw,
	}
	d := &Daemon{
		cfg:         &DaemonConfig{PollInterval: time.Second},
		repos:       NewRepoManager(),
		spawner:     newTmuxSpawner(slog.Default()),
		logger:      slog.Default(),
		broadcaster: api.NewEventBroadcaster(),
	}

	d.tickRepo(context.Background(), entry)

	files, err := os.ReadDir(signalsDir)
	require.NoError(t, err)
	assert.Empty(t, files)

	done, err := gw.List("test-project", taskstore.SignalDone)
	require.NoError(t, err)
	assert.Len(t, done, 1)

	updated, err := store.Get("test-project", "gw-plan")
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusReady, updated.Status)
}

func TestReapStuckSignals(t *testing.T) {
	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = gw.Close() })

	require.NoError(t, gw.Create("proj", taskstore.SignalEntry{PlanFile: "stuck-plan", SignalType: "implement_finished"}))
	claimed, err := gw.Claim("proj", "worker-1")
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.NoError(t, gw.BackdateClaimedAt(claimed.ID, 2*time.Minute))

	n := reapStuckSignals([]RepoEntry{{SignalGateway: gw}}, 60*time.Second, slog.Default())
	assert.Equal(t, 1, n)

	pending, err := gw.List("proj", taskstore.SignalPending)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
}
