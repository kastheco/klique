package clickup

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/kastheco/kasmos/internal/mcpclient"
)

// MCPCaller is the subset of mcpclient.Client that Importer needs.
type MCPCaller interface {
	CallTool(name string, args map[string]interface{}) (*mcpclient.ToolResult, error)
	FindTool(substring string) (mcpclient.Tool, bool)
}

// Importer searches and fetches ClickUp tasks via MCP.
type Importer struct {
	client      MCPCaller
	workspaceID string
}

// NewImporter creates an Importer with the given MCP client.
func NewImporter(client MCPCaller) *Importer {
	return &Importer{client: client}
}

// SetWorkspaceID sets the workspace ID to include in MCP tool calls.
// Required when the user has multiple ClickUp workspaces.
func (im *Importer) SetWorkspaceID(id string) {
	im.workspaceID = id
}

// FetchWorkspaceNames resolves workspace IDs to human-readable names by
// calling clickup_get_workspace_hierarchy for each ID. Returns a map of
// id → name. IDs that fail to resolve are omitted from the map.
func (im *Importer) FetchWorkspaceNames(ids []string) map[string]string {
	tool, found := im.client.FindTool("clickup_get_workspace_hierarchy")
	if !found {
		return nil
	}
	names := make(map[string]string, len(ids))
	for _, id := range ids {
		result, err := im.client.CallTool(tool.Name, map[string]interface{}{
			"workspace_id": id,
			"max_depth":    0, // spaces only — we just need the workspace name
		})
		if err != nil {
			continue
		}
		text := extractText(result)
		if text == "" {
			continue
		}
		if name := parseWorkspaceName(text, id); name != "" {
			names[id] = name
		}
	}
	return names
}

// parseWorkspaceName extracts the workspace name from a clickup_get_workspace_hierarchy
// response. The response typically contains a "workspace_name" field or a structured
// header with the workspace name.
func parseWorkspaceName(text, id string) string {
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return ""
	}
	// Try "workspace_name" (common in MCP responses)
	if name := getString(resp, "workspace_name"); name != "" {
		return name
	}
	// Try "name" directly
	if name := getString(resp, "name"); name != "" {
		return name
	}
	// Try nested workspace object
	if ws, ok := resp["workspace"].(map[string]interface{}); ok {
		if name := getString(ws, "name"); name != "" {
			return name
		}
	}
	return ""
}

// Search finds ClickUp tasks matching the query.
func (im *Importer) Search(query string) ([]SearchResult, error) {
	tool, found := im.client.FindTool("clickup_search")
	if !found {
		return nil, fmt.Errorf("no search tool found in MCP server")
	}

	// ClickUp MCP search uses the "keywords" parameter.
	args := map[string]interface{}{
		"keywords": query,
	}
	if im.workspaceID != "" {
		args["workspace_id"] = im.workspaceID
	}

	result, err := im.client.CallTool(tool.Name, args)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	text := extractText(result)
	if text == "" {
		return nil, nil
	}

	// Check for the multiple-workspaces error before parsing results.
	if mwErr := checkMultipleWorkspacesError(text); mwErr != nil {
		return nil, mwErr
	}

	// The ClickUp MCP may return:
	//   1. A wrapper object {"overview":"...", "results":[...], "next_cursor":"..."}
	//   2. A bare JSON array of tasks
	// Extract the results array from whichever format we receive.
	raw := []byte(text)

	var wrapper struct {
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Results) >= 2 && wrapper.Results[0] == '[' {
		raw = wrapper.Results
	}

	// If raw is still an object (not an array), the response is unrecognized.
	if len(raw) > 0 && raw[0] == '{' {
		return nil, fmt.Errorf("parse search results: unexpected object response (expected array): %.200s", string(raw))
	}

	// Always use the map-based parser to handle all field name variants
	// (hierarchy.subcategory.name, status as string or object, etc.)
	var items []map[string]interface{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("parse search results: %w", err)
	}
	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		results = append(results, parseSearchResult(item))
	}

	return results, nil
}

// FetchTask gets full details for a ClickUp task by ID.
func (im *Importer) FetchTask(taskID string) (*Task, error) {
	tool, found := im.client.FindTool("clickup_get_task")
	if !found {
		return nil, fmt.Errorf("no get_task tool found in MCP server")
	}

	args := map[string]interface{}{
		"task_id":  taskID,
		"subtasks": true,
	}
	if im.workspaceID != "" {
		args["workspace_id"] = im.workspaceID
	}

	result, err := im.client.CallTool(tool.Name, args)
	if err != nil {
		return nil, fmt.Errorf("fetch task: %w", err)
	}

	text := extractText(result)
	if text == "" {
		return nil, fmt.Errorf("empty response for task %s", taskID)
	}

	// Parse into a generic map first — the ClickUp MCP returns nested objects
	// for status, priority, list, subtasks, and custom_fields that don't match
	// flat string fields.
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse task: %w", err)
	}

	return parseTask(raw), nil
}

