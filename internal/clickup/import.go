package clickup

import (
	"encoding/json"
	"fmt"

	"github.com/kastheco/kasmos/internal/mcpclient"
)

// MCPCaller is the subset of mcpclient.Client that Importer needs.
type MCPCaller interface {
	CallTool(name string, args map[string]interface{}) (*mcpclient.ToolResult, error)
	FindTool(substring string) (mcpclient.Tool, bool)
}

// Importer searches and fetches ClickUp tasks via MCP.
type Importer struct {
	client MCPCaller
}

// NewImporter creates an Importer with the given MCP client.
func NewImporter(client MCPCaller) *Importer {
	return &Importer{client: client}
}

// Search finds ClickUp tasks matching the query.
func (im *Importer) Search(query string) ([]SearchResult, error) {
	tool, found := im.client.FindTool("search")
	if !found {
		return nil, fmt.Errorf("no search tool found in MCP server")
	}

	result, err := im.client.CallTool(tool.Name, map[string]interface{}{
		"query": query,
	})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	text := extractText(result)
	if text == "" {
		return nil, nil
	}

	var results []SearchResult
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		results = nil
		var raw []map[string]interface{}
		if err2 := json.Unmarshal([]byte(text), &raw); err2 != nil {
			return nil, fmt.Errorf("parse search results: %w", err)
		}
		for _, item := range raw {
			results = append(results, parseSearchResult(item))
		}
	}

	return results, nil
}

// FetchTask gets full details for a ClickUp task by ID.
func (im *Importer) FetchTask(taskID string) (*Task, error) {
	tool, found := im.client.FindTool("get_task")
	if !found {
		return nil, fmt.Errorf("no get_task tool found in MCP server")
	}

	result, err := im.client.CallTool(tool.Name, map[string]interface{}{
		"task_id": taskID,
	})
	if err != nil {
		return nil, fmt.Errorf("fetch task: %w", err)
	}

	text := extractText(result)
	if text == "" {
		return nil, fmt.Errorf("empty response for task %s", taskID)
	}

	var task Task
	if err := json.Unmarshal([]byte(text), &task); err != nil {
		return nil, fmt.Errorf("parse task: %w", err)
	}

	return &task, nil
}

func extractText(result *mcpclient.ToolResult) string {
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text
		}
	}
	return ""
}

func parseSearchResult(item map[string]interface{}) SearchResult {
	r := SearchResult{}

	if id, ok := item["id"].(string); ok {
		r.ID = id
	}
	if name, ok := item["name"].(string); ok {
		r.Name = name
	}

	if status, ok := item["status"].(map[string]interface{}); ok {
		if s, ok := status["status"].(string); ok {
			r.Status = s
		}
	} else if status, ok := item["status"].(string); ok {
		r.Status = status
	}

	if list, ok := item["list"].(map[string]interface{}); ok {
		if name, ok := list["name"].(string); ok {
			r.ListName = name
		}
	}

	if url, ok := item["url"].(string); ok {
		r.URL = url
	}

	return r
}
