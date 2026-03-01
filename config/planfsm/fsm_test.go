package planfsm

import (
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestFSM creates a PlanStateMachine backed by an in-memory SQLite store.
func newTestFSM(t *testing.T) (*PlanStateMachine, planstore.Store) {
	t.Helper()
	store := planstore.NewTestSQLiteStore(t)
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

func TestPlanStateMachine_TransitionWritesToStore(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	dir := t.TempDir()

	// Seed with a ready plan
	ps, err := planstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	require.NoError(t, ps.Register("test.md", "test plan", "plan/test", time.Now()))

	fsm := New(store, "test-proj", dir)
	err = fsm.Transition("test.md", PlanStart)
	require.NoError(t, err)

	// Re-read from store to verify persistence
	reloaded, err := planstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry("test.md")
	require.True(t, ok)
	assert.Equal(t, "planning", string(entry.Status))
}

func TestPlanStateMachine_RejectsInvalidTransition(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	dir := t.TempDir()

	ps, err := planstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	require.NoError(t, ps.Register("test.md", "test plan", "plan/test", time.Now()))

	fsm := New(store, "test-proj", dir)
	err = fsm.Transition("test.md", ImplementFinished) // ready → implement_finished is invalid
	assert.Error(t, err)

	// Status must remain unchanged in store
	reloaded, err := planstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	entry, ok := reloaded.Entry("test.md")
	require.True(t, ok)
	assert.Equal(t, "ready", string(entry.Status))
}

func TestPlanStateMachine_MissingPlanReturnsError(t *testing.T) {
	fsm, _ := newTestFSM(t)
	err := fsm.Transition("nonexistent.md", PlanStart)
	assert.Error(t, err)
}

func TestFSM_TransitionWithStore(t *testing.T) {
	store := planstore.NewTestSQLiteStore(t)
	err := store.Create("test-project", planstore.PlanEntry{
		Filename: "test.md", Status: "ready",
	})
	require.NoError(t, err)

	fsm := New(store, "test-project", t.TempDir())
	require.NoError(t, fsm.Transition("test.md", PlanStart))

	// Verify the store was updated
	entry, err := store.Get("test-project", "test.md")
	require.NoError(t, err)
	assert.Equal(t, "planning", string(entry.Status))
}
