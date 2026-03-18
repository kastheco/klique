package cmd

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteStatus_HappyPath(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "test-project"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "feature-a",
		Status:      taskstore.Status("implementing"),
		Description: "feature a",
		Branch:      "plan/feature-a",
		CreatedAt:   time.Now(),
	}))
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:    "feature-b",
		Status:      taskstore.StatusReady,
		Description: "feature b",
		Branch:      "plan/feature-b",
		CreatedAt:   time.Now(),
	}))

	state := newTestStateFromRaw(t, []instanceTestData{
		{Title: "coder-1", Status: 0, Branch: "plan/feature-a", Program: "claude", TaskFile: "feature-a", AgentType: "coder"},
		{Title: "solo-agent", Status: 3, Branch: "main", Program: "claude"},
	})

	epoch := int64(1741084800)
	tmuxOutput := strings.Join([]string{
		fakeTmuxLine("kas_coder-1", epoch, 1, false, 200, 50),
		fakeTmuxLine("kas_orphan-sess", epoch, 1, false, 200, 50),
	}, "\n")
	ex := cmd_test.NewMockExecutor()
	ex.OutputFunc = func(_ *exec.Cmd) ([]byte, error) {
		return []byte(tmuxOutput), nil
	}

	output := executeStatus(state, store, project, ex, "text")

	// tasks section
	assert.Contains(t, output, "tasks:")
	assert.Contains(t, output, "feature-a")
	assert.Contains(t, output, "implementing")
	// instances section
	assert.Contains(t, output, "instances:")
	assert.Contains(t, output, "coder-1")
	assert.Contains(t, output, "running")
	// orphan section
	assert.Contains(t, output, "orphan tmux sessions:")
	assert.Contains(t, output, "kas_orphan-sess")
	// hints: ready task → implement hint, paused instance → resume hint, orphan → tmux hints
	assert.Contains(t, output, "hints:")
	assert.Contains(t, output, "kas task implement <task-name>")
	assert.Contains(t, output, "kas instance resume <title>")
	assert.Contains(t, output, "kas tmux adopt <session> <title>")
	assert.Contains(t, output, "kas tmux kill <session>")
}

func TestExecuteStatus_Empty(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "empty-project"
	state := newTestStateFromRaw(t, []instanceTestData{})
	ex := cmd_test.NewMockExecutor()
	ex.OutputFunc = func(_ *exec.Cmd) ([]byte, error) {
		return nil, errors.New("no tmux")
	}

	output := executeStatus(state, store, project, ex, "text")
	assert.Contains(t, output, "no active tasks")
	assert.Contains(t, output, "no instances")
	assert.Contains(t, output, "no orphan tmux sessions")
}

func TestExecuteStatus_JSON(t *testing.T) {
	store := taskstore.NewTestSQLiteStore(t)
	project := "json-project"
	require.NoError(t, store.Create(project, taskstore.TaskEntry{
		Filename:  "plan-x",
		Status:    taskstore.StatusReady,
		Branch:    "plan/plan-x",
		CreatedAt: time.Now(),
	}))
	state := newTestStateFromRaw(t, []instanceTestData{
		{Title: "agent-1", Status: 0, Branch: "plan/plan-x", Program: "claude"},
	})
	ex := cmd_test.NewMockExecutor()
	ex.OutputFunc = func(_ *exec.Cmd) ([]byte, error) {
		return nil, errors.New("no tmux")
	}

	output := executeStatus(state, store, project, ex, "json")
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Contains(t, parsed, "tasks")
	assert.Contains(t, parsed, "instances")
	assert.Contains(t, parsed, "orphan_sessions")
}

func TestExecuteStatus_NilStore(t *testing.T) {
	state := newTestStateFromRaw(t, []instanceTestData{
		{Title: "solo", Status: 0, Branch: "main", Program: "claude"},
	})
	ex := cmd_test.NewMockExecutor()
	ex.OutputFunc = func(_ *exec.Cmd) ([]byte, error) {
		return nil, errors.New("no tmux")
	}

	output := executeStatus(state, nil, "test-project", ex, "text")
	assert.Contains(t, output, "no active tasks")
	assert.Contains(t, output, "solo")
}
