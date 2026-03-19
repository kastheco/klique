package instancetools

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/daemon/api"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner is a testable CmdRunner that delegates to provided function fields.
type mockRunner struct {
	runFn    func(ctx context.Context, name string, args ...string) error
	outputFn func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) error {
	if m.runFn != nil {
		return m.runFn(ctx, name, args...)
	}
	return nil
}

func (m *mockRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if m.outputFn != nil {
		return m.outputFn(ctx, name, args...)
	}
	return nil, nil
}

// mockReq builds a CallToolRequest with the provided argument map.
func mockReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

// seedInstances returns a StateLoader backed by an in-memory State pre-populated
// with the given records. None of the seeded state is written to disk.
func seedInstances(records ...instanceRecord) StateLoader {
	state := config.DefaultState()
	raw, _ := json.Marshal(records)
	state.InstancesData = json.RawMessage(raw)
	return func() config.StateManager { return state }
}

// textResult extracts the first TextContent from a CallToolResult and returns
// its text string. The test fails if the result is nil, has no content, or the
// first content item is not a TextContent.
func textResult(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	return tc.Text
}

// ---------------------------------------------------------------------------
// RegisterTools
// ---------------------------------------------------------------------------

// TestRegisterTools_NilServer verifies RegisterTools does not panic when
// passed a nil server.
func TestRegisterTools_NilServer(t *testing.T) {
	loader := seedInstances()
	runner := &mockRunner{}
	assert.NotPanics(t, func() { RegisterTools(nil, loader, runner, "") })
}

// TestRegisterTools_NilRunner verifies RegisterTools replaces a nil runner
// with ExecRunner without panicking.
func TestRegisterTools_NilRunner(t *testing.T) {
	loader := seedInstances()
	assert.NotPanics(t, func() { RegisterTools(nil, loader, nil, "") })
}

// ---------------------------------------------------------------------------
// instance_list
// ---------------------------------------------------------------------------

// TestInstanceList_Empty verifies instance_list returns an empty JSON array
// when no instances are registered.
func TestInstanceList_Empty(t *testing.T) {
	handler := makeInstanceListHandler(seedInstances())
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := textResult(t, result)
	var entries []instanceListEntry
	require.NoError(t, json.Unmarshal([]byte(text), &entries))
	assert.Empty(t, entries)
}

// TestInstanceList_WithTimestamp verifies that a non-zero CreatedAt value is
// formatted as RFC3339, matching cmd/instance.go:212.
func TestInstanceList_WithTimestamp(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	loader := seedInstances(instanceRecord{
		Title:     "my-instance",
		Status:    instanceRunning,
		Branch:    "main",
		Program:   "claude",
		TaskFile:  "task.md",
		AgentType: "coder",
		CreatedAt: now,
	})
	handler := makeInstanceListHandler(loader)
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := textResult(t, result)
	var entries []instanceListEntry
	require.NoError(t, json.Unmarshal([]byte(text), &entries))
	require.Len(t, entries, 1)

	assert.Equal(t, "my-instance", entries[0].Title)
	assert.Equal(t, "running", entries[0].Status)
	assert.Equal(t, "main", entries[0].Branch)
	assert.Equal(t, "claude", entries[0].Program)
	assert.Equal(t, "task.md", entries[0].TaskFile)
	assert.Equal(t, "coder", entries[0].AgentType)

	// Verify RFC3339 format.
	parsed, parseErr := time.Parse(time.RFC3339, entries[0].CreatedAt)
	require.NoError(t, parseErr, "CreatedAt should be RFC3339")
	assert.Equal(t, now, parsed.UTC())
}

// TestInstanceList_ZeroTimestamp verifies that a zero CreatedAt is omitted
// from the JSON output (omitempty behaviour).
func TestInstanceList_ZeroTimestamp(t *testing.T) {
	loader := seedInstances(instanceRecord{
		Title:  "my-instance",
		Status: instanceRunning,
		// CreatedAt is zero value — should be omitted.
	})
	handler := makeInstanceListHandler(loader)
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := textResult(t, result)
	var entries []instanceListEntry
	require.NoError(t, json.Unmarshal([]byte(text), &entries))
	require.Len(t, entries, 1)
	assert.Empty(t, entries[0].CreatedAt)
}

