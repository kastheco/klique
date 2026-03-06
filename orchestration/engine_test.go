package orchestration

import (
	"testing"

	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWaveOrchestrator(t *testing.T) {
	plan := &taskparser.Plan{
		Goal: "test",
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
				{Number: 2, Title: "Second", Body: "do second"},
			}},
			{Number: 2, Tasks: []taskparser.Task{
				{Number: 3, Title: "Third", Body: "do third"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)
	assert.Equal(t, WaveStateIdle, orch.State())
	assert.Equal(t, 2, orch.TotalWaves())
	assert.Equal(t, 3, orch.TotalTasks())
}

func TestWaveOrchestrator_StartWave(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)
	tasks := orch.StartNextWave()

	assert.Equal(t, WaveStateRunning, orch.State())
	assert.Equal(t, 1, orch.CurrentWaveNumber())
	require.Len(t, tasks, 1)
	assert.Equal(t, "First", tasks[0].Title)
}

func TestWaveOrchestrator_TaskCompleted(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
				{Number: 2, Title: "Second", Body: "do second"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)
	orch.StartNextWave()

	assert.False(t, orch.IsCurrentWaveComplete())

	orch.MarkTaskComplete(1)
	assert.False(t, orch.IsCurrentWaveComplete())

	orch.MarkTaskComplete(2)
	assert.True(t, orch.IsCurrentWaveComplete())
	assert.Equal(t, WaveStateAllComplete, orch.State())
}

func TestWaveOrchestrator_TaskFailed(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
				{Number: 2, Title: "Second", Body: "do second"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)
	orch.StartNextWave()

	orch.MarkTaskFailed(1)
	orch.MarkTaskComplete(2)

	assert.Equal(t, WaveStateAllComplete, orch.State())
	assert.Equal(t, 1, orch.FailedTaskCount())
	assert.Equal(t, 1, orch.CompletedTaskCount())
}

func TestWaveOrchestrator_MultiWaveProgression(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
			}},
			{Number: 2, Tasks: []taskparser.Task{
				{Number: 2, Title: "Second", Body: "do second"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)

	// Wave 1
	orch.StartNextWave()
	orch.MarkTaskComplete(1)
	assert.Equal(t, WaveStateWaveComplete, orch.State())

	// Advance to wave 2
	tasks := orch.StartNextWave()
	assert.Equal(t, WaveStateRunning, orch.State())
	assert.Equal(t, 2, orch.CurrentWaveNumber())
	require.Len(t, tasks, 1)

	// Complete wave 2
	orch.MarkTaskComplete(2)
	assert.Equal(t, WaveStateAllComplete, orch.State())
}

func TestWaveOrchestrator_AllComplete(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "Only", Body: "do it"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)
	orch.StartNextWave()
	orch.MarkTaskComplete(1)

	// No more waves — should be AllComplete
	assert.Equal(t, WaveStateAllComplete, orch.State())
}

func TestWaveOrchestrator_ResetConfirmAllowsReprompt(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
			}},
			{Number: 2, Tasks: []taskparser.Task{
				{Number: 2, Title: "Second", Body: "do second"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)
	orch.StartNextWave()
	orch.MarkTaskComplete(1) // wave 1 complete

	// First call consumes the one-shot latch
	assert.True(t, orch.NeedsConfirm(), "first call must return true")
	assert.False(t, orch.NeedsConfirm(), "second call must return false (latch consumed)")

	// After reset, NeedsConfirm should fire again
	orch.ResetConfirm()
	assert.True(t, orch.NeedsConfirm(), "after ResetConfirm, must return true again")
}

func TestIsTaskRunning(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1}, {Number: 2}}},
		},
	}
	orch := NewWaveOrchestrator("test", plan)
	orch.StartNextWave()

	assert.True(t, orch.IsTaskRunning(1), "task 1 should be running after StartNextWave")
	assert.True(t, orch.IsTaskRunning(2), "task 2 should be running after StartNextWave")

	orch.MarkTaskComplete(1)
	assert.False(t, orch.IsTaskRunning(1), "task 1 should not be running after MarkTaskComplete")
	assert.True(t, orch.IsTaskRunning(2), "task 2 should still be running")

	assert.False(t, orch.IsTaskRunning(99), "unknown task should return false")
}

