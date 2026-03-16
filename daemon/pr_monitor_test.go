package daemon

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/daemon/api"
	"github.com/kastheco/kasmos/orchestration/loop"
	gitpkg "github.com/kastheco/kasmos/session/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Pure helper tests ────────────────────────────────────────────────────────

func TestShouldTriggerFixer(t *testing.T) {
	tests := []struct {
		name   string
		review gitpkg.PRReview
		want   bool
	}{
		{
			name:   "changes_requested with body from human",
			review: gitpkg.PRReview{State: "CHANGES_REQUESTED", Body: "fix the nits", User: "alice"},
			want:   true,
		},
		{
			name:   "changes_requested with empty body still triggers",
			review: gitpkg.PRReview{State: "CHANGES_REQUESTED", Body: "   ", User: "dave"},
			want:   true,
		},
		{
			name:   "commented with body from human",
			review: gitpkg.PRReview{State: "COMMENTED", Body: "please refactor this", User: "bob"},
			want:   true,
		},
		{
			name:   "approved — should not trigger",
			review: gitpkg.PRReview{State: "APPROVED", Body: "lgtm", User: "carol"},
			want:   false,
		},
		{
			name:   "commented but empty body",
			review: gitpkg.PRReview{State: "COMMENTED", Body: "   ", User: "dave"},
			want:   false,
		},
		{
			name:   "changes_requested from github-actions bot",
			review: gitpkg.PRReview{State: "CHANGES_REQUESTED", Body: "lint failed", User: "github-actions[bot]"},
			want:   false,
		},
		{
			name:   "commented from renovate bot",
			review: gitpkg.PRReview{State: "COMMENTED", Body: "bump dep", User: "renovate[bot]"},
			want:   false,
		},
		{
			name:   "changes_requested from dependabot",
			review: gitpkg.PRReview{State: "CHANGES_REQUESTED", Body: "please update", User: "dependabot"},
			want:   false,
		},
		{
			name:   "pending state — should not trigger",
			review: gitpkg.PRReview{State: "PENDING", Body: "draft review", User: "alice"},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldTriggerFixer(tt.review))
		})
	}
}

func TestIsBotLogin(t *testing.T) {
	tests := []struct {
		login string
		want  bool
	}{
		{"github-actions[bot]", true},
		{"dependabot[bot]", true},
		{"renovate[bot]", true},
		{"coderabbitai[bot]", true},
		{"dependabot", true},
		{"auto-bot", true},
		{"release-bot", true},
		// human accounts
		{"alice", false},
		{"bob-the-human", false},
		{"dave", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.login, func(t *testing.T) {
			assert.Equal(t, tt.want, isBotLogin(tt.login))
		})
	}
}

// ─── Constructor ──────────────────────────────────────────────────────────────

func TestNewPRMonitor_DefaultsApplied(t *testing.T) {
	cfg := PRMonitorConfig{} // zero value — no interval, no reactions
	m := NewPRMonitor(cfg, 0, NewRepoManager(), api.NewEventBroadcaster(), slog.Default(), nil)

	assert.Greater(t, m.cfg.PollInterval, time.Duration(0))
	assert.NotEmpty(t, m.cfg.Reactions)
}

// ─── Run / lifecycle ──────────────────────────────────────────────────────────

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
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()

	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err, "Run must return nil on context cancellation")
	case <-time.After(2 * time.Second):
		t.Fatal("PRMonitor.Run did not exit within 2s after context cancellation")
	}
}

// ─── pollOnce — repo/task-level filtering ────────────────────────────────────

func TestPRMonitor_PollOnce_SkipsNilStore(t *testing.T) {
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

	repos.mu.Lock()
	repos.repos = append(repos.repos, RepoEntry{
		Path:    t.TempDir(),
		Project: "no-store",
		Store:   nil,
	})
	repos.mu.Unlock()

	m.pollOnce(context.Background())
	assert.Equal(t, 0, dispatched, "expected no dispatch for repo without store")
}

