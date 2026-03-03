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
