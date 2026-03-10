package taskfsm

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingHook stores delivered events for assertions.
type recordingHook struct {
	mu     sync.Mutex
	events []TransitionEvent
}

func (r *recordingHook) Name() string { return "recording" }
func (r *recordingHook) Run(_ context.Context, ev TransitionEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return nil
}
func (r *recordingHook) Events() []TransitionEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]TransitionEvent, len(r.events))
	copy(cp, r.events)
	return cp
}

type failingHook struct{}

func (f *failingHook) Name() string { return "failing" }
func (f *failingHook) Run(_ context.Context, _ TransitionEvent) error {
	return fmt.Errorf("intentional failure")
}

func newTestFSMWithStore(t *testing.T) (*TaskStateMachine, taskstore.Store, string) {
	t.Helper()
	store := taskstore.NewTestSQLiteStore(t)
	dir := t.TempDir()

	ps, err := taskstate.Load(store, "test-proj", dir)
	require.NoError(t, err)
	require.NoError(t, ps.Register("test", "test plan", "plan/test", time.Now()))

	fsm := New(store, "test-proj", dir)
	return fsm, store, dir
}

func TestHookRegistry_FiresOnTransition(t *testing.T) {
	fsm, _, _ := newTestFSMWithStore(t)

	rec := &recordingHook{}
	registry := NewHookRegistry()
	registry.Add(rec, nil)
	fsm.SetHooks(registry)

	err := fsm.Transition("test", PlanStart)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(rec.Events()) > 0
	}, 2*time.Second, 10*time.Millisecond)

	events := rec.Events()
	require.Len(t, events, 1)
	ev := events[0]
	assert.Equal(t, "test", ev.PlanFile)
	assert.Equal(t, StatusReady, ev.FromStatus)
	assert.Equal(t, StatusPlanning, ev.ToStatus)
	assert.Equal(t, PlanStart, ev.Event)
	assert.Equal(t, "test-proj", ev.Project)
	assert.False(t, ev.Timestamp.IsZero())
}

func TestHookRegistry_FiltersEvents(t *testing.T) {
	fsm, _, _ := newTestFSMWithStore(t)

	rec := &recordingHook{}
	registry := NewHookRegistry()
	// Only listen for ImplementStart, not PlanStart
	registry.Add(rec, []Event{ImplementStart})
	fsm.SetHooks(registry)

	err := fsm.Transition("test", PlanStart)
	require.NoError(t, err)

	// Give hooks time to fire if they were going to
	time.Sleep(100 * time.Millisecond)

	assert.Empty(t, rec.Events(), "hook should not fire for filtered-out event")
}

func TestHookRegistry_ErrorDoesNotBlock(t *testing.T) {
	fsm, _, _ := newTestFSMWithStore(t)

	failing := &failingHook{}
	rec := &recordingHook{}
	registry := NewHookRegistry()
	registry.Add(failing, nil)
	registry.Add(rec, nil)
	fsm.SetHooks(registry)

	err := fsm.Transition("test", PlanStart)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(rec.Events()) > 0
	}, 2*time.Second, 10*time.Millisecond)

	assert.Len(t, rec.Events(), 1, "recording hook should receive event despite failing hook")
}
