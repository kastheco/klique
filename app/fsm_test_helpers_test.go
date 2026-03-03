package app

import (
	"fmt"
	"sync"
	"testing"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/require"
)

// testFSM wraps PlanStateMachine with a convenience method for string-based events.
type testFSM struct {
	*taskfsm.TaskStateMachine
}

var (
	testStoreMu sync.Mutex
	testStores  = make(map[string]taskstore.Store)
)

func storeForDir(t testing.TB, dir string) taskstore.Store {
	t.Helper()
	testStoreMu.Lock()
	defer testStoreMu.Unlock()
	if store, ok := testStores[dir]; ok {
		return store
	}
	store := taskstore.NewTestSQLiteStore(t)
	testStores[dir] = store
	t.Cleanup(func() {
		testStoreMu.Lock()
		delete(testStores, dir)
		testStoreMu.Unlock()
	})
	return store
}

// newTestStore creates an in-memory SQLite store for use in tests.
func newTestStore(t testing.TB) taskstore.Store {
	t.Helper()
	return taskstore.NewTestSQLiteStore(t)
}

// newTestPlanState creates a PlanState backed by an in-memory SQLite store.
// This replaces taskstate.Load(dir) in tests after the JSON fallback was removed.
func newTestPlanState(t testing.TB, dir string) (*taskstate.TaskState, error) {
	t.Helper()
	store := storeForDir(t, dir)
	return taskstate.Load(store, "test", dir)
}

// newTestPlanStateWithStore creates a PlanState backed by the given store.
// Use this when the store must be shared with an FSM or other component.
func newTestPlanStateWithStore(t testing.TB, store taskstore.Store, dir string) (*taskstate.TaskState, error) {
	t.Helper()
	return taskstate.Load(store, "test", dir)
}

func newPlanFSMForTest(t testing.TB, dir string) *taskfsm.TaskStateMachine {
	t.Helper()
	store := storeForDir(t, dir)
	return taskfsm.New(store, "test", dir)
}

// newPlanFSMForTestWithStore creates a PlanStateMachine backed by the given store.
// Use this when the store must be shared with a PlanState.
func newPlanFSMForTestWithStore(t testing.TB, store taskstore.Store, dir string) *taskfsm.TaskStateMachine {
	t.Helper()
	return taskfsm.New(store, "test", dir)
}

func newFSMForTest(t testing.TB, dir string) *testFSM {
	t.Helper()
	return &testFSM{newPlanFSMForTest(t, dir)}
}

// newSharedStoreForTest creates a shared in-memory SQLite store and returns
// a PlanState and FSM that both use it. Use this when tests need the FSM and
// PlanState to share the same backing store.
func newSharedStoreForTest(t testing.TB, dir string) (taskstore.Store, *taskstate.TaskState, *taskfsm.TaskStateMachine) {
	t.Helper()
	store := newTestStore(t)
	ps, err := taskstate.Load(store, "test", dir)
	if err != nil {
		t.Fatalf("newSharedStoreForTest: load plan state: %v", err)
	}
	fsm := taskfsm.New(store, "test", dir)
	return store, ps, fsm
}

// seedPlanStatus directly sets a plan's status in the PlanState for test setup,
// bypassing the FSM. Persists to the store immediately.
func seedPlanStatus(t *testing.T, ps *taskstate.TaskState, planFile string, status taskstate.Status) {
	t.Helper()
	require.NoError(t, ps.ForceSetStatus(planFile, status))
}

// TransitionByName applies an event by its string name (for table-driven tests).
func (f *testFSM) TransitionByName(planFile, eventName string) error {
	eventMap := map[string]taskfsm.Event{
		"plan_start":         taskfsm.PlanStart,
		"planner_finished":   taskfsm.PlannerFinished,
		"implement_start":    taskfsm.ImplementStart,
		"implement_finished": taskfsm.ImplementFinished,
		"review_approved":    taskfsm.ReviewApproved,
		"review_changes":     taskfsm.ReviewChangesRequested,
		"start_over":         taskfsm.StartOver,
		"cancel":             taskfsm.Cancel,
		"reopen":             taskfsm.Reopen,
	}
	ev, ok := eventMap[eventName]
	if !ok {
		return fmt.Errorf("unknown event name: %q", eventName)
	}
	return f.Transition(planFile, ev)
}