func extractText(result *mcpclient.ToolResult) string {
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text
		}
	}
	return ""
}

// parseTask extracts a Task from a generic map, handling nested ClickUp MCP
// response objects (status, priority, list are objects, not flat strings).
func parseTask(raw map[string]interface{}) *Task {
	t := &Task{}

	t.ID = getString(raw, "id")
	t.Name = getString(raw, "name")
	t.Description = getString(raw, "description")
	t.URL = getString(raw, "url")

	// status: {"status": "ready", ...} or flat string
	t.Status = getNestedString(raw, "status", "status")

	// priority: {"priority": "normal", ...} or flat string
	t.Priority = getNestedString(raw, "priority", "priority")

	// list: {"name": "features", ...}
	t.ListName = getNestedString(raw, "list", "name")

	// subtasks: array of task-like objects with nested status
	if subs, ok := raw["subtasks"].([]interface{}); ok {
		for _, s := range subs {
			if sub, ok := s.(map[string]interface{}); ok {
				t.Subtasks = append(t.Subtasks, Subtask{
					Name:   getString(sub, "name"),
					Status: getNestedString(sub, "status", "status"),
				})
			}
		}
	}

	// custom_fields: array of {name, value, type, ...} where value can be any type
	if fields, ok := raw["custom_fields"].([]interface{}); ok {
		for _, f := range fields {
			if field, ok := f.(map[string]interface{}); ok {
				name := getString(field, "name")
				value := stringifyValue(field["value"])
				if name != "" {
					t.CustomFields = append(t.CustomFields, CustomField{
						Name:  name,
						Value: value,
					})
				}
			}
		}
	}

	return t
}

func parseSearchResult(item map[string]interface{}) SearchResult {
	r := SearchResult{}

	r.ID = getString(item, "id")
	r.Name = getString(item, "name")
	r.URL = getString(item, "url")

	// status: nested object {"status": "open"} or flat string
	r.Status = getNestedString(item, "status", "status")

	// list: nested object {"name": "Backend"}
	r.ListName = getNestedString(item, "list", "name")

	// ClickUp MCP search returns hierarchy.subcategory.name as the list name
	if r.ListName == "" {
		if hier, ok := item["hierarchy"].(map[string]interface{}); ok {
			r.ListName = getNestedString(hier, "subcategory", "name")
		}
	}

	return r
}

// getString extracts a string value from a map, returning "" if missing or wrong type.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// getNestedString handles fields that can be either a flat string or a nested
// object with an inner key. e.g. status can be "open" or {"status": "open"}.
func getNestedString(m map[string]interface{}, outerKey, innerKey string) string {
	v, ok := m[outerKey]
	if !ok {
		return ""
	}
	// Try as nested object first (most common in MCP responses).
	if obj, ok := v.(map[string]interface{}); ok {
		if s, ok := obj[innerKey].(string); ok {
			return s
		}
		return ""
	}
	// Fall back to flat string.
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// multiWorkspaceRe matches workspace IDs in the ClickUp MCP multiple-workspaces error.
var multiWorkspaceRe = regexp.MustCompile(`Available workspaces:\s*([\d,\s]+)`)

// checkMultipleWorkspacesError detects the ClickUp MCP "multiple workspaces"
// error and returns a typed error with the parsed workspace IDs.
func checkMultipleWorkspacesError(text string) *MultipleWorkspacesError {
	// The MCP returns a JSON object: {"error":"Multiple workspaces available..."}
	var errObj struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(text), &errObj); err != nil || errObj.Error == "" {
		return nil
	}
	if !strings.Contains(errObj.Error, "Multiple workspaces") {
		return nil
	}

	m := multiWorkspaceRe.FindStringSubmatch(errObj.Error)
	if m == nil {
		return &MultipleWorkspacesError{}
	}

	parts := strings.Split(m[1], ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			ids = append(ids, id)
		}
	}

	return &MultipleWorkspacesError{WorkspaceIDs: ids}
}

// stringifyValue converts an arbitrary JSON value to a string representation.
// Handles string, float64 (JSON numbers), bool, and nil.
func stringifyValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Avoid trailing .0 for integers
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		// For complex types (arrays, objects), marshal to JSON string.
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
