package clickup_test

import (
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMCPCaller struct {
	tools        []mcpclient.Tool
	lastToolName string
	lastArgs     map[string]interface{}
}

func (m *mockMCPCaller) CallTool(name string, args map[string]interface{}) (*mcpclient.ToolResult, error) {
	m.lastToolName = name
	m.lastArgs = args
	return &mcpclient.ToolResult{}, nil
}

func (m *mockMCPCaller) FindTool(substring string) (mcpclient.Tool, bool) {
	for _, t := range m.tools {
		if len(t.Name) >= len(substring) && containsSubstring(t.Name, substring) {
			return t, true
		}
	}
	return mcpclient.Tool{}, false
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestParseClickUpTaskID(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantID  string
	}{
		{"standard", "**Source:** ClickUp abc123 (https://app.clickup.com/t/abc123)", "abc123"},
		{"no url", "**Source:** ClickUp xyz789", "xyz789"},
		{"clickup url format", "**ClickUp:** https://app.clickup.com/t/86dzfbqz9", "86dzfbqz9"},
		{"clickup url with trailing", "**ClickUp:** https://app.clickup.com/t/86dzfbqz9\n**Workspace:** 123", "86dzfbqz9"},
		{"cu prefix source", "**Source:** ClickUp CU-abc123", "abc123"},
		{"cu prefix url", "**ClickUp:** https://app.clickup.com/t/CU-86dzfbqz9", "86dzfbqz9"},
		{"source preferred over url", "**Source:** ClickUp src123\n**ClickUp:** https://app.clickup.com/t/url456", "src123"},
		{"missing", "# Plan\n\nNo clickup here", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantID, clickup.ParseClickUpTaskID(tt.content))
		})
	}
}

func TestCommenterPostComment(t *testing.T) {
	mock := &mockMCPCaller{
		tools: []mcpclient.Tool{{Name: "clickup_create_task_comment"}},
	}
	c := clickup.NewCommenter(mock)
	err := c.PostComment("abc123", "test comment")
	require.NoError(t, err)
	assert.Equal(t, "clickup_create_task_comment", mock.lastToolName)
	assert.Equal(t, "abc123", mock.lastArgs["task_id"])
	assert.Equal(t, "test comment", mock.lastArgs["comment_text"])
}

func TestCommenterNilSafe(t *testing.T) {
	// nil commenter should be a no-op
	var c *clickup.Commenter
	err := c.PostComment("abc123", "test")
	assert.NoError(t, err)
}

// TestCommenterWorkspaceIDPropagation verifies that SetWorkspaceID causes the
// workspace_id to be included in the MCP tool call arguments.
func TestCommenterWorkspaceIDPropagation(t *testing.T) {
	mock := &mockMCPCaller{
		tools: []mcpclient.Tool{{Name: "clickup_create_task_comment"}},
	}
	c := clickup.NewCommenter(mock)
	c.SetWorkspaceID("ws-99")

	err := c.PostComment("task-abc", "progress update")
	require.NoError(t, err)
	assert.Equal(t, "ws-99", mock.lastArgs["workspace_id"],
		"workspace_id must be present in tool call args after SetWorkspaceID")
}

// TestCommenterNoWorkspaceIDByDefault verifies that workspace_id is absent from
// tool call args when SetWorkspaceID has not been called.
func TestCommenterNoWorkspaceIDByDefault(t *testing.T) {
	mock := &mockMCPCaller{
		tools: []mcpclient.Tool{{Name: "clickup_create_task_comment"}},
	}
	c := clickup.NewCommenter(mock)

	err := c.PostComment("task-abc", "no ws")
	require.NoError(t, err)
	_, hasWS := mock.lastArgs["workspace_id"]
	assert.False(t, hasWS, "workspace_id must not appear when SetWorkspaceID was not called")
}

// TestCommenterToolNotFoundError verifies that PostComment returns an error
// when the clickup_create_task_comment MCP tool is not available.
func TestCommenterToolNotFoundError(t *testing.T) {
	mock := &mockMCPCaller{
		tools: []mcpclient.Tool{}, // no tools registered
	}
	c := clickup.NewCommenter(mock)

	err := c.PostComment("task-xyz", "some comment")
	require.Error(t, err, "PostComment must return an error when MCP tool is not found")
	assert.Contains(t, err.Error(), "create_task_comment",
		"error message should mention the missing tool name")
}

// TestCommenterEmptyCommentText verifies that PostComment passes through an
// empty comment text to the MCP tool without short-circuiting.
func TestCommenterEmptyCommentText(t *testing.T) {
	mock := &mockMCPCaller{
		tools: []mcpclient.Tool{{Name: "clickup_create_task_comment"}},
	}
	c := clickup.NewCommenter(mock)

	err := c.PostComment("task-abc", "")
	require.NoError(t, err)
	assert.Equal(t, "clickup_create_task_comment", mock.lastToolName,
		"tool must be called even with an empty comment")
	assert.Equal(t, "", mock.lastArgs["comment_text"],
		"empty comment text must be forwarded to the tool")
}
