package app

import (
	"fmt"
	"sync"
	"testing"

	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/require"
)

// testFSM wraps PlanStateMachine with a convenience method for string-based events.
type testFSM struct {
	*planfsm.PlanStateMachine
}

var (
	testStoreMu sync.Mutex
	testStores  = make(map[string]planstore.Store)
)

func storeForDir(t testing.TB, dir string) planstore.Store {
	t.Helper()
	testStoreMu.Lock()
	defer testStoreMu.Unlock()
	if store, ok := testStores[dir]; ok {
		return store
	}
	store := planstore.NewTestSQLiteStore(t)
	testStores[dir] = store
	t.Cleanup(func() {
		testStoreMu.Lock()
		delete(testStores, dir)
		testStoreMu.Unlock()
	})
	return store
}

// newTestStore creates an in-memory SQLite store for use in tests.
func newTestStore(t testing.TB) planstore.Store {
	t.Helper()
	return planstore.NewTestSQLiteStore(t)
}

// newTestPlanState creates a PlanState backed by an in-memory SQLite store.
// This replaces planstate.Load(dir) in tests after the JSON fallback was removed.
func newTestPlanState(t testing.TB, dir string) (*planstate.PlanState, error) {
	t.Helper()
	store := storeForDir(t, dir)
	return planstate.Load(store, "test", dir)
}

// newTestPlanStateWithStore creates a PlanState backed by the given store.
// Use this when the store must be shared with an FSM or other component.
func newTestPlanStateWithStore(t testing.TB, store planstore.Store, dir string) (*planstate.PlanState, error) {
	t.Helper()
	return planstate.Load(store, "test", dir)
}

func newPlanFSMForTest(t testing.TB, dir string) *planfsm.PlanStateMachine {
	t.Helper()
	store := storeForDir(t, dir)
	return planfsm.New(store, "test", dir)
}

// newPlanFSMForTestWithStore creates a PlanStateMachine backed by the given store.
// Use this when the store must be shared with a PlanState.
func newPlanFSMForTestWithStore(t testing.TB, store planstore.Store, dir string) *planfsm.PlanStateMachine {
	t.Helper()
	return planfsm.New(store, "test", dir)
}

func newFSMForTest(t testing.TB, dir string) *testFSM {
	t.Helper()
	return &testFSM{newPlanFSMForTest(t, dir)}
}

// newSharedStoreForTest creates a shared in-memory SQLite store and returns
// a PlanState and FSM that both use it. Use this when tests need the FSM and
// PlanState to share the same backing store.
func newSharedStoreForTest(t testing.TB, dir string) (planstore.Store, *planstate.PlanState, *planfsm.PlanStateMachine) {
	t.Helper()
	store := newTestStore(t)
	ps, err := planstate.Load(store, "test", dir)
	if err != nil {
		t.Fatalf("newSharedStoreForTest: load plan state: %v", err)
	}
	fsm := planfsm.New(store, "test", dir)
	return store, ps, fsm
}

// seedPlanStatus directly sets a plan's status in the PlanState for test setup,
// bypassing the FSM. Persists to the store immediately.
func seedPlanStatus(t *testing.T, ps *planstate.PlanState, planFile string, status planstate.Status) {
	t.Helper()
	require.NoError(t, ps.ForceSetStatus(planFile, status))
}

// TransitionByName applies an event by its string name (for table-driven tests).
func (f *testFSM) TransitionByName(planFile, eventName string) error {
	eventMap := map[string]planfsm.Event{
		"plan_start":         planfsm.PlanStart,
		"planner_finished":   planfsm.PlannerFinished,
		"implement_start":    planfsm.ImplementStart,
		"implement_finished": planfsm.ImplementFinished,
		"review_approved":    planfsm.ReviewApproved,
		"review_changes":     planfsm.ReviewChangesRequested,
		"start_over":         planfsm.StartOver,
		"cancel":             planfsm.Cancel,
		"reopen":             planfsm.Reopen,
	}
	ev, ok := eventMap[eventName]
	if !ok {
		return fmt.Errorf("unknown event name: %q", eventName)
	}
	return f.Transition(planFile, ev)
}