func TestWaveOrchestrator_TaskStatusQueries(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "Task 1"},
				{Number: 2, Title: "Task 2"},
				{Number: 3, Title: "Task 3"},
			}},
		},
	}
	orch := NewWaveOrchestrator("test", plan)
	orch.StartNextWave()

	// All should be running initially.
	assert.True(t, orch.IsTaskRunning(1))
	assert.False(t, orch.IsTaskComplete(1))
	assert.False(t, orch.IsTaskFailed(1))

	orch.MarkTaskComplete(1)
	assert.True(t, orch.IsTaskComplete(1))
	assert.False(t, orch.IsTaskRunning(1))

	orch.MarkTaskFailed(2)
	assert.True(t, orch.IsTaskFailed(2))
	assert.False(t, orch.IsTaskRunning(2))

	// Task 3 still running.
	assert.True(t, orch.IsTaskRunning(3))
}

func TestWaveOrchestrator_RetryFailedTasksRestoresRunning(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
				{Number: 2, Title: "Second", Body: "do second"},
			}},
			{Number: 2, Tasks: []taskparser.Task{
				{Number: 3, Title: "Third", Body: "do third"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)
	orch.StartNextWave()

	// T1 fails, T2 completes — wave done with failure
	orch.MarkTaskFailed(1)
	orch.MarkTaskComplete(2)
	require.Equal(t, WaveStateWaveComplete, orch.State(), "wave must be WaveComplete with failure")
	assert.Equal(t, 1, orch.FailedTaskCount())

	// Retry the failed task
	retried := orch.RetryFailedTasks()

	assert.Equal(t, WaveStateRunning, orch.State(), "state must be Running after retry")
	require.Len(t, retried, 1, "must return only the failed task")
	assert.Equal(t, 1, retried[0].Number, "retried task must be T1")

	// After the retried task completes, wave is done again (with more waves pending)
	orch.MarkTaskComplete(1)
	assert.Equal(t, WaveStateWaveComplete, orch.State(), "wave must be WaveComplete after retry+complete")
	assert.Equal(t, 0, orch.FailedTaskCount(), "no more failures after retry completes")
}

func TestRestoreToWave(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1}, {Number: 2}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 3}}},
		},
	}
	orch := NewWaveOrchestrator("plan", plan)
	orch.RestoreToWave(2, []int{3})
	assert.Equal(t, WaveStateAllComplete, orch.State())
	assert.Equal(t, 2, orch.CurrentWaveNumber())
}

func TestRestoreToWave_PartialCompletion(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{{Number: 1}}},
			{Number: 2, Tasks: []taskparser.Task{{Number: 2}, {Number: 3}}},
		},
	}
	orch := NewWaveOrchestrator("plan", plan)
	orch.RestoreToWave(2, []int{2}) // task 3 still running
	assert.Equal(t, WaveStateRunning, orch.State())
	assert.True(t, orch.IsTaskComplete(2))
	assert.True(t, orch.IsTaskRunning(3))
}

func TestShouldPostWaveCompleteComment(t *testing.T) {
	single := &taskparser.Plan{Waves: []taskparser.Wave{
		{Number: 1, Tasks: []taskparser.Task{{Number: 1}}},
	}}
	multi := &taskparser.Plan{Waves: []taskparser.Wave{
		{Number: 1, Tasks: []taskparser.Task{{Number: 1}}},
		{Number: 2, Tasks: []taskparser.Task{{Number: 2}}},
	}}
	assert.False(t, NewWaveOrchestrator("s", single).ShouldPostWaveCompleteComment())
	assert.True(t, NewWaveOrchestrator("m", multi).ShouldPostWaveCompleteComment())

	// nil receiver safety
	var nilOrch *WaveOrchestrator
	assert.False(t, nilOrch.ShouldPostWaveCompleteComment())
}