func TestPRMonitor_PollOnce_SkipsTasksWithoutPRURLOrBranch(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"

	// Task without PRURL.
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "no-url.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "review/no-url",
	}))
	// Task without Branch.
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "no-branch.md",
		Status:   taskstore.StatusReviewing,
		PRURL:    "https://github.com/owner/repo/pull/1",
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
	// No gh injection needed — tasks are filtered before any gh call.
	m.pollOnce(context.Background())
	assert.Equal(t, 0, dispatched, "expected no dispatch for tasks missing PRURL or Branch")
}

func TestPRMonitor_PollOnce_SkipsClosedPR(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "review/plan",
		PRURL:    "https://github.com/owner/repo/pull/99",
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
	m.ghPROpen = func(string, int) (bool, error) { return false, nil } // PR is closed
	m.ghListReviews = func(string, int) ([]gitpkg.PRReview, error) {
		t.Fatal("ListReviews should not be called for a closed PR")
		return nil, nil
	}

	m.pollOnce(context.Background())
	assert.Equal(t, 0, dispatched, "expected no dispatch for closed PR")
}

// ─── Test helper ─────────────────────────────────────────────────────────────

// monitorTestFixture bundles a PRMonitor with its log buffer and collected
// dispatch actions for easy test assertion.
type monitorTestFixture struct {
	m       *PRMonitor
	logBuf  *bytes.Buffer
	actions *[]loop.Action
	mu      sync.Mutex
}

// newMonitorFixture creates a PRMonitor wired to in-memory test doubles.
// Pass noop stubs for git functions that the specific test does not need.
func newMonitorFixture(
	t *testing.T,
	project, repoPath string,
	store taskstore.Store,
	maxCycles int,
	ghPROpen func(string, int) (bool, error),
	ghListReviews func(string, int) ([]gitpkg.PRReview, error),
	ghListComments func(string, int, int) ([]gitpkg.PRReviewComment, error),
	ghAddReaction func(string, int, string) error,
) *monitorTestFixture {
	t.Helper()
	f := &monitorTestFixture{
		actions: &[]loop.Action{},
	}
	f.logBuf = &bytes.Buffer{}

	logger := slog.New(slog.NewTextHandler(f.logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	repos := NewRepoManager()
	repos.mu.Lock()
	repos.repos = append(repos.repos, RepoEntry{
		Path:    repoPath,
		Project: project,
		Store:   store,
	})
	repos.mu.Unlock()

	dispatch := func(_ context.Context, _ RepoEntry, action loop.Action) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		*f.actions = append(*f.actions, action)
		return nil
	}

	m := NewPRMonitor(
		PRMonitorConfig{PollInterval: time.Hour, Reactions: []string{"eyes"}},
		maxCycles,
		repos,
		api.NewEventBroadcaster(),
		logger,
		dispatch,
	)
	m.ghPROpen = ghPROpen
	m.ghListReviews = ghListReviews
	m.ghListComments = ghListComments
	m.ghAddReaction = ghAddReaction

	f.m = m
	return f
}

// ─── handleReview — dispatch happy path ──────────────────────────────────────

func TestPRMonitor_HandleReview_DispatchesFixer(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	repoPath := t.TempDir()
	const prNum = 42

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "review/plan",
		PRURL:    fmt.Sprintf("https://github.com/owner/repo/pull/%d", prNum),
	}))

	fakeReview := gitpkg.PRReview{
		ID: 10, State: "CHANGES_REQUESTED", Body: "please address the nits", User: "alice",
	}

	f := newMonitorFixture(t, project, repoPath, store, 0,
		func(string, int) (bool, error) { return true, nil },
		func(string, int) ([]gitpkg.PRReview, error) { return []gitpkg.PRReview{fakeReview}, nil },
		func(string, int, int) ([]gitpkg.PRReviewComment, error) { return nil, nil },
		func(string, int, string) error { return nil },
	)

	f.m.pollOnce(context.Background())

	require.Len(t, *f.actions, 1)
	act, ok := (*f.actions)[0].(loop.SpawnFixerAction)
	require.True(t, ok, "expected SpawnFixerAction, got %T", (*f.actions)[0])
	assert.Equal(t, "plan.md", act.PlanFile)
	assert.Equal(t, fakeReview.Body, act.Feedback)

	// Review should be marked dispatched — ListPendingReviews returns empty.
	pending, err := store.ListPendingReviews(project, "plan.md")
	require.NoError(t, err)
	assert.Empty(t, pending, "review must not be pending after successful dispatch")
}

