package loop

import (
	"testing"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessor_ProcessFSMSignals_ImplementFinished(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusImplementing,
		Branch:   "plan/my-plan",
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test", AutoReviewFix: true})
	signals := []taskfsm.Signal{
		{Event: taskfsm.ImplementFinished, TaskFile: "my-plan.md"},
	}

	actions := p.ProcessFSMSignals(signals)
	require.Len(t, actions, 1)
	spawnReviewer, ok := actions[0].(SpawnReviewerAction)
	require.True(t, ok, "expected SpawnReviewerAction, got %T", actions[0])
	assert.Equal(t, "my-plan.md", spawnReviewer.PlanFile)
}

func TestProcessor_ProcessFSMSignals_ReviewApproved(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "plan/my-plan",
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test", AutoReviewFix: true})
	signals := []taskfsm.Signal{
		{Event: taskfsm.ReviewApproved, TaskFile: "my-plan.md", Body: "LGTM"},
	}

	actions := p.ProcessFSMSignals(signals)

	// ReviewApprovedAction must always be emitted (carries side-effect obligation).
	var foundApproved, foundPR bool
	for _, a := range actions {
		if ra, ok := a.(ReviewApprovedAction); ok {
			assert.Equal(t, "my-plan.md", ra.PlanFile)
			assert.Equal(t, "LGTM", ra.ReviewBody)
			foundApproved = true
		}
		if pr, ok := a.(CreatePRAction); ok {
			assert.Equal(t, "my-plan.md", pr.PlanFile)
			foundPR = true
		}
	}
	assert.True(t, foundApproved, "expected ReviewApprovedAction")
	// Plan has a branch and no PR URL so CreatePRAction should also be emitted.
	assert.True(t, foundPR, "expected CreatePRAction when plan has branch and no PR yet")
}

func TestProcessor_ProcessFSMSignals_ReviewApproved_NoBranch(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "", // no branch — PR not eligible
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test", AutoReviewFix: true})
	signals := []taskfsm.Signal{
		{Event: taskfsm.ReviewApproved, TaskFile: "my-plan.md", Body: "LGTM"},
	}

	actions := p.ProcessFSMSignals(signals)

	// ReviewApprovedAction must be emitted even when no PR will be created.
	var foundApproved, foundPR bool
	for _, a := range actions {
		if _, ok := a.(ReviewApprovedAction); ok {
			foundApproved = true
		}
		if _, ok := a.(CreatePRAction); ok {
			foundPR = true
		}
	}
	assert.True(t, foundApproved, "expected ReviewApprovedAction regardless of branch")
	assert.False(t, foundPR, "expected no CreatePRAction when plan has no branch")
}

func TestProcessor_ProcessFSMSignals_ReviewChangesRequested(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "plan/my-plan",
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test", AutoReviewFix: true})
	feedback := "fix the error handling in handler.go"
	signals := []taskfsm.Signal{
		{Event: taskfsm.ReviewChangesRequested, TaskFile: "my-plan.md", Body: feedback},
	}

	actions := p.ProcessFSMSignals(signals)
	var foundReviewChanges, foundFixer, foundIncrement bool
	for _, a := range actions {
		if rc, ok := a.(ReviewChangesAction); ok {
			assert.Equal(t, feedback, rc.Feedback)
			foundReviewChanges = true
		}
		if sf, ok := a.(SpawnFixerAction); ok {
			assert.Equal(t, feedback, sf.Feedback)
			foundFixer = true
		}
		if _, ok := a.(IncrementReviewCycleAction); ok {
			foundIncrement = true
		}
	}
	assert.True(t, foundReviewChanges, "expected ReviewChangesAction")
	assert.True(t, foundFixer, "expected SpawnFixerAction")
	assert.True(t, foundIncrement, "expected IncrementReviewCycleAction")
}

func TestProcessor_ProcessFSMSignals_InvalidReviewChangesRequested_HasNoActions(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusImplementing,
		Branch:   "plan/my-plan",
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test", AutoReviewFix: true})
	actions := p.ProcessFSMSignals([]taskfsm.Signal{{
		Event:    taskfsm.ReviewChangesRequested,
		TaskFile: "my-plan.md",
		Body:     "stale feedback",
	}})

	assert.Empty(t, actions, "invalid review_changes_requested should not emit side-effect actions")
}

func TestProcessor_ProcessFSMSignals_ReviewChangesRequested_AutoReviewFixDisabled(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "plan/my-plan",
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test"})
	signals := []taskfsm.Signal{{Event: taskfsm.ReviewChangesRequested, TaskFile: "my-plan.md", Body: "fix this"}}

	actions := p.ProcessFSMSignals(signals)
	require.Len(t, actions, 1)
	rc, ok := actions[0].(ReviewChangesAction)
	require.True(t, ok, "expected ReviewChangesAction when auto review-fix is disabled")
	assert.Equal(t, "my-plan.md", rc.PlanFile)
	assert.Equal(t, "fix this", rc.Feedback)
}

func TestProcessor_ProcessFSMSignals_PlannerFinished(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusPlanning,
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test"})
	signals := []taskfsm.Signal{
		{Event: taskfsm.PlannerFinished, TaskFile: "my-plan.md"},
	}

	actions := p.ProcessFSMSignals(signals)
	var found bool
	for _, a := range actions {
		if pc, ok := a.(PlannerCompleteAction); ok {
			assert.Equal(t, "my-plan.md", pc.PlanFile)
			found = true
		}
	}
	assert.True(t, found, "expected PlannerCompleteAction")
}

func TestProcessor_ProcessFSMSignals_SkipIfWaveOrchestratorActive(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusImplementing,
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test"})
	p.SetWaveOrchestratorActive("my-plan.md", true)

	signals := []taskfsm.Signal{
		{Event: taskfsm.ImplementFinished, TaskFile: "my-plan.md"},
	}
	actions := p.ProcessFSMSignals(signals)
	assert.Empty(t, actions, "implement-finished should be suppressed when wave orchestrator active")
}

func TestProcessor_ProcessTaskSignals(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusImplementing,
		Branch:   "plan/my-plan",
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test"})
	p.RegisterOrchestrator("my-plan.md", 1, []int{1, 2})

	taskSignals := []taskfsm.TaskSignal{
		{TaskFile: "my-plan.md", TaskNumber: 1, WaveNumber: 1},
	}

	actions := p.ProcessTaskSignals(taskSignals)
	var found bool
	for _, a := range actions {
		if tc, ok := a.(TaskCompleteAction); ok {
			assert.Equal(t, 1, tc.TaskNumber)
			found = true
		}
	}
	assert.True(t, found, "expected TaskCompleteAction")
}

func TestProcessor_ProcessTaskSignals_RestoresOrchestratorFromStore(t *testing.T) {
	store := taskstore.NewTestStore(t)
	require.NoError(t, store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusImplementing,
		Branch:   "plan/my-plan",
	}))
	require.NoError(t, store.SetContent("test", "my-plan.md", "# Plan\n\n**Goal:** test\n\n**Architecture:** test\n\n**Tech Stack:** go\n\n**Size:** Small\n\n---\n\n## Wave 1\n\n### Task 1: First\n\nDo the first thing.\n\n### Task 2: Second\n\nDo the second thing."))
	require.NoError(t, store.SetSubtasks("test", "my-plan.md", []taskstore.SubtaskEntry{
		{TaskNumber: 1, Title: "First", Status: taskstore.SubtaskStatusComplete},
		{TaskNumber: 2, Title: "Second", Status: taskstore.SubtaskStatusRunning},
	}))

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test"})
	actions := p.ProcessTaskSignals([]taskfsm.TaskSignal{{
		TaskFile:   "my-plan.md",
		TaskNumber: 2,
		WaveNumber: 1,
	}})

	require.Len(t, actions, 1)
	taskAction, ok := actions[0].(TaskCompleteAction)
	require.True(t, ok)
	assert.Equal(t, 2, taskAction.TaskNumber)

	orch := p.WaveOrchestrator("my-plan.md")
	require.NotNil(t, orch)
	assert.Equal(t, orchestration.WaveStateAllComplete, orch.State())
	assert.True(t, orch.IsTaskComplete(1))
	assert.True(t, orch.IsTaskComplete(2))
}

