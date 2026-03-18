package tasktools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/internal/mcpserver"
	"github.com/kastheco/kasmos/internal/mcpserver/tasktools"
)

const testProject = "test-project"

// callTool finds a registered tool by name and invokes its handler directly,
// bypassing the HTTP transport layer for faster, hermetic tests.
func callTool(t *testing.T, srv *mcpserver.Server, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := srv.MCPServer().GetTool(name)
	require.NotNil(t, tool, "tool %q must be registered", name)
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := tool.Handler(context.Background(), req)
	require.NoError(t, err)
	return result
}

// resultText returns the text content from the first content item in the result.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "result must have at least one content item")
	return mcp.GetTextFromContent(result.Content[0])
}

// setupServer creates an in-memory test store, builds an mcpserver.Server, and
// registers tasktools. The store is automatically closed when the test ends.
func setupServer(t *testing.T) *mcpserver.Server {
	t.Helper()
	store := taskstore.NewTestStore(t)
	srv := mcpserver.NewServer("0.1.0", store, nil, testProject)
	tasktools.Register(srv)
	return srv
}

// seedTask creates a task in the test store with the given filename and content.
func seedTask(t *testing.T, srv *mcpserver.Server, filename, content string) {
	t.Helper()
	ps, err := taskstate.Load(srv.Store(), testProject, "")
	require.NoError(t, err)
	err = ps.CreateWithContent(filename, filename, "", "", time.Now(), content)
	require.NoError(t, err)
}

// TestTaskShow_ReturnsContent verifies that task_show returns the stored content as JSON.
func TestTaskShow_ReturnsContent(t *testing.T) {
	srv := setupServer(t)
	seedTask(t, srv, "my-task", "# My Task\n\nSome content here.")

	result := callTool(t, srv, "task_show", map[string]any{"filename": "my-task"})
	assert.False(t, result.IsError, "task_show should not return an error")

	text := resultText(t, result)
	var resp struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, "my-task", resp.Filename)
	assert.Contains(t, resp.Content, "# My Task")
}

// TestTaskShow_NotFound verifies that task_show returns an error result for a missing task.
func TestTaskShow_NotFound(t *testing.T) {
	srv := setupServer(t)

	result := callTool(t, srv, "task_show", map[string]any{"filename": "no-such-task"})
	assert.True(t, result.IsError, "task_show should return an error for missing task")
	assert.Contains(t, resultText(t, result), "no-such-task")
}

// TestTaskShow_EmptyContent verifies that task_show returns an error when no content is stored.
func TestTaskShow_EmptyContent(t *testing.T) {
	srv := setupServer(t)
	// Create task without content so GetContent returns "".
	ps, err := taskstate.Load(srv.Store(), testProject, "")
	require.NoError(t, err)
	require.NoError(t, ps.Create("empty-task", "empty-task", "", "", time.Now()))

	result := callTool(t, srv, "task_show", map[string]any{"filename": "empty-task"})
	assert.True(t, result.IsError, "task_show should return an error for empty content")
	assert.Contains(t, resultText(t, result), "empty-task")
}

// TestTaskShow_StoreNotConfigured verifies that task_show returns a clear error
// when the server was constructed without a store.
func TestTaskShow_StoreNotConfigured(t *testing.T) {
	srv := mcpserver.NewServer("0.1.0", nil, nil, testProject)
	tasktools.Register(srv)

	result := callTool(t, srv, "task_show", map[string]any{"filename": "anything"})
	assert.True(t, result.IsError, "task_show must error when store is nil")
	assert.Equal(t, "task store not configured", resultText(t, result))
}