func TestPRMonitor_HandleReview_IdempotentOnReDispatch(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	repoPath := t.TempDir()
	const prNum = 7

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "review/plan",
		PRURL:    fmt.Sprintf("https://github.com/owner/repo/pull/%d", prNum),
	}))

	fakeReview := gitpkg.PRReview{
		ID: 20, State: "CHANGES_REQUESTED", Body: "fix it", User: "alice",
	}

	f := newMonitorFixture(t, project, repoPath, store, 0,
		func(string, int) (bool, error) { return true, nil },
		func(string, int) ([]gitpkg.PRReview, error) { return []gitpkg.PRReview{fakeReview}, nil },
		func(string, int, int) ([]gitpkg.PRReviewComment, error) { return nil, nil },
		func(string, int, string) error { return nil },
	)

	// First poll — dispatches fixer.
	f.m.pollOnce(context.Background())
	require.Len(t, *f.actions, 1)

	// Second poll — review is already dispatched; no new action.
	f.m.pollOnce(context.Background())
	assert.Len(t, *f.actions, 1, "fixer must not be re-dispatched on subsequent polls")
}

func TestPRMonitor_HandleReview_StatusDoneAlsoPolleed(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	repoPath := t.TempDir()
	const prNum = 55

	// Task is in "done" status — should still be polled for PR reviews.
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "done-plan.md",
		Status:   taskstore.StatusDone,
		Branch:   "review/done-plan",
		PRURL:    fmt.Sprintf("https://github.com/owner/repo/pull/%d", prNum),
	}))

	fakeReview := gitpkg.PRReview{
		ID: 3, State: "COMMENTED", Body: "minor nit", User: "reviewer",
	}

	f := newMonitorFixture(t, project, repoPath, store, 0,
		func(string, int) (bool, error) { return true, nil },
		func(string, int) ([]gitpkg.PRReview, error) { return []gitpkg.PRReview{fakeReview}, nil },
		func(string, int, int) ([]gitpkg.PRReviewComment, error) { return nil, nil },
		func(string, int, string) error { return nil },
	)

	f.m.pollOnce(context.Background())

	require.Len(t, *f.actions, 1)
	_, ok := (*f.actions)[0].(loop.SpawnFixerAction)
	assert.True(t, ok, "expected SpawnFixerAction for done task, got %T", (*f.actions)[0])
}

// ─── Review-fix cycle limit ───────────────────────────────────────────────────

func TestPRMonitor_HandleReview_CycleLimit(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	repoPath := t.TempDir()
	const prNum = 3
	const maxCycles = 2

	// ReviewCycle is at the limit — next would be ReviewCycle+1 = 3 > 2.
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "plan.md",
		Status:      taskstore.StatusReviewing,
		Branch:      "review/plan",
		PRURL:       fmt.Sprintf("https://github.com/owner/repo/pull/%d", prNum),
		ReviewCycle: maxCycles,
	}))

	fakeReview := gitpkg.PRReview{
		ID: 5, State: "CHANGES_REQUESTED", Body: "more nits", User: "alice",
	}

	f := newMonitorFixture(t, project, repoPath, store, maxCycles,
		func(string, int) (bool, error) { return true, nil },
		func(string, int) ([]gitpkg.PRReview, error) { return []gitpkg.PRReview{fakeReview}, nil },
		func(string, int, int) ([]gitpkg.PRReviewComment, error) { return nil, nil },
		func(string, int, string) error { return nil },
	)

	f.m.pollOnce(context.Background())

	require.Len(t, *f.actions, 1)
	act, ok := (*f.actions)[0].(loop.ReviewCycleLimitAction)
	require.True(t, ok, "expected ReviewCycleLimitAction, got %T", (*f.actions)[0])
	assert.Equal(t, "plan.md", act.PlanFile)
	assert.Equal(t, maxCycles+1, act.Cycle)
	assert.Equal(t, maxCycles, act.Limit)
}

