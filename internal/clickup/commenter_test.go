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
