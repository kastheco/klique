package mcpclient_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_RequiresTransport(t *testing.T) {
	_, err := mcpclient.NewClient(nil)
	assert.ErrorContains(t, err, "transport required")
}

// mockTransport records calls and returns preconfigured responses.
type mockTransport struct {
	responses map[string]mcpclient.JSONRPCResponse
	closed    bool
}

func (m *mockTransport) Send(req mcpclient.JSONRPCRequest) (mcpclient.JSONRPCResponse, error) {
	if resp, ok := m.responses[req.Method]; ok {
		resp.ID = req.ID
		return resp, nil
	}
	return mcpclient.JSONRPCResponse{}, fmt.Errorf("unexpected method: %s", req.Method)
}

func (m *mockTransport) Close() error { m.closed = true; return nil }

func TestClient_Initialize(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"initialize": {Result: json.RawMessage(`{"protocolVersion":"2024-11-05"}`)},
	}}
	c, err := mcpclient.NewClient(mt)
	require.NoError(t, err)
	assert.NoError(t, c.Initialize())
}

func TestClient_Initialize_ServerError(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"initialize": {Error: &mcpclient.JSONRPCError{Code: -32600, Message: "bad request"}},
	}}
	c, err := mcpclient.NewClient(mt)
	require.NoError(t, err)
	err = c.Initialize()
	assert.ErrorContains(t, err, "bad request")
}

func TestClient_Initialize_TransportError(t *testing.T) {
	// No "initialize" response configured → transport returns error
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{}}
	c, err := mcpclient.NewClient(mt)
	require.NoError(t, err)
	err = c.Initialize()
	assert.ErrorContains(t, err, "initialize")
}

func TestClient_ListTools(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"tools/list": {Result: json.RawMessage(`{"tools":[{"name":"clickup_search_tasks"},{"name":"clickup_get_task"}]}`)},
	}}
	c, _ := mcpclient.NewClient(mt)
	tools, err := c.ListTools()
	require.NoError(t, err)
	assert.Len(t, tools, 2)
	tool, found := c.FindTool("search")
	assert.True(t, found)
	assert.Equal(t, "clickup_search_tasks", tool.Name)
}

func TestClient_ListTools_CaseInsensitiveFind(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"tools/list": {Result: json.RawMessage(`{"tools":[{"name":"ClickUp_Get_Task"}]}`)},
	}}
	c, _ := mcpclient.NewClient(mt)
	_, err := c.ListTools()
	require.NoError(t, err)
	tool, found := c.FindTool("GET_TASK")
	assert.True(t, found)
	assert.Equal(t, "ClickUp_Get_Task", tool.Name)
}

func TestClient_FindTool_NotFound(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"tools/list": {Result: json.RawMessage(`{"tools":[{"name":"some_tool"}]}`)},
	}}
	c, _ := mcpclient.NewClient(mt)
	_, _ = c.ListTools()
	_, found := c.FindTool("nonexistent")
	assert.False(t, found)
}

func TestClient_CallTool(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"tools/call": {Result: json.RawMessage(`{"content":[{"type":"text","text":"result data"}]}`)},
	}}
	c, _ := mcpclient.NewClient(mt)
	result, err := c.CallTool("clickup_get_task", map[string]any{"task_id": "abc123"})
	require.NoError(t, err)
	assert.Len(t, result.Content, 1)
	assert.Equal(t, "result data", result.Content[0].Text)
}

func TestClient_CallTool_ServerError(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"tools/call": {Error: &mcpclient.JSONRPCError{Code: -32601, Message: "tool not found"}},
	}}
	c, _ := mcpclient.NewClient(mt)
	_, err := c.CallTool("bogus", nil)
	assert.ErrorContains(t, err, "tool not found")
}

func TestClient_Close(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{}}
	c, _ := mcpclient.NewClient(mt)
	assert.NoError(t, c.Close())
	assert.True(t, mt.closed)
}

func TestClient_IDsIncrement(t *testing.T) {
	calls := 0
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"initialize": {Result: json.RawMessage(`{}`)},
		"tools/list": {Result: json.RawMessage(`{"tools":[]}`)},
	}}
	// Wrap to track IDs
	var seenIDs []int
	origSend := mt.Send
	_ = origSend
	wrapper := &idTrackingTransport{inner: mt, ids: &seenIDs}
	_ = calls

	c, _ := mcpclient.NewClient(wrapper)
	_ = c.Initialize()
	_, _ = c.ListTools()

	assert.Equal(t, []int{1, 2}, seenIDs)
}

// idTrackingTransport wraps a transport to record request IDs.
type idTrackingTransport struct {
	inner *mockTransport
	ids   *[]int
}

func (t *idTrackingTransport) Send(req mcpclient.JSONRPCRequest) (mcpclient.JSONRPCResponse, error) {
	*t.ids = append(*t.ids, req.ID)
	return t.inner.Send(req)
}

func (t *idTrackingTransport) Close() error { return t.inner.Close() }

func TestJSONRPCError_Error(t *testing.T) {
	e := &mcpclient.JSONRPCError{Code: -32600, Message: "invalid request"}
	assert.Equal(t, "invalid request", e.Error())
}