// ─── Reaction path ────────────────────────────────────────────────────────────

func TestPRMonitor_ReactsToFirstComment(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	repoPath := t.TempDir()
	const prNum = 9

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "review/plan",
		PRURL:    fmt.Sprintf("https://github.com/owner/repo/pull/%d", prNum),
	}))

	fakeReview := gitpkg.PRReview{
		ID: 1, State: "CHANGES_REQUESTED", Body: "some feedback", User: "alice",
	}

	var reactedCommentIDs []int
	var reactedReactions []string

	f := newMonitorFixture(t, project, repoPath, store, 0,
		func(string, int) (bool, error) { return true, nil },
		func(string, int) ([]gitpkg.PRReview, error) { return []gitpkg.PRReview{fakeReview}, nil },
		func(string, int, int) ([]gitpkg.PRReviewComment, error) {
			// Return two comments — only the first should be reacted to.
			return []gitpkg.PRReviewComment{{ID: 101}, {ID: 102}}, nil
		},
		func(_ string, commentID int, reaction string) error {
			reactedCommentIDs = append(reactedCommentIDs, commentID)
			reactedReactions = append(reactedReactions, reaction)
			return nil
		},
	)

	f.m.pollOnce(context.Background())

	// Only the FIRST comment receives the reaction.
	require.Len(t, reactedCommentIDs, 1, "expected exactly one comment reacted to")
	assert.Equal(t, 101, reactedCommentIDs[0])
	assert.Equal(t, []string{"eyes"}, reactedReactions)

	// Fixer should have been dispatched too.
	require.Len(t, *f.actions, 1)
	_, ok := (*f.actions)[0].(loop.SpawnFixerAction)
	assert.True(t, ok)
}

func TestPRMonitor_ReactionFailure_LeavesFixerPending(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	repoPath := t.TempDir()
	const prNum = 11

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "review/plan",
		PRURL:    fmt.Sprintf("https://github.com/owner/repo/pull/%d", prNum),
	}))

	fakeReview := gitpkg.PRReview{
		ID: 77, State: "CHANGES_REQUESTED", Body: "feedback", User: "alice",
	}

	f := newMonitorFixture(t, project, repoPath, store, 0,
		func(string, int) (bool, error) { return true, nil },
		func(string, int) ([]gitpkg.PRReview, error) { return []gitpkg.PRReview{fakeReview}, nil },
		func(string, int, int) ([]gitpkg.PRReviewComment, error) {
			return []gitpkg.PRReviewComment{{ID: 55}}, nil
		},
		func(string, int, string) error { return fmt.Errorf("rate limited") },
	)

	f.m.pollOnce(context.Background())

	// Fixer must NOT have been dispatched because the reaction failed.
	assert.Empty(t, *f.actions, "fixer must not be dispatched when reaction fails")

	// Review must remain pending so the next poll retries.
	pending, err := store.ListPendingReviews(project, "plan.md")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, 77, pending[0].ReviewID)
	assert.False(t, pending[0].ReactionPosted)
}

