package daemon

// Integration-style tests for PRMonitor that drive full poll cycles using an
// in-memory store and a mock gh executor. No real tmux, no network, no real gh.
//
// These tests call monitor.poll(ctx) directly and verify persisted state via
// Store methods rather than white-boxing monitor internals.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"testing"
	"time"

	cmd_test "github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/daemon/api"
	"github.com/kastheco/kasmos/orchestration/loop"
	gitpkg "github.com/kastheco/kasmos/session/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Shared harness types
// ---------------------------------------------------------------------------

// dispatchedAction captures an action dispatched by PRMonitor during a test
// poll cycle.
type dispatchedAction struct {
	repo   string
	action loop.Action
}

// ---------------------------------------------------------------------------
// Mock GH executor helpers
// ---------------------------------------------------------------------------

type ghOutputFn = func(*exec.Cmd) ([]byte, error)
type ghRunFn = func(*exec.Cmd) error

// seqMock builds a MockCmdExec whose Output and Run functions respond with the
// given sequence of handlers in order. Unexpected calls trigger t.Fatalf.
func seqMock(t *testing.T, outputs []ghOutputFn, runs []ghRunFn) *cmd_test.MockCmdExec {
	t.Helper()
	oIdx := 0
	rIdx := 0
	return &cmd_test.MockCmdExec{
		OutputFunc: func(c *exec.Cmd) ([]byte, error) {
			if oIdx >= len(outputs) {
				t.Fatalf("unexpected gh Output call #%d (args: %v)", oIdx, c.Args)
			}
			fn := outputs[oIdx]
			oIdx++
			return fn(c)
		},
		RunFunc: func(c *exec.Cmd) error {
			if rIdx >= len(runs) {
				t.Fatalf("unexpected gh Run call #%d (args: %v)", rIdx, c.Args)
			}
			fn := runs[rIdx]
			rIdx++
			return fn(c)
		},
	}
}

// staticOut returns a ghOutputFn that always returns the given JSON bytes.
func staticOut(data string) ghOutputFn {
	return func(*exec.Cmd) ([]byte, error) { return []byte(data), nil }
}

// errOut returns a ghOutputFn that always returns the given error.
func errOut(msg string) ghOutputFn {
	return func(*exec.Cmd) ([]byte, error) { return nil, fmt.Errorf("%s", msg) }
}

// runOK is a ghRunFn that always returns nil.
func runOK(*exec.Cmd) error { return nil }

const repoViewJSON = `{"nameWithOwner":"owner/repo"}`

// prOpenJSON builds the JSON response for a PR open/closed state.
func prOpenJSON(open bool) string {
	state := "open"
	if !open {
		state = "closed"
	}
	return fmt.Sprintf(`{"state":%q,"merged_at":null}`, state)
}

// rawReview is used only for JSON marshalling inside reviewsJSON.
type rawReview struct {
	ID    int    `json:"id"`
	State string `json:"state"`
	Body  string `json:"body"`
	User  struct {
		Login string `json:"login"`
	} `json:"user"`
	SubmittedAt string `json:"submitted_at"`
}

// reviewsJSON builds the JSON response for a list of PR reviews.
func reviewsJSON(reviews []rawReview) string {
	b, _ := json.Marshal(reviews)
	return string(b)
}

// commentsJSON builds the JSON response for a list of review comment IDs.
func commentsJSON(ids ...int) string {
	type c struct {
		ID int `json:"id"`
	}
	cs := make([]c, len(ids))
	for i, id := range ids {
		cs[i] = c{ID: id}
	}
	b, _ := json.Marshal(cs)
	return string(b)
}

// makeRawReview is a convenience builder for rawReview.
func makeRawReview(id int, state, body, login string) rawReview {
	r := rawReview{ID: id, State: state, Body: body, SubmittedAt: "2024-01-01T12:00:00Z"}
	r.User.Login = login
	return r
}

// ---------------------------------------------------------------------------
// Test setup helpers
// ---------------------------------------------------------------------------

const (
	intTestProject  = "proj"
	intTestPlanFile = "plan.md"
	intTestBranch   = "plan/plan"
	intTestPRURL    = "https://github.com/owner/repo/pull/42"
)

// newIntTestStore creates an in-memory store and inserts one reviewing task
// with a valid PRURL.
func newIntTestStore(t *testing.T) taskstore.Store {
	t.Helper()
	store := taskstore.NewTestStore(t)
	require.NoError(t, store.Create(intTestProject, taskstore.TaskEntry{
		Filename: intTestPlanFile,
		Status:   taskstore.StatusReviewing,
		Branch:   intTestBranch,
		PRURL:    intTestPRURL,
	}))
	return store
}

