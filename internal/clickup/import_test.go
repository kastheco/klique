package clickup_test

import (
	"encoding/json"
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubMCPClient struct {
	callResults map[string]*mcpclient.ToolResult
	tools       []mcpclient.Tool
}

func (s *stubMCPClient) ListTools() ([]mcpclient.Tool, error) { return s.tools, nil }

func (s *stubMCPClient) CallTool(name string, args map[string]interface{}) (*mcpclient.ToolResult, error) {
	if r, ok := s.callResults[name]; ok {
		return r, nil
	}
	return &mcpclient.ToolResult{}, nil
}

func (s *stubMCPClient) FindTool(sub string) (mcpclient.Tool, bool) {
	for _, t := range s.tools {
		if contains(t.Name, sub) {
			return t, true
		}
	}
	return mcpclient.Tool{}, false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && stringContains(s, sub)))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSearch(t *testing.T) {
	taskJSON, _ := json.Marshal([]map[string]interface{}{
		{"id": "abc", "name": "Auth flow", "status": map[string]string{"status": "open"}, "list": map[string]string{"name": "Backend"}},
	})
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "clickup_search_tasks"}},
		callResults: map[string]*mcpclient.ToolResult{
			"clickup_search_tasks": {Content: []mcpclient.ToolContent{{Type: "text", Text: string(taskJSON)}}},
		},
	}

	importer := clickup.NewImporter(stub)
	results, err := importer.Search("auth")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "abc", results[0].ID)
}
