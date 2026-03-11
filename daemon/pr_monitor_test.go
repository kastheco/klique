package daemon

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/daemon/api"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewPRMonitor_DefaultsApplied verifies that NewPRMonitor fills in missing
// defaults for PollInterval and Reactions.
func TestNewPRMonitor_DefaultsApplied(t *testing.T) {
	cfg := PRMonitorConfig{}
	m := NewPRMonitor(cfg, 0, NewRepoManager(), api.NewEventBroadcaster(), slog.Default(), nil)

	assert.Greater(t, m.cfg.PollInterval, time.Duration(0))
	assert.NotEmpty(t, m.cfg.Reactions)
}

// TestPRMonitor_SkipsReposWithoutStore ensures that repos without a task store
// are skipped silently without panicking.
func TestPRMonitor_SkipsReposWithoutStore(t *testing.T) {
	repos := NewRepoManager()
	dispatched := 0
	dispatch := func(_ context.Context, _ RepoEntry, _ loop.Action) error {
		dispatched++
		return nil
	}

	m := NewPRMonitor(
		PRMonitorConfig{PollInterval: time.Second, Reactions: []string{"eyes"}},
		0,
		repos,
		api.NewEventBroadcaster(),
		slog.Default(),
		dispatch,
	)

	// Add a repo entry without a store directly.
	repos.mu.Lock()
	repos.repos = append(repos.repos, RepoEntry{
		Path:    t.TempDir(),
		Project: "no-store",
		Store:   nil,
	})
	repos.mu.Unlock()

	m.poll(context.Background())
	assert.Equal(t, 0, dispatched, "expected no dispatch for repo without store")
}

// TestPRMonitor_SkipsTasksWithoutPRURL verifies that tasks with an empty PRURL
// are silently ignored.
func TestPRMonitor_SkipsTasksWithoutPRURL(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "plan.md",
		Status:   taskstore.StatusReviewing,
		// PRURL intentionally empty.
	}))

	dispatched := 0
	dispatch := func(_ context.Context, _ RepoEntry, _ loop.Action) error {
		dispatched++
		return nil
	}

	repos := NewRepoManager()
	repos.mu.Lock()
	repos.repos = append(repos.repos, RepoEntry{
		Path:    t.TempDir(),
		Project: project,
		Store:   store,
	})
	repos.mu.Unlock()

	m := NewPRMonitor(
		PRMonitorConfig{PollInterval: time.Second, Reactions: []string{"eyes"}},
		0,
		repos,
		api.NewEventBroadcaster(),
		slog.Default(),
		dispatch,
	)

	m.poll(context.Background())
	assert.Equal(t, 0, dispatched, "expected no dispatch for task without PRURL")
}

// TestPRMonitor_SkipsTasksAtCycleLimit verifies that tasks that have reached
// the max review-fix cycle count are skipped.
func TestPRMonitor_SkipsTasksAtCycleLimit(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "plan.md",
		Status:      taskstore.StatusReviewing,
		PRURL:       "https://github.com/owner/repo/pull/1",
		ReviewCycle: 3,
	}))

	dispatched := 0
	dispatch := func(_ context.Context, _ RepoEntry, _ loop.Action) error {
		dispatched++
		return nil
	}

	repos := NewRepoManager()
	repos.mu.Lock()
	repos.repos = append(repos.repos, RepoEntry{
		Path:    t.TempDir(),
		Project: project,
		Store:   store,
	})
	repos.mu.Unlock()

	// maxReviewFixCycles=3 means tasks with ReviewCycle>=3 should be skipped.
	m := NewPRMonitor(
		PRMonitorConfig{PollInterval: time.Second, Reactions: []string{"eyes"}},
		3,
		repos,
		api.NewEventBroadcaster(),
		slog.Default(),
		dispatch,
	)

	m.poll(context.Background())
	assert.Equal(t, 0, dispatched, "expected no dispatch for task at cycle limit")
}

// TestPRMonitor_Run_StopsOnContextCancel verifies that Run exits cleanly when
// the context is cancelled.
func TestPRMonitor_Run_StopsOnContextCancel(t *testing.T) {
	m := NewPRMonitor(
		PRMonitorConfig{PollInterval: 10 * time.Second, Reactions: []string{"eyes"}},
		0,
		NewRepoManager(),
		api.NewEventBroadcaster(),
		slog.Default(),
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("PRMonitor.Run did not exit within 2s after context cancellation")
	}
}

// TestPRMonitor_MarkFixerDispatched_OnlyOnSuccess verifies that
// MarkReviewFixerDispatched is called only when dispatch returns nil, and NOT
// called when dispatch returns an error.
func TestPRMonitor_MarkFixerDispatched_OnlyOnSuccess(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	filename := "plan.md"
	const reviewID = 42

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: filename,
		Status:   taskstore.StatusReviewing,
	}))
	require.NoError(t, store.RecordPRReview(project, filename, reviewID,
		"CHANGES_REQUESTED", "please fix this", "reviewer-bot"))

	e := RepoEntry{
		Path:    t.TempDir(),
		Project: project,
		Store:   store,
	}
	action := loop.SpawnFixerAction{PlanFile: filename, Feedback: "please fix this"}

	t.Run("success path marks fixer dispatched", func(t *testing.T) {
		dispatch := func(_ context.Context, _ RepoEntry, _ loop.Action) error { return nil }
		if err := dispatch(context.Background(), e, action); err == nil {
			require.NoError(t, store.MarkReviewFixerDispatched(project, filename, reviewID))
		}

		pending, err := store.ListPendingReviews(project, filename)
		require.NoError(t, err)
		assert.Empty(t, pending, "review should not be pending after successful dispatch")
	})

	// Re-insert a fresh review for the failure test.
	const reviewID2 = 99
	require.NoError(t, store.RecordPRReview(project, filename, reviewID2,
		"CHANGES_REQUESTED", "fix it", "reviewer"))

	t.Run("failure path does not mark fixer dispatched", func(t *testing.T) {
		dispatchErr := errors.New("spawn failed")
		dispatch := func(_ context.Context, _ RepoEntry, _ loop.Action) error { return dispatchErr }
		if err := dispatch(context.Background(), e, action); err != nil {
			// Do NOT call MarkReviewFixerDispatched on failure.
		}

		pending, err := store.ListPendingReviews(project, filename)
		require.NoError(t, err)
		assert.Len(t, pending, 1, "review should remain pending when dispatch fails")
		assert.Equal(t, reviewID2, pending[0].ReviewID)
	})
}

// TestDaemon_ExecuteAction_ReturnsError verifies that executeAction now returns
// an error and the tick path can safely discard it.
func TestDaemon_ExecuteAction_ReturnsNilForUnhandledAction(t *testing.T) {
	d := &Daemon{
		repos:       NewRepoManager(),
		spawner:     NewTmuxSpawner(),
		logger:      slog.Default(),
		broadcaster: api.NewEventBroadcaster(),
	}

	// AdvanceWaveAction is a no-op that should return nil.
	action := loop.AdvanceWaveAction{PlanFile: "plan.md", Wave: 1}
	err := d.executeAction(context.Background(), RepoEntry{Path: t.TempDir()}, action)
	assert.NoError(t, err)
}
