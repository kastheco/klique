package taskfsm

import (
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestFSM creates a TaskStateMachine backed by an in-memory SQLite store.
func newTestFSM(t *testing.T) (*TaskStateMachine, taskstore.Store) {
	t.Helper()
	store := taskstore.NewTestSQLiteStore(t)
	fsm := New(store, "test-proj", t.TempDir())
	return fsm, store
}

func TestTransition_ValidTransitions(t *testing.T) {
	cases := []struct {
		from  Status
		event Event
		to    Status
	}{
		{StatusReady, PlanStart, StatusPlanning},
		{StatusPlanning, PlanStart, StatusPlanning}, // restart after crash/interrupt
		{StatusPlanning, PlannerFinished, StatusReady},
		{StatusReady, ImplementStart, StatusImplementing},
		{StatusImplementing, ImplementFinished, StatusReviewing},
		{StatusReviewing, ReviewApproved, StatusDone},
		{StatusReviewing, ReviewChangesRequested, StatusImplementing},
		{StatusDone, StartOver, StatusPlanning},
		{StatusDone, Cancel, StatusCancelled},
		{StatusReady, Cancel, StatusCancelled},
		{StatusPlanning, Cancel, StatusCancelled},
		{StatusImplementing, Cancel, StatusCancelled},
		{StatusReviewing, Cancel, StatusCancelled},
		{StatusCancelled, Reopen, StatusPlanning},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_"+string(tc.event), func(t *testing.T) {
			result, err := ApplyTransition(tc.from, tc.event)
			require.NoError(t, err)
			assert.Equal(t, tc.to, result)
		})
	}
}

func TestTransition_InvalidTransitions(t *testing.T) {
	cases := []struct {
		from  Status
		event Event
	}{
		{StatusReady, PlannerFinished},    // not planning
		{StatusReady, ImplementFinished},  // not implementing
		{StatusReady, ReviewApproved},     // not reviewing
		{StatusPlanning, ImplementStart},  // must go through ready
		{StatusImplementing, PlanStart},   // can't go backwards
		{StatusDone, PlanStart},           // terminal
		{StatusDone, ImplementFinished},   // terminal
		{StatusCancelled, ImplementStart}, // must reopen first
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"_"+string(tc.event), func(t *testing.T) {
			_, err := ApplyTransition(tc.from, tc.event)
			assert.Error(t, err)
		})
	}
}

func TestIsUserOnly(t *testing.T) {
	assert.True(t, StartOver.IsUserOnly())
	assert.True(t, Cancel.IsUserOnly())
	assert.True(t, Reopen.IsUserOnly())
	assert.False(t, PlannerFinished.IsUserOnly())
	assert.False(t, ReviewApproved.IsUserOnly())
}

func TestTaskStateMachine_TransitionWritesToStore(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	dir := t.TempDir()

	// Seed with a ready plan
	ps, err := taskstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	require.NoError(t, ps.Register("test", "test plan", "plan/test", time.Now()))

	fsm := New(store, "test-proj", dir)
	err = fsm.Transition("test", PlanStart)
	require.NoError(t, err)

	// Re-read from store to verify persistence
	reloaded, err := taskstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry("test")
	require.True(t, ok)
	assert.Equal(t, "planning", string(entry.Status))
}

func TestTaskStateMachine_RejectsInvalidTransition(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	dir := t.TempDir()

	ps, err := taskstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	require.NoError(t, ps.Register("test", "test plan", "plan/test", time.Now()))

	fsm := New(store, "test-proj", dir)
	err = fsm.Transition("test", ImplementFinished) // ready → implement_finished is invalid
	assert.Error(t, err)

	// Status must remain unchanged in store
	reloaded, err := taskstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry("test")
	require.True(t, ok)
	assert.Equal(t, "ready", string(entry.Status))
}

func TestTaskStateMachine_MissingPlanReturnsError(t *testing.T) {
	fsm, _ := newTestFSM(t)
	err := fsm.Transition("nonexistent", PlanStart)
	assert.Error(t, err)
}

func TestFSM_TransitionWithStore(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	err := store.Create("test-project", taskstore.TaskEntry{
		Filename: "test", Status: "ready",
	})
	require.NoError(t, err)

	fsm := New(store, "test-project", t.TempDir())
	require.NoError(t, fsm.Transition("test", PlanStart))

	// Verify the store was updated
	entry, err := store.Get("test-project", "test")
	require.NoError(t, err)
	assert.Equal(t, "planning", string(entry.Status))
}

func TestFSM_TransitionRecordsPhaseTimestamp(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	dir := t.TempDir()

	ps, err := taskstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	require.NoError(t, ps.Register("test", "test plan", "plan/test", time.Now()))

	fsm := New(store, "test-proj", dir)
	require.NoError(t, fsm.Transition("test", PlanStart))
	require.NoError(t, fsm.Transition("test", PlannerFinished))
	require.NoError(t, fsm.Transition("test", ImplementStart))
	require.NoError(t, fsm.Transition("test", ImplementFinished))
	require.NoError(t, fsm.Transition("test", ReviewApproved))

	entry, err := store.Get("test-proj", "test")
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusDone, entry.Status)
	assert.False(t, entry.PlanningAt.IsZero())
	assert.False(t, entry.ImplementingAt.IsZero())
	assert.False(t, entry.ReviewingAt.IsZero())
	assert.False(t, entry.DoneAt.IsZero())
}

func TestFSM_TransitionSkipsTimestampForNonPhaseStatuses(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	dir := t.TempDir()

	ps, err := taskstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	require.NoError(t, ps.Register("test", "test plan", "plan/test", time.Now()))

	fsm := New(store, "test-proj", dir)
	require.NoError(t, fsm.Transition("test", Cancel))

	entry, err := store.Get("test-proj", "test")
	require.NoError(t, err)
	assert.Equal(t, taskstore.StatusCancelled, entry.Status)
	assert.True(t, entry.PlanningAt.IsZero())
	assert.True(t, entry.ImplementingAt.IsZero())
	assert.True(t, entry.ReviewingAt.IsZero())
	assert.True(t, entry.DoneAt.IsZero())
}