// TestTaskList_ReturnsAllTasks verifies that task_list returns all non-cancelled tasks
// sorted by filename when no status filter is provided.
func TestTaskList_ReturnsAllTasks(t *testing.T) {
	srv := setupServer(t)
	seedTask(t, srv, "task-a", "# Task A")
	seedTask(t, srv, "task-b", "# Task B")

	result := callTool(t, srv, "task_list", map[string]any{})
	assert.False(t, result.IsError)

	text := resultText(t, result)
	var entries []struct {
		Filename    string `json:"filename"`
		Status      string `json:"status"`
		Description string `json:"description"`
		Branch      string `json:"branch"`
		Topic       string `json:"topic"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &entries))
	require.Len(t, entries, 2)
	assert.Equal(t, "task-a", entries[0].Filename)
	assert.Equal(t, "task-b", entries[1].Filename)
}

// TestTaskList_FilterByStatus verifies that task_list filters correctly by status.
func TestTaskList_FilterByStatus(t *testing.T) {
	srv := setupServer(t)
	seedTask(t, srv, "task-a", "# Task A")
	seedTask(t, srv, "task-b", "# Task B")

	// Change task-b to implementing so the filter can distinguish them.
	ps, err := taskstate.Load(srv.Store(), testProject, "")
	require.NoError(t, err)
	require.NoError(t, ps.ForceSetStatus("task-b", taskstate.StatusImplementing))

	result := callTool(t, srv, "task_list", map[string]any{"status": "implementing"})
	assert.False(t, result.IsError)

	text := resultText(t, result)
	var entries []struct {
		Filename string `json:"filename"`
		Status   string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "task-b", entries[0].Filename)
	assert.Equal(t, "implementing", entries[0].Status)
}

// TestTaskList_HidesCancelledByDefault verifies that cancelled tasks are hidden
// when no status filter is provided.
func TestTaskList_HidesCancelledByDefault(t *testing.T) {
	srv := setupServer(t)
	seedTask(t, srv, "task-a", "# Task A")
	seedTask(t, srv, "task-b", "# Task B")

	ps, err := taskstate.Load(srv.Store(), testProject, "")
	require.NoError(t, err)
	require.NoError(t, ps.ForceSetStatus("task-b", taskstate.StatusCancelled))

	result := callTool(t, srv, "task_list", map[string]any{})
	assert.False(t, result.IsError)

	text := resultText(t, result)
	var entries []struct {
		Filename string `json:"filename"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "task-a", entries[0].Filename)
}

// TestTaskList_StoreNotConfigured verifies that task_list returns a clear error
// when the server was constructed without a store.
func TestTaskList_StoreNotConfigured(t *testing.T) {
	srv := mcpserver.NewServer("0.1.0", nil, nil, testProject)
	tasktools.Register(srv)

	result := callTool(t, srv, "task_list", map[string]any{})
	assert.True(t, result.IsError, "task_list must error when store is nil")
	assert.Equal(t, "task store not configured", resultText(t, result))
}

// TestTaskUpdateContent_StoresContent verifies that task_update_content replaces
// the stored content and returns a success response with the filename.
func TestTaskUpdateContent_StoresContent(t *testing.T) {
	srv := setupServer(t)
	seedTask(t, srv, "my-task", "# My Task\n\nOriginal content.")

	newContent := "# My Task\n\n## Wave 1\n\n### Task 1: Do Something\n\n**Goal:** Updated goal."
	result := callTool(t, srv, "task_update_content", map[string]any{
		"filename": "my-task",
		"content":  newContent,
	})
	assert.False(t, result.IsError, "task_update_content should not return an error")

	text := resultText(t, result)
	var resp struct {
		Filename string `json:"filename"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, "my-task", resp.Filename)

	// Verify the content was actually updated in the store.
	ps, err := taskstate.Load(srv.Store(), testProject, "")
	require.NoError(t, err)
	stored, err := ps.GetContent("my-task")
	require.NoError(t, err)
	assert.Equal(t, newContent, stored)
}

// TestTaskUpdateContent_WarningOnBadStructure verifies that task_update_content
// returns IsError=false with a non-empty warning field when the content is
// stored but the plan structure (wave headers) cannot be parsed.
func TestTaskUpdateContent_WarningOnBadStructure(t *testing.T) {
	srv := setupServer(t)
	// Seed a task without content so IngestContent can find it.
	ps, err := taskstate.Load(srv.Store(), testProject, "")
	require.NoError(t, err)
	require.NoError(t, ps.Create("my-task", "my-task", "", "", time.Now()))

	// Content is valid text but has no Wave sections — parse will warn.
	badContent := "# Plan\n\n**Goal:** parsed but no waves"
	result := callTool(t, srv, "task_update_content", map[string]any{
		"filename": "my-task",
		"content":  badContent,
	})
	assert.False(t, result.IsError, "task_update_content must not return IsError for a parse warning")

	text := resultText(t, result)
	var resp struct {
		Filename string `json:"filename"`
		Warning  string `json:"warning"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, "my-task", resp.Filename)
	assert.NotEmpty(t, resp.Warning, "warning field must be non-empty for bad structure")
}