func TestProcessor_ProcessWaveSignals(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusImplementing,
		Branch:   "plan/my-plan",
	})
	store.SetContent("test", "my-plan.md", "# Plan\n\n**Goal:** test\n\n**Architecture:** test\n\n**Tech Stack:** go\n\n**Size:** Small\n\n---\n\n## Wave 1\n\n### Task 1: Test\n\nDo the thing.")

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test"})
	waveSignals := []taskfsm.WaveSignal{
		{TaskFile: "my-plan.md", WaveNumber: 1},
	}

	actions := p.ProcessWaveSignals(waveSignals)
	var found bool
	for _, a := range actions {
		if aw, ok := a.(AdvanceWaveAction); ok {
			assert.Equal(t, 1, aw.Wave)
			found = true
		}
	}
	assert.True(t, found, "expected AdvanceWaveAction")
}

func TestProcessor_ProcessFSMSignals_ReviewCycleLimitReached(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("proj", taskstore.TaskEntry{
		Filename: "test.md",
		Status:   taskstore.StatusReviewing,
	})
	// Increment review_cycle to 2 (the limit).
	store.IncrementReviewCycle("proj", "test.md")
	store.IncrementReviewCycle("proj", "test.md")

	p := NewProcessor(ProcessorConfig{
		AutoReviewFix:      true,
		Store:              store,
		Project:            "proj",
		MaxReviewFixCycles: 2,
	})

	signals := []taskfsm.Signal{
		{Event: taskfsm.ReviewChangesRequested, TaskFile: "test.md", Body: "fix this"},
	}

	actions := p.ProcessFSMSignals(signals)

	var foundLimit, foundIncrement bool
	for _, a := range actions {
		if lim, ok := a.(ReviewCycleLimitAction); ok {
			assert.Equal(t, "test.md", lim.PlanFile)
			assert.Equal(t, 3, lim.Cycle) // current(2) + 1 for the pending increment
			assert.Equal(t, 2, lim.Limit)
			foundLimit = true
		}
		if _, ok := a.(IncrementReviewCycleAction); ok {
			foundIncrement = true
		}
	}
	assert.True(t, foundLimit, "expected ReviewCycleLimitAction when cycle limit reached")
	assert.False(t, foundIncrement, "should not increment review cycle when cycle limit reached")

	// Should NOT have SpawnCoderAction
	for _, a := range actions {
		if _, ok := a.(SpawnCoderAction); ok {
			t.Fatal("should not emit SpawnCoderAction when cycle limit reached")
		}
	}
}

