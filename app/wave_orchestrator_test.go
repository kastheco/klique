package app

import (
	"testing"

	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildClickUpComment(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		detail  string
		wantSub string
	}{
		{"plan_ready", "plan_ready", "3 tasks, 2 waves", "plan finalized"},
		{"wave_complete", "wave_complete", "wave 1/2: 3/3 tasks", "wave 1/2"},
		{"review_approved", "review_approved", "", "review approved"},
		{"review_changes", "review_changes_requested", "fix the tests", "changes requested"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := buildClickUpProgressComment(tt.event, "my-feature", tt.detail)
			assert.Contains(t, comment, tt.wantSub)
		})
	}
}

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

	orch := NewWaveOrchestrator("plan.md", plan)
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

	orch := NewWaveOrchestrator("plan.md", plan)
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

	orch := NewWaveOrchestrator("plan.md", plan)
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

	orch := NewWaveOrchestrator("plan.md", plan)
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

	orch := NewWaveOrchestrator("plan.md", plan)

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

	orch := NewWaveOrchestrator("plan.md", plan)
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

	orch := NewWaveOrchestrator("plan.md", plan)
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
	orch := NewWaveOrchestrator("test.md", plan)
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
	orch := NewWaveOrchestrator("test.md", plan)
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

	orch := NewWaveOrchestrator("plan.md", plan)
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