// TestInstanceList_StatusFilter verifies that only instances matching the
// requested status label are returned.
func TestInstanceList_StatusFilter(t *testing.T) {
	loader := seedInstances(
		instanceRecord{Title: "running-one", Status: instanceRunning},
		instanceRecord{Title: "paused-one", Status: instancePaused},
	)
	handler := makeInstanceListHandler(loader)
	result, err := handler(context.Background(), mockReq(map[string]any{"status": "paused"}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := textResult(t, result)
	var entries []instanceListEntry
	require.NoError(t, json.Unmarshal([]byte(text), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "paused-one", entries[0].Title)
}

// ---------------------------------------------------------------------------
// instance_send
// ---------------------------------------------------------------------------

// TestInstanceSend_SendsKeys verifies that instance_send issues the exact
// tmux send-keys call: tmux send-keys -t kas_<title> <prompt> Enter.
func TestInstanceSend_SendsKeys(t *testing.T) {
	rec := instanceRecord{
		Title:   "my-agent",
		Status:  instanceRunning,
		Program: "claude",
	}
	loader := seedInstances(rec)

	var capturedName string
	var capturedArgs []string
	runner := &mockRunner{
		runFn: func(_ context.Context, name string, args ...string) error {
			capturedName = name
			capturedArgs = args
			return nil
		},
	}

	handler := makeInstanceSendHandler(loader, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{
		"title":  "my-agent",
		"prompt": "hello world",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// Verify exact tmux send-keys argument sequence.
	assert.Equal(t, "tmux", capturedName)
	require.Len(t, capturedArgs, 5)
	assert.Equal(t, "send-keys", capturedArgs[0])
	assert.Equal(t, "-t", capturedArgs[1])
	assert.Equal(t, kasTmuxName("my-agent"), capturedArgs[2])
	assert.Equal(t, "hello world", capturedArgs[3])
	assert.Equal(t, "Enter", capturedArgs[4])
}

// TestInstanceSend_MissingTitle verifies that instance_send returns a tool
// error immediately when the required "title" argument is absent.
func TestInstanceSend_MissingTitle(t *testing.T) {
	loader := seedInstances()
	handler := makeInstanceSendHandler(loader, &mockRunner{})
	result, err := handler(context.Background(), mockReq(map[string]any{
		"prompt": "hello",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

// TestInstanceSend_MissingPrompt verifies that instance_send returns a tool
// error when the required "prompt" argument is absent (title is present).
func TestInstanceSend_MissingPrompt(t *testing.T) {
	rec := instanceRecord{Title: "my-agent", Status: instanceRunning}
	loader := seedInstances(rec)
	handler := makeInstanceSendHandler(loader, &mockRunner{})
	result, err := handler(context.Background(), mockReq(map[string]any{
		"title": "my-agent",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

// TestInstanceSend_PausedInstance verifies that instance_send returns a tool
// error when the target instance is paused (validateAction enforcement).
func TestInstanceSend_PausedInstance(t *testing.T) {
	rec := instanceRecord{Title: "paused-agent", Status: instancePaused}
	loader := seedInstances(rec)
	handler := makeInstanceSendHandler(loader, &mockRunner{})
	result, err := handler(context.Background(), mockReq(map[string]any{
		"title":  "paused-agent",
		"prompt": "wake up",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "paused")
}

// ---------------------------------------------------------------------------
// daemon_status
// ---------------------------------------------------------------------------

// TestDaemonStatus_Running stands up a real Unix-socket HTTP server serving a
// canned StatusResponse and verifies the tool decodes it correctly.
func TestDaemonStatus_Running(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "kas.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.StatusResponse{Running: true, RepoCount: 1})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	handler := makeDaemonStatusHandler(socketPath)
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := textResult(t, result)
	var status api.StatusResponse
	require.NoError(t, json.Unmarshal([]byte(text), &status))
	assert.True(t, status.Running)
	assert.Equal(t, 1, status.RepoCount)
}

// TestDaemonStatus_NotRunning verifies that when no daemon is listening at the
// given socket path, the tool returns a tool error containing "daemon not running".
func TestDaemonStatus_NotRunning(t *testing.T) {
	nonExistentPath := filepath.Join(t.TempDir(), "nonexistent.sock")
	handler := makeDaemonStatusHandler(nonExistentPath)
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "daemon not running")
}