// newIntTestMonitor assembles a PRMonitor with the given store and dispatch
// capture slice.
func newIntTestMonitor(
	t *testing.T,
	store taskstore.Store,
	repoDir string,
	actions *[]dispatchedAction,
	broadcaster *api.EventBroadcaster,
	maxCycles int,
) *PRMonitor {
	t.Helper()

	rm := NewRepoManager()
	rm.repos = []RepoEntry{{
		Path:    repoDir,
		Project: intTestProject,
		Store:   store,
	}}

	dispatch := func(ctx context.Context, e RepoEntry, action loop.Action) error {
		*actions = append(*actions, dispatchedAction{repo: e.Path, action: action})
		return nil
	}

	cfg := PRMonitorConfig{
		Enabled:      true,
		PollInterval: time.Second,
		Reactions:    []string{"eyes"},
	}
	return NewPRMonitor(cfg, maxCycles, rm, broadcaster, slog.Default(), dispatch)
}

// collectEvents drains up to n events from ch with the given timeout.
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
// Integration tests
// ---------------------------------------------------------------------------

// TestPRMonitorInt_ChangesRequested_DispatchesFixer covers the primary happy
// path: a CHANGES_REQUESTED review with one inline comment causes PRMonitor to
// record the review, post the "eyes" reaction, dispatch a SpawnFixerAction, and
// emit at least one SSE event.
func TestPRMonitorInt_ChangesRequested_DispatchesFixer(t *testing.T) {
	store := newIntTestStore(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()
	eventCh := broadcaster.Subscribe()

	var actions []dispatchedAction
	monitor := newIntTestMonitor(t, store, repoDir, &actions, broadcaster, 5)

	// gh call sequence:
	// 1 PROpen: view + pulls/42
	// 2 ListPRReviews: view + reviews
	// 3 ListReviewComments (review 10): view + reviews/10/comments
	// 4 AddReviewReaction (comment 101): view; then Run POST /reactions
	mock := seqMock(t,
		[]ghOutputFn{
			staticOut(repoViewJSON),
			staticOut(prOpenJSON(true)),
			staticOut(repoViewJSON),
			staticOut(reviewsJSON([]rawReview{makeRawReview(10, "CHANGES_REQUESTED", "please fix the nits", "alice")})),
			staticOut(repoViewJSON),
			staticOut(commentsJSON(101)),
			staticOut(repoViewJSON),
		},
		[]ghRunFn{runOK},
	)
	t.Cleanup(gitpkg.SetGHExec(mock))

	monitor.pollOnce(context.Background())

	// One SpawnFixerAction must have been dispatched.
	require.Len(t, actions, 1)
	fa, ok := actions[0].action.(loop.SpawnFixerAction)
	require.True(t, ok, "expected SpawnFixerAction, got %T", actions[0].action)
	assert.Equal(t, intTestPlanFile, fa.PlanFile)
	assert.NotEmpty(t, fa.Feedback)

	// Review row must be recorded.
	assert.True(t, store.IsReviewProcessed(intTestProject, intTestPlanFile, 10),
		"review 10 must be recorded in pr_reviews table")

	// fixer_dispatched = 1 — no pending reviews remain.
	pending, err := store.ListPendingReviews(intTestProject, intTestPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending, "no pending reviews expected after fixer dispatched")

	// At least one SSE event must have been emitted (pr_review_detected is
	// emitted before the fixer dispatch; pr_reaction_posted follows the reaction).
	events := collectEvents(eventCh, 2, 500*time.Millisecond)
	assert.NotEmpty(t, events, "expected at least one SSE event from PRMonitor")
	kinds := make([]string, len(events))
	for i, ev := range events {
		kinds[i] = ev.Kind
	}
	assert.Contains(t, kinds, "pr_review_detected",
		"expected 'pr_review_detected' SSE event, got: %v", kinds)
}

func TestPRMonitorInt_ChangesRequested_EmptyBodyWithInlineComments_DispatchesFixer(t *testing.T) {
	store := newIntTestStore(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := newIntTestMonitor(t, store, repoDir, &actions, broadcaster, 5)

	mock := seqMock(t,
		[]ghOutputFn{
			staticOut(repoViewJSON),
			staticOut(prOpenJSON(true)),
			staticOut(repoViewJSON),
			staticOut(reviewsJSON([]rawReview{makeRawReview(11, "CHANGES_REQUESTED", "", "alice")})),
			staticOut(repoViewJSON),
			staticOut(commentsJSON(111)),
			staticOut(repoViewJSON),
		},
		[]ghRunFn{runOK},
	)
	t.Cleanup(gitpkg.SetGHExec(mock))

	monitor.pollOnce(context.Background())

	require.Len(t, actions, 1)
	fa, ok := actions[0].action.(loop.SpawnFixerAction)
	require.True(t, ok, "expected SpawnFixerAction, got %T", actions[0].action)
	assert.Equal(t, intTestPlanFile, fa.PlanFile)
	assert.True(t, store.IsReviewProcessed(intTestProject, intTestPlanFile, 11))

	pending, err := store.ListPendingReviews(intTestProject, intTestPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending, "review must be fully processed after fixer dispatch")
}

// TestPRMonitorInt_Approved_RecordsRow verifies that an APPROVED review with
// body text is recorded in the store but does NOT dispatch a fixer or post a
// reaction. shouldTriggerFixer only fires for CHANGES_REQUESTED and COMMENTED.
func TestPRMonitorInt_Approved_RecordsRow(t *testing.T) {
	store := newIntTestStore(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := newIntTestMonitor(t, store, repoDir, &actions, broadcaster, 5)

	// gh call sequence:
	// 1 PROpen: view + pulls/42
	// 2 ListPRReviews: view + reviews (returns APPROVED with body)
	//
	// Per spec: APPROVED reviews must NOT trigger reaction posts or fixer dispatch.
	// The seqMock will t.Fatalf if ListReviewComments or AddReviewReaction are called.
	mock := seqMock(t,
		[]ghOutputFn{
			staticOut(repoViewJSON),
			staticOut(prOpenJSON(true)),
			staticOut(repoViewJSON),
			staticOut(reviewsJSON([]rawReview{makeRawReview(20, "APPROVED", "lgtm", "bob")})),
		},
		[]ghRunFn{}, // no Run calls expected for APPROVED
	)
	t.Cleanup(gitpkg.SetGHExec(mock))

	monitor.pollOnce(context.Background())

	// Review row must be persisted so re-polls don't re-process it.
	assert.True(t, store.IsReviewProcessed(intTestProject, intTestPlanFile, 20),
		"APPROVED review row must be recorded in pr_reviews table")

	// No fixer should be dispatched for an APPROVED review.
	assert.Empty(t, actions, "APPROVED review must not dispatch any action")
}

// TestPRMonitorInt_DuplicateReview_IdempotentDispatch verifies that the same
// CHANGES_REQUESTED review returned across two poll cycles dispatches the fixer
// exactly once and leaves no row in the pending state after the second poll.
func TestPRMonitorInt_DuplicateReview_IdempotentDispatch(t *testing.T) {
	store := newIntTestStore(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := newIntTestMonitor(t, store, repoDir, &actions, broadcaster, 5)

	reviewResp := reviewsJSON([]rawReview{makeRawReview(30, "CHANGES_REQUESTED", "fix the docstring", "carol")})

	// First poll: full sequence (PROpen + reviews + comments + reaction).
	// Second poll: PROpen + reviews only; review already processed, so
	// ListReviewComments and AddReviewReaction must NOT be called again.
	mock := seqMock(t,
		[]ghOutputFn{
			// Poll 1
			staticOut(repoViewJSON), staticOut(prOpenJSON(true)),
			staticOut(repoViewJSON), staticOut(reviewResp),
			staticOut(repoViewJSON), staticOut(commentsJSON(201)),
			staticOut(repoViewJSON),
			// Poll 2 (review already recorded — skipped after RecordPRReview INSERT OR IGNORE)
			staticOut(repoViewJSON), staticOut(prOpenJSON(true)),
			staticOut(repoViewJSON), staticOut(reviewResp),
		},
		[]ghRunFn{runOK}, // only one reaction POST (poll 1)
	)
	t.Cleanup(gitpkg.SetGHExec(mock))

	monitor.pollOnce(context.Background())
	monitor.pollOnce(context.Background())

	// Exactly one action across both polls.
	assert.Len(t, actions, 1, "duplicate review must dispatch fixer exactly once")

	// Review must be fully processed (fixer_dispatched = 1).
	pending, err := store.ListPendingReviews(intTestProject, intTestPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending, "review must be fully processed after two polls")
	assert.True(t, store.IsReviewProcessed(intTestProject, intTestPlanFile, 30))
}

// TestPRMonitorInt_MalformedPRURL_NoAction verifies that a task with a
// malformed PRURL does not panic, produces no dispatched actions, and leaves no
// pr_reviews rows. No real gh calls are needed because ExtractPRNumber fails
// before any CLI invocation.
func TestPRMonitorInt_MalformedPRURL_NoAction(t *testing.T) {
	store := taskstore.NewTestStore(t)
	require.NoError(t, store.Create(intTestProject, taskstore.TaskEntry{
		Filename: intTestPlanFile,
		Status:   taskstore.StatusReviewing,
		Branch:   intTestBranch,
		PRURL:    "https://not-a-pr-url/foo/bar/issues/3",
	}))

	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	rm := NewRepoManager()
	rm.repos = []RepoEntry{{Path: repoDir, Project: intTestProject, Store: store}}
	dispatch := func(ctx context.Context, e RepoEntry, action loop.Action) error {
		t.Fatalf("dispatch must not be called for malformed PRURL")
		return nil
	}
	cfg := PRMonitorConfig{Enabled: true, PollInterval: time.Second, Reactions: []string{"eyes"}}
	monitor := NewPRMonitor(cfg, 5, rm, broadcaster, slog.Default(), dispatch)

	// Use a mock that fatals on any gh call to prove no gh calls happen.
	fatalMock := &cmd_test.MockCmdExec{
		OutputFunc: func(c *exec.Cmd) ([]byte, error) {
			t.Fatalf("gh Output must not be called for malformed PRURL (args: %v)", c.Args)
			return nil, nil
		},
		RunFunc: func(c *exec.Cmd) error {
			t.Fatalf("gh Run must not be called for malformed PRURL (args: %v)", c.Args)
			return nil
		},
	}
	t.Cleanup(gitpkg.SetGHExec(fatalMock))

	monitor.pollOnce(context.Background())

	pending, err := store.ListPendingReviews(intTestProject, intTestPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

// TestPRMonitorInt_GHAuthFailure_WarnsAndNoDispatch verifies that when the gh
// CLI returns an auth failure (HTTP 401), PRMonitor logs a warning and does not
// dispatch any action. The warning must appear in the log even after a second
// poll — no silencing of repeated errors.
func TestPRMonitorInt_GHAuthFailure_WarnsAndNoDispatch(t *testing.T) {
	store := newIntTestStore(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var actions []dispatchedAction
	rm := NewRepoManager()
	rm.repos = []RepoEntry{{Path: repoDir, Project: intTestProject, Store: store}}
	dispatch := func(ctx context.Context, e RepoEntry, action loop.Action) error {
		actions = append(actions, dispatchedAction{repo: e.Path, action: action})
		return nil
	}
	cfg := PRMonitorConfig{Enabled: true, PollInterval: time.Second, Reactions: []string{"eyes"}}
	monitor := NewPRMonitor(cfg, 5, rm, broadcaster, logger, dispatch)

	// gh call sequence per poll:
	// 1 PROpen: view → OK, pulls/42 → OK (open)
	// 2 ListPRReviews: view → OK, reviews → 401 error
	// (duplicated for two poll cycles)
	mock := seqMock(t,
		[]ghOutputFn{
			// Poll 1
			staticOut(repoViewJSON), staticOut(prOpenJSON(true)),
			staticOut(repoViewJSON), errOut("HTTP 401 Unauthorized: authentication required"),
			// Poll 2
			staticOut(repoViewJSON), staticOut(prOpenJSON(true)),
			staticOut(repoViewJSON), errOut("HTTP 401 Unauthorized: authentication required"),
		},
		[]ghRunFn{},
	)
	t.Cleanup(gitpkg.SetGHExec(mock))

	monitor.pollOnce(context.Background())
	monitor.pollOnce(context.Background())

	// No actions must have been dispatched.
	assert.Empty(t, actions, "auth failure must not dispatch any action")

	// At least one WARN-level log line must be present.
	logOutput := logBuf.String()
	assert.True(t, strings.Contains(logOutput, "WARN") || strings.Contains(logOutput, "warn"),
		"expected at least one WARN log entry for auth failure; got:\n%s", logOutput)

	// No rows persisted.
	pending, err := store.ListPendingReviews(intTestProject, intTestPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

// TestPRMonitorInt_MultipleReviews_OnlyChangesRequestedDispatches verifies
// that when a poll returns APPROVED, CHANGES_REQUESTED, and COMMENTED reviews,
// only the CHANGES_REQUESTED review triggers a SpawnFixerAction. All three
// reviews must be recorded in the store.
func TestPRMonitorInt_MultipleReviews_OnlyChangesRequestedDispatches(t *testing.T) {
	store := newIntTestStore(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := newIntTestMonitor(t, store, repoDir, &actions, broadcaster, 5)

	multi := reviewsJSON([]rawReview{
		makeRawReview(40, "APPROVED", "looks good", "eve"),
		makeRawReview(41, "CHANGES_REQUESTED", "fix the test", "mallory"),
		// COMMENTED with empty body: shouldTriggerFixer returns false (body is blank),
		// so no reaction or dispatch is expected for review 42.
		makeRawReview(42, "COMMENTED", "", "alice"),
	})

	// PROpen + reviews fetched once.
	// ListReviewComments + AddReviewReaction only for review 41 (CHANGES_REQUESTED).
	// Reviews 40 (APPROVED) and 42 (COMMENTED with empty body) must NOT trigger
	// reactions or dispatch.
	mock := seqMock(t,
		[]ghOutputFn{
			staticOut(repoViewJSON), staticOut(prOpenJSON(true)),
			staticOut(repoViewJSON), staticOut(multi),
			// ListReviewComments for review 41
			staticOut(repoViewJSON), staticOut(commentsJSON(301)),
			// AddReviewReaction for comment 301
			staticOut(repoViewJSON),
		},
		[]ghRunFn{runOK}, // one reaction POST for CHANGES_REQUESTED only
	)
	t.Cleanup(gitpkg.SetGHExec(mock))

	monitor.pollOnce(context.Background())

	// Only review 41 (CHANGES_REQUESTED) should dispatch a SpawnFixerAction.
	require.Len(t, actions, 1, "only CHANGES_REQUESTED must dispatch fixer")
	_, ok := actions[0].action.(loop.SpawnFixerAction)
	assert.True(t, ok, "expected SpawnFixerAction for CHANGES_REQUESTED review 41")

	// All three reviews must be persisted in the store.
	assert.True(t, store.IsReviewProcessed(intTestProject, intTestPlanFile, 40), "APPROVED review 40 must be recorded")
	assert.True(t, store.IsReviewProcessed(intTestProject, intTestPlanFile, 41), "CHANGES_REQUESTED review 41 must be recorded")
	assert.True(t, store.IsReviewProcessed(intTestProject, intTestPlanFile, 42), "COMMENTED review 42 must be recorded")
}

// TestPRMonitorInt_ReviewCycleLimit_DispatchesLimitAction verifies that when a
// task's ReviewCycle has reached maxReviewFixCycles, PRMonitor dispatches
// ReviewCycleLimitAction instead of SpawnFixerAction.
//
// NOTE: The current PRMonitor implementation may skip the task entirely rather
// than dispatching ReviewCycleLimitAction. This test documents the intended
// spec behavior (ReviewCycleLimitAction must be dispatched so the daemon can
// notify the user and emit the review_cycle_limit SSE event).
func TestPRMonitorInt_ReviewCycleLimit_DispatchesLimitAction(t *testing.T) {
	const maxCycles = 3
	store := newIntTestStore(t)

	// Set the task's ReviewCycle to the limit so the next review would exceed it.
	task, err := store.Get(intTestProject, intTestPlanFile)
	require.NoError(t, err)
	task.ReviewCycle = maxCycles
	require.NoError(t, store.Update(intTestProject, intTestPlanFile, task))

	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := newIntTestMonitor(t, store, repoDir, &actions, broadcaster, maxCycles)

	// gh call sequence:
	// 1 PROpen: view + pulls/42
	// 2 ListPRReviews: view + reviews
	// 3 ListReviewComments (review 50): view + reviews/50/comments
	//   (reaction step runs before cycle-limit check; no Run call because
	//    ReviewCycleLimitAction is dispatched instead of SpawnFixerAction)
	mock := seqMock(t,
		[]ghOutputFn{
			staticOut(repoViewJSON), staticOut(prOpenJSON(true)),
			staticOut(repoViewJSON), staticOut(reviewsJSON([]rawReview{
				makeRawReview(50, "CHANGES_REQUESTED", "more changes needed", "frank"),
			})),
			staticOut(repoViewJSON), staticOut(commentsJSON(501)),
			staticOut(repoViewJSON), // AddReviewReaction: view
		},
		[]ghRunFn{runOK}, // reaction POST
	)
	t.Cleanup(gitpkg.SetGHExec(mock))

	monitor.pollOnce(context.Background())

	// Expect ReviewCycleLimitAction, not SpawnFixerAction.
	require.Len(t, actions, 1, "expected exactly one action when cycle limit is hit")
	la, ok := actions[0].action.(loop.ReviewCycleLimitAction)
	require.True(t, ok, "expected ReviewCycleLimitAction when ReviewCycle >= maxCycles, got %T", actions[0].action)
	assert.Equal(t, intTestPlanFile, la.PlanFile)
	assert.GreaterOrEqual(t, la.Cycle, maxCycles)
}

// TestPRMonitorInt_ClosedPR_SkipsPolling verifies that when PROpen returns
// false (closed or merged PR), PRMonitor skips all review fetching and does not
// dispatch any action or record any rows.
func TestPRMonitorInt_ClosedPR_SkipsPolling(t *testing.T) {
	store := newIntTestStore(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	var actions []dispatchedAction
	monitor := newIntTestMonitor(t, store, repoDir, &actions, broadcaster, 5)

	// PROpen returns closed — ListPRReviews must NOT be called.
	mock := seqMock(t,
		[]ghOutputFn{
			staticOut(repoViewJSON),
			staticOut(prOpenJSON(false)), // PR is closed/merged
			// No further output calls expected.
		},
		[]ghRunFn{},
	)
	t.Cleanup(gitpkg.SetGHExec(mock))

	monitor.pollOnce(context.Background())

	assert.Empty(t, actions, "closed PR must not dispatch any action")

	savedTask, err := store.Get(intTestProject, intTestPlanFile)
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusReviewing, savedTask.Status)

	pending, err := store.ListPendingReviews(intTestProject, intTestPlanFile)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

// ---------------------------------------------------------------------------
// Run / cancellation test
// ---------------------------------------------------------------------------

// TestPRMonitorInt_Run_StopsOnContextCancellation verifies that Run exits
// cleanly when the context is cancelled, without blocking or leaking goroutines.
func TestPRMonitorInt_Run_StopsOnContextCancellation(t *testing.T) {
	store := newIntTestStore(t)
	repoDir := t.TempDir()
	broadcaster := api.NewEventBroadcaster()
	defer broadcaster.Close()

	rm := NewRepoManager()
	rm.repos = []RepoEntry{{Path: repoDir, Project: intTestProject, Store: store}}
	dispatch := func(ctx context.Context, e RepoEntry, action loop.Action) error {
		return nil
	}

	// Short poll interval; each tick returns a closed PR so no reaction/dispatch
	// calls happen, allowing many ticks with minimal mock complexity.
	cfg := PRMonitorConfig{Enabled: true, PollInterval: 40 * time.Millisecond, Reactions: []string{"eyes"}}
	monitor := NewPRMonitor(cfg, 5, rm, broadcaster, slog.Default(), dispatch)

	// Provide plenty of responses for multiple ticks (each tick: view + pulls/42).
	outputs := make([]ghOutputFn, 0, 40)
	for i := 0; i < 20; i++ {
		outputs = append(outputs, staticOut(repoViewJSON), staticOut(prOpenJSON(false)))
	}
	oIdx := 0
	mock := &cmd_test.MockCmdExec{
		OutputFunc: func(c *exec.Cmd) ([]byte, error) {
			if oIdx < len(outputs) {
				fn := outputs[oIdx]
				oIdx++
				return fn(c)
			}
			// Fallback: return closed-PR for any extra calls.
			if strings.Contains(strings.Join(c.Args, " "), "nameWithOwner") {
				return []byte(repoViewJSON), nil
			}
			return []byte(prOpenJSON(false)), nil
		},
		RunFunc: func(c *exec.Cmd) error { return nil },
	}
	t.Cleanup(gitpkg.SetGHExec(mock))

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- monitor.Run(ctx)
	}()

	// Allow a few ticks.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case runErr := <-done:
		assert.NoError(t, runErr, "Run must return nil on clean cancellation")
	case <-time.After(2 * time.Second):
		t.Fatal("PRMonitor.Run did not return within 2s after context cancellation")
	}
}