// TestTaskUpdateContent_EmptyContent verifies that task_update_content returns
// an error result when content is empty or only whitespace.
func TestTaskUpdateContent_EmptyContent(t *testing.T) {
	srv := setupServer(t)
	seedTask(t, srv, "my-task", "# My Task\n\nSome content.")

	result := callTool(t, srv, "task_update_content", map[string]any{
		"filename": "my-task",
		"content":  "   ",
	})
	assert.True(t, result.IsError, "task_update_content must error on empty content")
	assert.Contains(t, resultText(t, result), "no content provided")
}

// TestTaskUpdateContent_NotFound verifies that task_update_content returns an
// error result when the task does not exist in the store.
func TestTaskUpdateContent_NotFound(t *testing.T) {
	srv := setupServer(t)

	result := callTool(t, srv, "task_update_content", map[string]any{
		"filename": "no-such-task",
		"content":  "# Some Content",
	})
	assert.True(t, result.IsError, "task_update_content must error for missing task")
	assert.Contains(t, resultText(t, result), "no-such-task")
}

// TestTaskUpdateContent_StoreNotConfigured verifies that task_update_content
// returns a clear error when the server was constructed without a store.
func TestTaskUpdateContent_StoreNotConfigured(t *testing.T) {
	srv := mcpserver.NewServer("0.1.0", nil, nil, testProject)
	tasktools.Register(srv)

	result := callTool(t, srv, "task_update_content", map[string]any{
		"filename": "anything",
		"content":  "# Content",
	})
	assert.True(t, result.IsError, "task_update_content must error when store is nil")
	assert.Equal(t, "task store not configured", resultText(t, result))
}

// TestTaskCreate_CreatesEntry verifies that task_create creates a new task entry
// and returns the expected JSON with filename, status=ready, and branch=plan/<name>.
func TestTaskCreate_CreatesEntry(t *testing.T) {
	srv := setupServer(t)

	result := callTool(t, srv, "task_create", map[string]any{
		"name": "new-task",
	})
	assert.False(t, result.IsError, "task_create should not return an error")

	text := resultText(t, result)
	var resp struct {
		Filename string `json:"filename"`
		Status   string `json:"status"`
		Branch   string `json:"branch"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, "new-task", resp.Filename)
	assert.Equal(t, "ready", resp.Status)
	assert.Equal(t, "plan/new-task", resp.Branch)

	// Verify entry in the store.
	ps, err := taskstate.Load(srv.Store(), testProject, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("new-task")
	assert.True(t, ok, "task entry must exist in store")
	assert.Equal(t, taskstate.StatusReady, entry.Status)
}

// TestTaskCreate_WithContent verifies that task_create stores the given content
// when the optional content argument is provided.
func TestTaskCreate_WithContent(t *testing.T) {
	srv := setupServer(t)
	content := "# My Task\n\nSome plan content."

	result := callTool(t, srv, "task_create", map[string]any{
		"name":    "content-task",
		"content": content,
	})
	assert.False(t, result.IsError, "task_create should not error when content is provided")

	// Only assert that content was stored; do not assert parsed goal/subtasks.
	ps, err := taskstate.Load(srv.Store(), testProject, "")
	require.NoError(t, err)
	stored, err := ps.GetContent("content-task")
	require.NoError(t, err)
	assert.Equal(t, content, stored)
}

// TestTaskCreate_WithBranchAndTopic verifies that custom branch and topic values
// are stored and reflected in the response.
func TestTaskCreate_WithBranchAndTopic(t *testing.T) {
	srv := setupServer(t)

	result := callTool(t, srv, "task_create", map[string]any{
		"name":   "branched-task",
		"branch": "feat/custom",
		"topic":  "my-topic",
	})
	assert.False(t, result.IsError, "task_create should not error with branch and topic")

	text := resultText(t, result)
	var resp struct {
		Filename string `json:"filename"`
		Branch   string `json:"branch"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, "branched-task", resp.Filename)
	assert.Equal(t, "feat/custom", resp.Branch)

	// Verify branch and topic in store.
	ps, err := taskstate.Load(srv.Store(), testProject, "")
	require.NoError(t, err)
	entry, ok := ps.Entry("branched-task")
	require.True(t, ok)
	assert.Equal(t, "feat/custom", entry.Branch)
	assert.Equal(t, "my-topic", entry.Topic)
}