func TestWaveOrchestrator_ElaboratingState(t *testing.T) {
	plan := &taskparser.Plan{
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)
	orch.SetElaborating()
	assert.Equal(t, WaveStateElaborating, orch.State())

	// StartNextWave should be blocked while elaborating
	tasks := orch.StartNextWave()
	assert.Nil(t, tasks, "must not start waves while elaborating")
	assert.Equal(t, WaveStateElaborating, orch.State())
}

func TestWaveOrchestrator_UpdatePlan(t *testing.T) {
	plan := &taskparser.Plan{
		Goal: "original",
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "terse body"},
			}},
		},
	}

	orch := NewWaveOrchestrator("plan", plan)
	orch.SetElaborating()

	updated := &taskparser.Plan{
		Goal: "original",
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "detailed body with signatures and patterns"},
			}},
		},
	}
	orch.UpdatePlan(updated)

	// Should transition back to Idle so StartNextWave works
	assert.Equal(t, WaveStateIdle, orch.State())

	// Verify the plan was replaced
	tasks := orch.StartNextWave()
	require.Len(t, tasks, 1)
	assert.Contains(t, tasks[0].Body, "detailed body")
}

func TestBuildTaskPrompt_Method(t *testing.T) {
	plan := &taskparser.Plan{
		Goal: "Test goal",
		Waves: []taskparser.Wave{
			{Number: 1, Tasks: []taskparser.Task{
				{Number: 1, Title: "First", Body: "do first"},
				{Number: 2, Title: "Second", Body: "do second"},
			}},
		},
	}
	orch := NewWaveOrchestrator("plan", plan)
	orch.StartNextWave()
	prompt := orch.BuildTaskPrompt(plan.Waves[0].Tasks[0], 2)
	assert.Contains(t, prompt, "Task 1")
	assert.Contains(t, prompt, "Test goal")
	assert.Contains(t, prompt, "Wave 1 of 1")
	assert.Contains(t, prompt, "parallel") // peerCount > 1
}

func TestWaveOrchestrator_PersistsSubtaskStatus(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "proj"
	planFile := "plan"

	require.NoError(t, store.Create(project, taskstore.TaskEntry{Filename: planFile, Status: taskstore.StatusImplementing}))
	require.NoError(t, store.SetSubtasks(project, planFile, []taskstore.SubtaskEntry{
		{TaskNumber: 1, Title: "first", Status: taskstore.SubtaskStatusPending},
		{TaskNumber: 2, Title: "second", Status: taskstore.SubtaskStatusPending},
	}))

	plan := &taskparser.Plan{Waves: []taskparser.Wave{{
		Number: 1,
		Tasks:  []taskparser.Task{{Number: 1, Title: "first"}, {Number: 2, Title: "second"}},
	}}}
	orch := NewWaveOrchestrator(planFile, plan)
	orch.SetStore(store, project)

	orch.StartNextWave()
	subtasks, err := store.GetSubtasks(project, planFile)
	require.NoError(t, err)
	require.Len(t, subtasks, 2)
	assert.Equal(t, taskstore.SubtaskStatusRunning, subtasks[0].Status)
	assert.Equal(t, taskstore.SubtaskStatusRunning, subtasks[1].Status)

	orch.MarkTaskComplete(1)
	orch.MarkTaskFailed(2)

	subtasks, err = store.GetSubtasks(project, planFile)
	require.NoError(t, err)
	require.Len(t, subtasks, 2)
	assert.Equal(t, taskstore.SubtaskStatusComplete, subtasks[0].Status)
	assert.Equal(t, taskstore.SubtaskStatusFailed, subtasks[1].Status)
}
