package clickup

import (
	"fmt"
	"strings"
)

// MultipleWorkspacesError is returned when the ClickUp MCP detects multiple
// workspaces and no workspace_id was provided.
type MultipleWorkspacesError struct {
	WorkspaceIDs   []string
	WorkspaceNames map[string]string // id → name, populated by FetchWorkspaceNames
}

func (e *MultipleWorkspacesError) Error() string {
	return fmt.Sprintf("multiple clickup workspaces available: %s", strings.Join(e.WorkspaceIDs, ", "))
}

// MCPServerConfig holds the detected ClickUp MCP server configuration.
type MCPServerConfig struct {
	Type    string            // "http" or "stdio"
	URL     string            // for http type
	Command string            // for stdio type
	Args    []string          // for stdio type
	Env     map[string]string // for stdio type
}

// SearchResult is a ClickUp task from search results.
type SearchResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	ListName string `json:"list_name"`
	URL      string `json:"url"`
}

// Task is a full ClickUp task with details.
type Task struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Status       string        `json:"status"`
	Priority     string        `json:"priority"`
	URL          string        `json:"url"`
	ListName     string        `json:"list_name"`
	Subtasks     []Subtask     `json:"subtasks"`
	CustomFields []CustomField `json:"custom_fields"`
}

// Subtask is a ClickUp subtask reference.
type Subtask struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// CustomField is a ClickUp custom field value.
type CustomField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