// TestTaskCreate_Duplicate verifies that creating a task with an already-used
// name returns an error result.
func TestTaskCreate_Duplicate(t *testing.T) {
	srv := setupServer(t)
	seedTask(t, srv, "existing-task", "# Existing Task")

	result := callTool(t, srv, "task_create", map[string]any{
		"name": "existing-task",
	})
	assert.True(t, result.IsError, "task_create must error on duplicate name")
}

// TestTaskCreate_StoreNotConfigured verifies that task_create returns a clear
// error when the server was constructed without a store.
func TestTaskCreate_StoreNotConfigured(t *testing.T) {
	srv := mcpserver.NewServer("0.1.0", nil, nil, testProject)
	tasktools.Register(srv)

	result := callTool(t, srv, "task_create", map[string]any{
		"name": "anything",
	})
	assert.True(t, result.IsError, "task_create must error when store is nil")
	assert.Equal(t, "task store not configured", resultText(t, result))
}

// setupServerWithGateway creates an in-memory test store and signal gateway,
// builds an mcpserver.Server, and registers tasktools. Both the store and
// gateway are automatically closed when the test ends.
func setupServerWithGateway(t *testing.T) *mcpserver.Server {
	t.Helper()
	store := taskstore.NewTestStore(t)
	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = gw.Close() })
	srv := mcpserver.NewServer("0.1.0", store, gw, testProject)
	tasktools.Register(srv)
	return srv
}

// taskTransitionResult is the JSON payload returned by the task_transition tool.
type taskTransitionResult struct {
	Filename  string `json:"filename"`
	NewStatus string `json:"new_status"`
}

// signalCreateResult is the JSON payload returned by the signal_create tool.
type signalCreateResult struct {
	PlanFile   string `json:"plan_file"`
	SignalType string `json:"signal_type"`
}

// TestTaskTransition_AppliesEvent verifies that task_transition applies a valid
// FSM event and returns the new status in JSON.
func TestTaskTransition_AppliesEvent(t *testing.T) {
	srv := setupServerWithGateway(t)
	seedTask(t, srv, "my-task", "# My Task")

	result := callTool(t, srv, "task_transition", map[string]any{
		"filename": "my-task",
		"event":    "plan_start",
	})
	assert.False(t, result.IsError, "task_transition should not return an error for valid event")

	text := resultText(t, result)
	var resp taskTransitionResult
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, "my-task", resp.Filename)
	assert.Equal(t, "planning", resp.NewStatus)
}

// TestTaskTransition_InvalidEvent verifies that task_transition returns an error
// result when an unknown event name is given.
func TestTaskTransition_InvalidEvent(t *testing.T) {
	srv := setupServerWithGateway(t)
	seedTask(t, srv, "my-task", "# My Task")

	result := callTool(t, srv, "task_transition", map[string]any{
		"filename": "my-task",
		"event":    "not_a_real_event",
	})
	assert.True(t, result.IsError, "task_transition must error for unknown event")
	assert.Contains(t, resultText(t, result), "not_a_real_event")
}

// TestTaskTransition_InvalidTransition verifies that task_transition returns an
// error result when the event is known but invalid for the current state.
func TestTaskTransition_InvalidTransition(t *testing.T) {
	srv := setupServerWithGateway(t)
	// Task is in "ready" status; review_approved is only valid from "reviewing".
	seedTask(t, srv, "my-task", "# My Task")

	result := callTool(t, srv, "task_transition", map[string]any{
		"filename": "my-task",
		"event":    "review_approved",
	})
	assert.True(t, result.IsError, "task_transition must error for invalid FSM transition")
}

