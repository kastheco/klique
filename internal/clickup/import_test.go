package clickup_test

import (
	"encoding/json"
	"strings"
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
		if strings.Contains(strings.ToLower(t.Name), strings.ToLower(sub)) {
			return t, true
		}
	}
	return mcpclient.Tool{}, false
}

func TestSearch_BareArray(t *testing.T) {
	// Legacy format: bare JSON array (some older MCP servers)
	taskJSON, _ := json.Marshal([]map[string]interface{}{
		{
			"id":     "abc",
			"name":   "Auth flow",
			"status": map[string]string{"status": "open"},
			"list":   map[string]string{"name": "Backend"},
		},
	})
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "clickup_search"}},
		callResults: map[string]*mcpclient.ToolResult{
			"clickup_search": {Content: []mcpclient.ToolContent{{Type: "text", Text: string(taskJSON)}}},
		},
	}

	importer := clickup.NewImporter(stub)
	results, err := importer.Search("auth")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "abc", results[0].ID)
	assert.Equal(t, "Auth flow", results[0].Name)
	assert.Equal(t, "open", results[0].Status)
	assert.Equal(t, "Backend", results[0].ListName)
}

func TestSearch_WrapperObject(t *testing.T) {
	// Real ClickUp MCP format: wrapper object with "results" array
	wrapperJSON, _ := json.Marshal(map[string]interface{}{
		"overview":    "Found 1 result.",
		"next_cursor": "abc123",
		"results": []map[string]interface{}{
			{
				"id":     "xyz",
				"name":   "Login bug",
				"status": "open",
				"url":    "https://app.clickup.com/t/xyz",
				"hierarchy": map[string]interface{}{
					"subcategory": map[string]interface{}{"name": "Features"},
				},
			},
		},
	})
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "clickup_search"}},
		callResults: map[string]*mcpclient.ToolResult{
			"clickup_search": {Content: []mcpclient.ToolContent{{Type: "text", Text: string(wrapperJSON)}}},
		},
	}

	importer := clickup.NewImporter(stub)
	results, err := importer.Search("login")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "xyz", results[0].ID)
	assert.Equal(t, "Login bug", results[0].Name)
	assert.Equal(t, "open", results[0].Status)
	assert.Equal(t, "https://app.clickup.com/t/xyz", results[0].URL)
	assert.Equal(t, "Features", results[0].ListName)
}

func TestSearch_EmptyResults(t *testing.T) {
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "clickup_search"}},
		callResults: map[string]*mcpclient.ToolResult{
			"clickup_search": {Content: []mcpclient.ToolContent{{Type: "text", Text: ""}}},
		},
	}

	importer := clickup.NewImporter(stub)
	results, err := importer.Search("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestSearch_NoToolFound(t *testing.T) {
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "some_other_tool"}},
	}

	importer := clickup.NewImporter(stub)
	_, err := importer.Search("test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no search tool")
}

func TestFetchTask_RealMCPResponse(t *testing.T) {
	// Real ClickUp MCP get_task response shape — status, priority, list are nested objects
	taskJSON, _ := json.Marshal(map[string]interface{}{
		"id":           "86dz50kwb",
		"name":         "conversion limiter tests",
		"description":  "Write comprehensive tests for the rate limiter",
		"text_content": "Write comprehensive tests for the rate limiter",
		"status": map[string]interface{}{
			"id":     "p90172985829_HKejkjyE",
			"status": "ready",
			"color":  "#87909e",
			"type":   "open",
		},
		"priority": map[string]interface{}{
			"id":       "3",
			"priority": "normal",
			"color":    "#6fddff",
		},
		"list": map[string]interface{}{
			"id":   "901708921002",
			"name": "features",
		},
		"url":  "https://app.clickup.com/t/86dz50kwb",
		"tags": []map[string]interface{}{},
		"custom_fields": []map[string]interface{}{
			{
				"id":    "cf1",
				"name":  "Sprint",
				"type":  "short_text",
				"value": "2026-W09",
			},
		},
	})
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "clickup_get_task"}},
		callResults: map[string]*mcpclient.ToolResult{
			"clickup_get_task": {Content: []mcpclient.ToolContent{{Type: "text", Text: string(taskJSON)}}},
		},
	}

	importer := clickup.NewImporter(stub)
	task, err := importer.FetchTask("86dz50kwb")
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, "86dz50kwb", task.ID)
	assert.Equal(t, "conversion limiter tests", task.Name)
	assert.Equal(t, "Write comprehensive tests for the rate limiter", task.Description)
	assert.Equal(t, "ready", task.Status)
	assert.Equal(t, "normal", task.Priority)
	assert.Equal(t, "features", task.ListName)
	assert.Equal(t, "https://app.clickup.com/t/86dz50kwb", task.URL)
	require.Len(t, task.CustomFields, 1)
	assert.Equal(t, "Sprint", task.CustomFields[0].Name)
	assert.Equal(t, "2026-W09", task.CustomFields[0].Value)
}

