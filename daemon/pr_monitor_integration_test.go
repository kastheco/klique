package daemon

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/daemon/api"
	"github.com/kastheco/kasmos/orchestration/loop"
	gitpkg "github.com/kastheco/kasmos/session/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dispatchedAction captures an action dispatched by PRMonitor during tests.
type dispatchedAction struct {
	repo   string
	action loop.Action
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const (
	testProject  = "proj"
	testPlanFile = "plan.md"
	testBranch   = "plan/plan"
	testPRURL    = "https://github.com/owner/repo/pull/42"
)

// fakeReview builds a gitpkg.PRReview with the given parameters.
func fakeReview(id int, state, body, user string) gitpkg.PRReview {
	return gitpkg.PRReview{
		ID:          id,
		State:       state,
		Body:        body,
		User:        user,
		SubmittedAt: time.Now(),
	}
}

// fakeComment builds a gitpkg.PRReviewComment.
func fakeComment(id int) gitpkg.PRReviewComment {
	return gitpkg.PRReviewComment{ID: id}
}

// makeTestStoreWithTask creates an in-memory store and inserts a reviewing task.
func makeTestStoreWithTask(t *testing.T) taskstore.Store {
	t.Helper()
	store := taskstore.NewTestStore(t)
	require.NoError(t, store.Create(testProject, taskstore.TaskEntry{
		Filename: testPlanFile,
		Status:   taskstore.StatusReviewing,
		Branch:   testBranch,
		PRURL:    testPRURL,
	}))
	return store
}

// makeTestRepoEntry builds a RepoEntry for the test.
func makeTestRepoEntry(repoDir string, store taskstore.Store) RepoEntry {
	return RepoEntry{
		Path:    repoDir,
		Project: testProject,
		Store:   store,
	}
}

// setupMonitor creates a PRMonitor wired with a capture-dispatch callback and
// a live broadcaster. The monitor's gh-interaction functions are NOT set —
// callers must set ghPROpen, ghListReviews, ghListComments, and ghAddReaction
// before calling pollOnce.
func setupMonitor(
	t *testing.T,
	store taskstore.Store,
	repoDir string,
	actions *[]dispatchedAction,
	broadcaster *api.EventBroadcaster,
	maxCycles int,
) *PRMonitor {
	t.Helper()

	rm := NewRepoManager()
	rm.repos = []RepoEntry{makeTestRepoEntry(repoDir, store)}

	dispatch := func(_ context.Context, e RepoEntry, action loop.Action) error {
		*actions = append(*actions, dispatchedAction{repo: e.Path, action: action})
		return nil
	}

	cfg := PRMonitorConfig{
		Enabled:      true,
		PollInterval: time.Second,
		Reactions:    []string{"eyes"},
	}
	monitor := NewPRMonitor(cfg, maxCycles, rm, broadcaster, slog.Default(), dispatch)
	return monitor
}

// collectEvents drains up to n events from ch with the given timeout.
// Returns however many arrived before the timeout.
func collectEvents(ch <-chan api.Event, n int, timeout time.Duration) []api.Event {
	out := make([]api.Event, 0, n)
	dl := time.NewTimer(timeout)
	defer dl.Stop()
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-dl.C:
			return out
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Table-driven scenarios
// ---------------------------------------------------------------------------

func TestPRMonitor_ChangesRequested_DispatchesFixer(t *testing.T) {
	store := makeTestStoreWithTask(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()
	eventCh := broadcaster.Subscribe()

	var actions []dispatchedAction
	monitor := setupMonitor(t, store, repoDir, &actions, broadcaster, 5)

	review := fakeReview(10, "CHANGES_REQUESTED", "please fix the nits", "alice")
	comment := fakeComment(101)

	monitor.ghPROpen = func(_ string, _ int) (bool, error) { return true, nil }
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) {
		return []gitpkg.PRReview{review}, nil
	}
	monitor.ghListComments = func(_ string, _, _ int) ([]gitpkg.PRReviewComment, error) {
		return []gitpkg.PRReviewComment{comment}, nil
	}
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	monitor.pollOnce(context.Background())

	// One SpawnFixerAction must have been dispatched.
	require.Len(t, actions, 1)
	fa, ok := actions[0].action.(loop.SpawnFixerAction)
	require.True(t, ok, "expected SpawnFixerAction, got %T", actions[0].action)
	assert.Equal(t, testPlanFile, fa.PlanFile)
	assert.NotEmpty(t, fa.Feedback)

	// Store: fixer_dispatched must be set — no pending reviews remain.
	pending, err := store.ListPendingReviews(testProject, testPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending, "no pending reviews expected after fixer dispatched")

	// At least two SSE events: pr_review_detected + pr_reaction_posted.
	events := collectEvents(eventCh, 2, 500*time.Millisecond)
	assert.GreaterOrEqual(t, len(events), 2, "expected at least 2 SSE events")
}

func TestPRMonitor_Approved_RecordsRowNoDispatch(t *testing.T) {
	store := makeTestStoreWithTask(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := setupMonitor(t, store, repoDir, &actions, broadcaster, 5)

	review := fakeReview(20, "APPROVED", "lgtm", "bob")

	monitor.ghPROpen = func(_ string, _ int) (bool, error) { return true, nil }
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) {
		return []gitpkg.PRReview{review}, nil
	}
	monitor.ghListComments = func(_ string, _, _ int) ([]gitpkg.PRReviewComment, error) {
		return nil, nil
	}
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	monitor.pollOnce(context.Background())

	// No fixer should be dispatched for an APPROVED review.
	assert.Empty(t, actions, "APPROVED review must not dispatch any action")

	// The review row must still be recorded in the store.
	// IsReviewProcessed returns true so we don't re-process it next poll.
	assert.True(t, store.IsReviewProcessed(testProject, testPlanFile, 20),
		"APPROVED review row must be persisted in the store")
}

func TestPRMonitor_DuplicateReview_IdempotentDispatch(t *testing.T) {
	store := makeTestStoreWithTask(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := setupMonitor(t, store, repoDir, &actions, broadcaster, 5)

	review := fakeReview(30, "CHANGES_REQUESTED", "fix the docstring", "carol")
	comment := fakeComment(201)

	monitor.ghPROpen = func(_ string, _ int) (bool, error) { return true, nil }
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) {
		return []gitpkg.PRReview{review}, nil
	}
	monitor.ghListComments = func(_ string, _, _ int) ([]gitpkg.PRReviewComment, error) {
		return []gitpkg.PRReviewComment{comment}, nil
	}
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	// Poll once — should record and dispatch.
	monitor.pollOnce(context.Background())

	// Poll again with the same review — must NOT produce a second dispatch.
	monitor.pollOnce(context.Background())

	// Exactly one action must have been dispatched across both polls.
	assert.Len(t, actions, 1, "duplicate review must dispatch fixer exactly once")

	// Exactly one persisted row.
	assert.True(t, store.IsReviewProcessed(testProject, testPlanFile, 30))

	// No row should remain in pending state.
	pending, err := store.ListPendingReviews(testProject, testPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending, "review must be fully processed after second poll")
}

func TestPRMonitor_MalformedPRURL_NoAction(t *testing.T) {
	store := taskstore.NewTestStore(t)
	// Task with a malformed PR URL.
	require.NoError(t, store.Create(testProject, taskstore.TaskEntry{
		Filename: testPlanFile,
		Status:   taskstore.StatusReviewing,
		Branch:   testBranch,
		PRURL:    "https://not-a-pr-url/foo/bar",
	}))

	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := setupMonitor(t, store, repoDir, &actions, broadcaster, 5)

	// These should never be called for a malformed URL.
	monitor.ghPROpen = func(_ string, _ int) (bool, error) {
		t.Fatal("ghPROpen must not be called for malformed PR URL")
		return false, nil
	}
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) {
		t.Fatal("ghListReviews must not be called for malformed PR URL")
		return nil, nil
	}
	monitor.ghListComments = func(_ string, _, _ int) ([]gitpkg.PRReviewComment, error) {
		t.Fatal("ghListComments must not be called for malformed PR URL")
		return nil, nil
	}
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	// Must not panic.
	monitor.pollOnce(context.Background())

	// No actions and no new rows.
	assert.Empty(t, actions)
	pending, err := store.ListPendingReviews(testProject, testPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestPRMonitor_GHAuthFailure_WarnsOnce(t *testing.T) {
	store := makeTestStoreWithTask(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	// Capture log output.
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	rm := NewRepoManager()
	rm.repos = []RepoEntry{makeTestRepoEntry(repoDir, store)}

	var actions []dispatchedAction
	dispatch := func(_ context.Context, e RepoEntry, action loop.Action) error {
		actions = append(actions, dispatchedAction{repo: e.Path, action: action})
		return nil
	}

	cfg := PRMonitorConfig{Enabled: true, PollInterval: time.Second, Reactions: []string{"eyes"}}
	monitor := NewPRMonitor(cfg, 5, rm, broadcaster, logger, dispatch)

	monitor.ghPROpen = func(_ string, _ int) (bool, error) { return true, nil }
	// Return an error that matches the "not logged in" pattern.
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) {
		return nil, fmt.Errorf("gh: not logged in to any GitHub hosts")
	}
	monitor.ghListComments = func(_ string, _, _ int) ([]gitpkg.PRReviewComment, error) {
		return nil, nil
	}
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	// First poll — should log the gh-unavailable warning.
	monitor.pollOnce(context.Background())
	// Second poll — warning must NOT appear again (warn-once).
	monitor.pollOnce(context.Background())

	// No actions dispatched.
	assert.Empty(t, actions, "auth failure must not dispatch any action")

	// Exactly one warning about gh being unavailable.
	logOutput := logBuf.String()
	occurrences := strings.Count(logOutput, "gh unavailable")
	assert.Equal(t, 1, occurrences,
		"expected exactly one 'gh unavailable' warning across two polls; got %d\nlog:\n%s",
		occurrences, logOutput)
}

func TestPRMonitor_MultipleReviews_OnlyQualifyingDispatched(t *testing.T) {
	store := makeTestStoreWithTask(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := setupMonitor(t, store, repoDir, &actions, broadcaster, 5)

	reviews := []gitpkg.PRReview{
		fakeReview(40, "APPROVED", "looks good", "eve"),                 // not dispatched (APPROVED)
		fakeReview(41, "CHANGES_REQUESTED", "fix the test", "mallory"),  // dispatched
		fakeReview(42, "COMMENTED", "nit: rename var", "renovate[bot]"), // not dispatched (bot)
	}

	monitor.ghPROpen = func(_ string, _ int) (bool, error) { return true, nil }
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) {
		return reviews, nil
	}
	monitor.ghListComments = func(_ string, _, reviewID int) ([]gitpkg.PRReviewComment, error) {
		if reviewID == 41 {
			return []gitpkg.PRReviewComment{{ID: 301}}, nil
		}
		return nil, nil
	}
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	monitor.pollOnce(context.Background())

	// Only the CHANGES_REQUESTED review (ID 41) should produce a SpawnFixerAction.
	require.Len(t, actions, 1, "only CHANGES_REQUESTED review from human must dispatch fixer")
	_, ok := actions[0].action.(loop.SpawnFixerAction)
	assert.True(t, ok, "expected SpawnFixerAction for CHANGES_REQUESTED review")

	// All three reviews must be persisted in the store.
	assert.True(t, store.IsReviewProcessed(testProject, testPlanFile, 40), "APPROVED review should be recorded")
	assert.True(t, store.IsReviewProcessed(testProject, testPlanFile, 41), "CHANGES_REQUESTED review should be recorded")
	assert.True(t, store.IsReviewProcessed(testProject, testPlanFile, 42), "COMMENTED review should be recorded")
}

func TestPRMonitor_ReviewCycleLimit_DispatchesLimitAction(t *testing.T) {
	store := makeTestStoreWithTask(t)

	// Set the task's review cycle to the limit.
	const maxCycles = 3
	task, err := store.Get(testProject, testPlanFile)
	require.NoError(t, err)
	task.ReviewCycle = maxCycles // at the limit — next cycle would exceed it
	require.NoError(t, store.Update(testProject, testPlanFile, task))

	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := setupMonitor(t, store, repoDir, &actions, broadcaster, maxCycles)

	review := fakeReview(50, "CHANGES_REQUESTED", "more changes needed", "frank")
	comment := fakeComment(401)

	monitor.ghPROpen = func(_ string, _ int) (bool, error) { return true, nil }
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) {
		return []gitpkg.PRReview{review}, nil
	}
	monitor.ghListComments = func(_ string, _, _ int) ([]gitpkg.PRReviewComment, error) {
		return []gitpkg.PRReviewComment{comment}, nil
	}
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	monitor.pollOnce(context.Background())

	// Must dispatch ReviewCycleLimitAction instead of SpawnFixerAction.
	require.Len(t, actions, 1)
	la, ok := actions[0].action.(loop.ReviewCycleLimitAction)
	require.True(t, ok, "expected ReviewCycleLimitAction when cycle limit exceeded, got %T", actions[0].action)
	assert.Equal(t, testPlanFile, la.PlanFile)
	assert.Greater(t, la.Cycle, la.Limit-1, "cycle must be >= limit")
}

func TestPRMonitor_ClosedPR_SkipsPolling(t *testing.T) {
	store := makeTestStoreWithTask(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := setupMonitor(t, store, repoDir, &actions, broadcaster, 5)

	// ghPROpen returns false (PR is closed or merged).
	monitor.ghPROpen = func(_ string, _ int) (bool, error) { return false, nil }
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) {
		t.Fatal("ghListReviews must not be called for a closed PR")
		return nil, nil
	}
	monitor.ghListComments = func(_ string, _, _ int) ([]gitpkg.PRReviewComment, error) {
		t.Fatal("ghListComments must not be called for a closed PR")
		return nil, nil
	}
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	monitor.pollOnce(context.Background())

	assert.Empty(t, actions, "closed PR must not dispatch any action")

	// Task metadata must be unchanged.
	task, err := store.Get(testProject, testPlanFile)
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusReviewing, task.Status)

	pending, err := store.ListPendingReviews(testProject, testPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

// ---------------------------------------------------------------------------
// Run / cancellation test
// ---------------------------------------------------------------------------

func TestPRMonitor_Run_StopsOnContextCancellation(t *testing.T) {
	store := makeTestStoreWithTask(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := setupMonitor(t, store, repoDir, &actions, broadcaster, 5)

	// Short poll interval so the ticker fires quickly.
	monitor.cfg.PollInterval = 50 * time.Millisecond

	// All gh calls return empty / open.
	monitor.ghPROpen = func(_ string, _ int) (bool, error) { return true, nil }
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) { return nil, nil }
	monitor.ghListComments = func(_ string, _, _ int) ([]gitpkg.PRReviewComment, error) { return nil, nil }
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		monitor.Run(ctx)
	}()

	// Allow a couple of ticks to run.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Run returned cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("PRMonitor.Run did not return after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// Edge-case: task without PRURL is ignored
// ---------------------------------------------------------------------------

func TestPRMonitor_TaskWithoutPRURL_Ignored(t *testing.T) {
	store := taskstore.NewTestStore(t)
	// Task in reviewing status but with no PRURL.
	require.NoError(t, store.Create(testProject, taskstore.TaskEntry{
		Filename: "no-pr.md",
		Status:   taskstore.StatusReviewing,
		Branch:   testBranch,
		// PRURL intentionally empty.
	}))

	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	rm := NewRepoManager()
	rm.repos = []RepoEntry{{Path: repoDir, Project: testProject, Store: store}}

	dispatch := func(_ context.Context, e RepoEntry, action loop.Action) error {
		actions = append(actions, dispatchedAction{repo: e.Path, action: action})
		return nil
	}

	cfg := PRMonitorConfig{Enabled: true, PollInterval: time.Second, Reactions: []string{"eyes"}}
	monitor := NewPRMonitor(cfg, 5, rm, broadcaster, slog.Default(), dispatch)

	// These must NOT be called.
	monitor.ghPROpen = func(_ string, _ int) (bool, error) {
		t.Fatal("ghPROpen must not be called when PRURL is empty")
		return false, nil
	}
	monitor.ghListReviews = func(_ string, _ int) ([]gitpkg.PRReview, error) {
		t.Fatal("ghListReviews must not be called when PRURL is empty")
		return nil, nil
	}
	monitor.ghListComments = func(_ string, _, _ int) ([]gitpkg.PRReviewComment, error) {
		return nil, nil
	}
	monitor.ghAddReaction = func(_ string, _ int, _ string) error { return nil }

	monitor.pollOnce(context.Background())
	assert.Empty(t, actions)
}
