package loop

import (
	"testing"

	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstore"
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

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test"})
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

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test"})
	signals := []taskfsm.Signal{
		{Event: taskfsm.ReviewApproved, TaskFile: "my-plan.md", Body: "LGTM"},
	}

	actions := p.ProcessFSMSignals(signals)
	var foundPR bool
	for _, a := range actions {
		if pr, ok := a.(CreatePRAction); ok {
			assert.Equal(t, "my-plan.md", pr.PlanFile)
			foundPR = true
		}
	}
	assert.True(t, foundPR, "expected CreatePRAction")
}

func TestProcessor_ProcessFSMSignals_ReviewChangesRequested(t *testing.T) {
	store := taskstore.NewTestStore(t)
	store.Create("test", taskstore.TaskEntry{
		Filename: "my-plan.md",
		Status:   taskstore.StatusReviewing,
		Branch:   "plan/my-plan",
	})

	p := NewProcessor(ProcessorConfig{Store: store, Project: "test"})
	feedback := "fix the error handling in handler.go"
	signals := []taskfsm.Signal{
		{Event: taskfsm.ReviewChangesRequested, TaskFile: "my-plan.md", Body: feedback},
	}

	actions := p.ProcessFSMSignals(signals)
	var foundCoder, foundIncrement bool
	for _, a := range actions {
		if sc, ok := a.(SpawnCoderAction); ok {
			assert.Equal(t, feedback, sc.Feedback)
			foundCoder = true
		}
		if _, ok := a.(IncrementReviewCycleAction); ok {
			foundIncrement = true
		}
	}
	assert.True(t, foundCoder, "expected SpawnCoderAction")
	assert.True(t, foundIncrement, "expected IncrementReviewCycleAction")
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