func TestFetchTask_WithSubtasks(t *testing.T) {
	// Subtasks in real MCP response are full task objects with nested status
	taskJSON, _ := json.Marshal(map[string]interface{}{
		"id":          "parent1",
		"name":        "Setup CI/CD",
		"description": "Configure CI pipeline",
		"status":      map[string]interface{}{"status": "in progress"},
		"priority":    map[string]interface{}{"priority": "high"},
		"list":        map[string]interface{}{"name": "DevOps"},
		"url":         "https://app.clickup.com/t/parent1",
		"subtasks": []map[string]interface{}{
			{
				"id":     "sub1",
				"name":   "Add Dockerfile",
				"status": map[string]interface{}{"status": "closed"},
			},
			{
				"id":     "sub2",
				"name":   "Configure GitHub Actions",
				"status": map[string]interface{}{"status": "open"},
			},
		},
	})
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "clickup_get_task"}},
		callResults: map[string]*mcpclient.ToolResult{
			"clickup_get_task": {Content: []mcpclient.ToolContent{{Type: "text", Text: string(taskJSON)}}},
		},
	}

	importer := clickup.NewImporter(stub)
	task, err := importer.FetchTask("parent1")
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, "in progress", task.Status)
	assert.Equal(t, "high", task.Priority)
	assert.Equal(t, "DevOps", task.ListName)
	require.Len(t, task.Subtasks, 2)
	assert.Equal(t, "Add Dockerfile", task.Subtasks[0].Name)
	assert.Equal(t, "closed", task.Subtasks[0].Status)
	assert.Equal(t, "Configure GitHub Actions", task.Subtasks[1].Name)
	assert.Equal(t, "open", task.Subtasks[1].Status)
}

func TestFetchTask_EmptyResponse(t *testing.T) {
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "clickup_get_task"}},
		callResults: map[string]*mcpclient.ToolResult{
			"clickup_get_task": {Content: []mcpclient.ToolContent{{Type: "text", Text: ""}}},
		},
	}

	importer := clickup.NewImporter(stub)
	_, err := importer.FetchTask("missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestFetchTask_CustomFieldTypes(t *testing.T) {
	// Custom field values can be various types — numbers, booleans, etc.
	taskJSON, _ := json.Marshal(map[string]interface{}{
		"id":     "cf_task",
		"name":   "Task with fields",
		"status": map[string]interface{}{"status": "open"},
		"url":    "https://app.clickup.com/t/cf_task",
		"custom_fields": []map[string]interface{}{
			{"id": "cf1", "name": "Story Points", "type": "number", "value": 5},
			{"id": "cf2", "name": "Sprint", "type": "short_text", "value": "2026-W09"},
			{"id": "cf3", "name": "Flagged", "type": "checkbox", "value": true},
			{"id": "cf4", "name": "Empty Field", "type": "short_text", "value": nil},
		},
	})
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "clickup_get_task"}},
		callResults: map[string]*mcpclient.ToolResult{
			"clickup_get_task": {Content: []mcpclient.ToolContent{{Type: "text", Text: string(taskJSON)}}},
		},
	}

	importer := clickup.NewImporter(stub)
	task, err := importer.FetchTask("cf_task")
	require.NoError(t, err)
	require.Len(t, task.CustomFields, 4)
	assert.Equal(t, "Story Points", task.CustomFields[0].Name)
	assert.Equal(t, "5", task.CustomFields[0].Value)
	assert.Equal(t, "Sprint", task.CustomFields[1].Name)
	assert.Equal(t, "2026-W09", task.CustomFields[1].Value)
	assert.Equal(t, "Flagged", task.CustomFields[2].Name)
	assert.Equal(t, "true", task.CustomFields[2].Value)
	assert.Equal(t, "Empty Field", task.CustomFields[3].Name)
	assert.Equal(t, "", task.CustomFields[3].Value)
}