func TestPRMonitor_NoInlineComments_StillDispatchesFixer(t *testing.T) {
	// When a review has no inline comments, the monitor must still dispatch
	// the fixer (reaction is best-effort per task spec).
	store := taskstore.NewTestStore(t)
	project := "test-project"
	repoPath := t.TempDir()
	const prNum = 13

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "review/plan",
		PRURL:    fmt.Sprintf("https://github.com/owner/repo/pull/%d", prNum),
	}))

	fakeReview := gitpkg.PRReview{
		ID: 88, State: "COMMENTED", Body: "please check this", User: "alice",
	}

	f := newMonitorFixture(t, project, repoPath, store, 0,
		func(string, int) (bool, error) { return true, nil },
		func(string, int) ([]gitpkg.PRReview, error) { return []gitpkg.PRReview{fakeReview}, nil },
		func(string, int, int) ([]gitpkg.PRReviewComment, error) {
			return []gitpkg.PRReviewComment{}, nil // body-only review, no inline comments
		},
		func(string, int, string) error { return nil },
	)

	f.m.pollOnce(context.Background())

	require.Len(t, *f.actions, 1)
	_, ok := (*f.actions)[0].(loop.SpawnFixerAction)
	assert.True(t, ok, "expected SpawnFixerAction even with no inline comments; got %T", (*f.actions)[0])
}

func TestPRMonitor_ChangesRequested_EmptyBodyWithInlineComments_DispatchesFixer(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	repoPath := t.TempDir()
	const prNum = 17

	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename: "plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "review/plan",
		PRURL:    fmt.Sprintf("https://github.com/owner/repo/pull/%d", prNum),
	}))

	fakeReview := gitpkg.PRReview{
		ID: 91, State: "CHANGES_REQUESTED", Body: "", User: "alice",
	}

	f := newMonitorFixture(t, project, repoPath, store, 0,
		func(string, int) (bool, error) { return true, nil },
		func(string, int) ([]gitpkg.PRReview, error) { return []gitpkg.PRReview{fakeReview}, nil },
		func(string, int, int) ([]gitpkg.PRReviewComment, error) {
			return []gitpkg.PRReviewComment{{ID: 701}}, nil
		},
		func(string, int, string) error { return nil },
	)

	f.m.pollOnce(context.Background())

	require.Len(t, *f.actions, 1)
	act, ok := (*f.actions)[0].(loop.SpawnFixerAction)
	require.True(t, ok, "expected SpawnFixerAction, got %T", (*f.actions)[0])
	assert.Equal(t, "plan.md", act.PlanFile)

	pending, err := store.ListPendingReviews(project, "plan.md")
	require.NoError(t, err)
	assert.Empty(t, pending, "review must not remain pending after successful dispatch")
}

// ─── warn-once gh unavailability ─────────────────────────────────────────────

func TestPRMonitor_WarnOnce_GHUnavailable(t *testing.T) {
	store := taskstore.NewTestStore(t)
	project := "test-project"
	repoPath := t.TempDir()

	// Create two tasks so that, without the warn-once guard, the log would have
	// two warning lines.
	for i, name := range []string{"plan-a.md", "plan-b.md"} {
		require.NoError(t, store.Create(project, taskstore.TaskEntry{
			Filename: name,
			Status:   taskstore.StatusReviewing,
			Branch:   fmt.Sprintf("review/plan-%d", i),
			PRURL:    fmt.Sprintf("https://github.com/owner/repo/pull/%d", i+1),
		}))
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	repos := NewRepoManager()
	repos.mu.Lock()
	repos.repos = append(repos.repos, RepoEntry{
		Path:    repoPath,
		Project: project,
		Store:   store,
	})
	repos.mu.Unlock()

	m := NewPRMonitor(
		PRMonitorConfig{PollInterval: time.Hour, Reactions: []string{"eyes"}},
		0,
		repos,
		api.NewEventBroadcaster(),
		logger,
		func(_ context.Context, _ RepoEntry, _ loop.Action) error { return nil },
	)
	// ghPROpen returns an error that matches the "not logged in" unavailability pattern.
	m.ghPROpen = func(string, int) (bool, error) {
		return false, fmt.Errorf("gh: not logged in to any GitHub hosts")
	}

	// Two consecutive poll cycles.
	m.pollOnce(context.Background())
	m.pollOnce(context.Background())

	logOutput := buf.String()
	occurrences := strings.Count(logOutput, "gh unavailable")
	assert.Equal(t, 1, occurrences,
		"expected exactly one 'gh unavailable' warning across two poll cycles; got %d\nlog:\n%s",
		occurrences, logOutput)
}