// TestTaskTransition_NotFound verifies that task_transition returns an error
// result when the task does not exist in the store.
func TestTaskTransition_NotFound(t *testing.T) {
	srv := setupServerWithGateway(t)

	result := callTool(t, srv, "task_transition", map[string]any{
		"filename": "no-such-task",
		"event":    "plan_start",
	})
	assert.True(t, result.IsError, "task_transition must error for missing task")
	assert.Contains(t, resultText(t, result), "no-such-task")
}

// TestTaskTransition_StoreNotConfigured verifies that task_transition returns a
// clear error when the server was constructed without a store.
func TestTaskTransition_StoreNotConfigured(t *testing.T) {
	srv := mcpserver.NewServer("0.1.0", nil, nil, testProject)
	tasktools.Register(srv)

	result := callTool(t, srv, "task_transition", map[string]any{
		"filename": "anything",
		"event":    "plan_start",
	})
	assert.True(t, result.IsError, "task_transition must error when store is nil")
	assert.Equal(t, "task store not configured", resultText(t, result))
}

// TestSignalCreate_EmitsSignal verifies that signal_create inserts a pending
// signal into the gateway.
func TestSignalCreate_EmitsSignal(t *testing.T) {
	srv := setupServerWithGateway(t)

	result := callTool(t, srv, "signal_create", map[string]any{
		"signal_type": "planner_finished",
		"plan_file":   "my-plan",
	})
	assert.False(t, result.IsError, "signal_create should not return an error")

	text := resultText(t, result)
	var resp signalCreateResult
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, "my-plan", resp.PlanFile)
	assert.Equal(t, "planner_finished", resp.SignalType)

	// Verify the signal was actually inserted into the gateway.
	signals, err := srv.Gateway().List(testProject, taskstore.SignalPending)
	require.NoError(t, err)
	require.Len(t, signals, 1)
	assert.Equal(t, "my-plan", signals[0].PlanFile)
	assert.Equal(t, "planner_finished", signals[0].SignalType)
}

// TestSignalCreate_WithPayload verifies that signal_create correctly stores a
// structured payload for implement_task_finished.
func TestSignalCreate_WithPayload(t *testing.T) {
	srv := setupServerWithGateway(t)
	payload := `{"wave_number":2,"task_number":3}`

	result := callTool(t, srv, "signal_create", map[string]any{
		"signal_type": "implement_task_finished",
		"plan_file":   "my-plan",
		"payload":     payload,
	})
	assert.False(t, result.IsError, "signal_create should not return an error with valid payload")

	signals, err := srv.Gateway().List(testProject, taskstore.SignalPending)
	require.NoError(t, err)
	require.Len(t, signals, 1)
	assert.Equal(t, payload, signals[0].Payload)
}

// TestSignalCreate_InvalidType verifies that signal_create returns an error
// result for an unknown signal type.
func TestSignalCreate_InvalidType(t *testing.T) {
	srv := setupServerWithGateway(t)

	result := callTool(t, srv, "signal_create", map[string]any{
		"signal_type": "not_a_real_signal",
		"plan_file":   "my-plan",
	})
	assert.True(t, result.IsError, "signal_create must error for unknown signal type")
	assert.Contains(t, resultText(t, result), "not_a_real_signal")
}

// TestSignalCreate_InvalidPayload verifies that signal_create returns an error
// result when the payload is invalid for the given signal type.
func TestSignalCreate_InvalidPayload(t *testing.T) {
	srv := setupServerWithGateway(t)

	// elaborator_finished does not accept a payload.
	result := callTool(t, srv, "signal_create", map[string]any{
		"signal_type": "elaborator_finished",
		"plan_file":   "my-plan",
		"payload":     "unexpected payload",
	})
	assert.True(t, result.IsError, "signal_create must error for invalid payload")
}

// TestSignalCreate_NilGateway verifies that signal_create returns a clear error
// when the server was constructed without a signal gateway.
func TestSignalCreate_NilGateway(t *testing.T) {
	srv := setupServer(t) // store-only, no gateway

	result := callTool(t, srv, "signal_create", map[string]any{
		"signal_type": "planner_finished",
		"plan_file":   "my-plan",
	})
	assert.True(t, result.IsError, "signal_create must error when gateway is nil")
	assert.Equal(t, "signal gateway not configured", resultText(t, result))
}