func TestProcessor_HooksAttachedToFSM(t *testing.T) {
	store := taskstore.NewTestStore(t)

	// Build a registry with a single notify hook.
	hookCfgs := []taskfsm.HookConfig{
		{Type: "notify"},
	}
	registry := taskfsm.BuildHookRegistry(hookCfgs)
	require.NotNil(t, registry)
	require.Equal(t, 1, registry.Len())

	// Pass the registry through ProcessorConfig — ensures startup wiring compiles
	// and the field is accepted without error.
	p := NewProcessor(ProcessorConfig{
		Store:   store,
		Project: "test",
		Hooks:   registry,
	})
	require.NotNil(t, p)
	// The FSM should have hooks attached; we verify indirectly: Processor was
	// successfully constructed (no panic) and the FSM field is non-nil.
	assert.NotNil(t, p.fsm, "expected non-nil FSM")
}

func TestProcessor_ProcessFSMSignals_ReviewCycleBelowLimit(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("proj", taskstore.TaskEntry{
		Filename: "test.md",
		Status:   taskstore.StatusReviewing,
	})
	// review_cycle = 0 (below limit of 3)

	p := NewProcessor(ProcessorConfig{
		AutoReviewFix:      true,
		Store:              store,
		Project:            "proj",
		MaxReviewFixCycles: 3,
	})

	signals := []taskfsm.Signal{
		{Event: taskfsm.ReviewChangesRequested, TaskFile: "test.md", Body: "fix this"},
	}

	actions := p.ProcessFSMSignals(signals)

	var foundFixer bool
	for _, a := range actions {
		if _, ok := a.(SpawnFixerAction); ok {
			foundFixer = true
		}
	}
	assert.True(t, foundFixer, "expected SpawnFixerAction when below cycle limit")
}
