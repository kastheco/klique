package cmd

import (
	"encoding/json"
	"testing"

	"github.com/kastheco/kasmos/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// instanceTestData mirrors session.InstanceData fields needed for constructing
// test states without importing the session package (which would create an import
// cycle via session/tmux → cmd → session).
type instanceTestData struct {
	Title     string `json:"title"`
	Status    int    `json:"status"` // 0=Running,1=Ready,2=Loading,3=Paused
	Branch    string `json:"branch"`
	Program   string `json:"program"`
	TaskFile  string `json:"task_file,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

// newTestStateFromRaw returns a State pre-populated with the given instance records.
func newTestStateFromRaw(t *testing.T, instances []instanceTestData) *config.State {
	t.Helper()
	raw, err := json.Marshal(instances)
	require.NoError(t, err)
	s := config.DefaultState()
	s.InstancesData = raw
	return s
}

func TestInstanceList_Text(t *testing.T) {
	state := newTestStateFromRaw(t, []instanceTestData{
		{Title: "planner-foo", Status: 0 /* Running */, Branch: "plan/foo", Program: "claude", TaskFile: "foo.md"},
		{Title: "coder-bar", Status: 1 /* Ready */, Branch: "plan/bar", Program: "opencode", TaskFile: "bar.md"},
		{Title: "solo-baz", Status: 3 /* Paused */, Branch: "plan/baz", Program: "claude"},
	})

	output := executeInstanceList(state, "text")
	assert.Contains(t, output, "planner-foo")
	assert.Contains(t, output, "coder-bar")
	assert.Contains(t, output, "solo-baz")
	assert.Contains(t, output, "running")
	assert.Contains(t, output, "ready")
	assert.Contains(t, output, "paused")
}

func TestInstanceList_JSON(t *testing.T) {
	instances := []instanceTestData{
		{Title: "agent-1", Status: 0 /* Running */, Branch: "plan/agent", Program: "claude"},
	}
	state := newTestStateFromRaw(t, instances)

	output := executeInstanceList(state, "json")
	var parsed []map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed, 1)
	assert.Equal(t, "agent-1", parsed[0]["title"])
}

func TestInstanceList_Empty(t *testing.T) {
	state := newTestStateFromRaw(t, []instanceTestData{})
	output := executeInstanceList(state, "text")
	assert.Equal(t, "no instances\n", output)
}

func TestInstanceList_StatusFilter(t *testing.T) {
	state := newTestStateFromRaw(t, []instanceTestData{
		{Title: "running-1", Status: 0 /* Running */, Program: "claude"},
		{Title: "paused-1", Status: 3 /* Paused */, Program: "claude"},
	})

	output := executeInstanceList(state, "text", "paused")
	assert.Contains(t, output, "paused-1")
	assert.NotContains(t, output, "running-1")
}

func TestFindInstanceData_Found(t *testing.T) {
	records := []instanceRecord{
		{Title: "alpha", Status: instanceRunning},
		{Title: "beta", Status: instancePaused},
	}
	found, err := findInstanceData(records, "beta")
	require.NoError(t, err)
	assert.Equal(t, "beta", found.Title)
	assert.Equal(t, instancePaused, found.Status)
}

func TestFindInstanceData_NotFound(t *testing.T) {
	records := []instanceRecord{
		{Title: "alpha", Status: instanceRunning},
	}
	_, err := findInstanceData(records, "missing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindInstanceData_FuzzyMatch(t *testing.T) {
	records := []instanceRecord{
		{Title: "planner-my-feature", Status: instanceRunning},
		{Title: "coder-my-feature-task-1", Status: instanceRunning},
	}
	// Exact match should work
	found, err := findInstanceData(records, "planner-my-feature")
	require.NoError(t, err)
	assert.Equal(t, "planner-my-feature", found.Title)

	// Substring match when no exact match
	found, err = findInstanceData(records, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "coder-my-feature-task-1", found.Title)
}

func TestFindInstanceData_AmbiguousSubstring(t *testing.T) {
	records := []instanceRecord{
		{Title: "planner-foo", Status: instanceRunning},
		{Title: "coder-foo", Status: instanceRunning},
	}
	_, err := findInstanceData(records, "foo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestValidateInstanceStatus_Kill(t *testing.T) {
	// Kill should work on any status
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceRunning}, "kill"))
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceReady}, "kill"))
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instancePaused}, "kill"))
}

func TestValidateInstanceStatus_Pause(t *testing.T) {
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceRunning}, "pause"))
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceReady}, "pause"))
	assert.Error(t, validateStatusForAction(instanceRecord{Status: instancePaused}, "pause"))
}

func TestValidateInstanceStatus_Resume(t *testing.T) {
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instancePaused}, "resume"))
	assert.Error(t, validateStatusForAction(instanceRecord{Status: instanceRunning}, "resume"))
}

func TestValidateInstanceStatus_Send(t *testing.T) {
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceRunning}, "send"))
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceReady}, "send"))
	assert.Error(t, validateStatusForAction(instanceRecord{Status: instancePaused}, "send"))
}
