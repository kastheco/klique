package clickup

import (
	"fmt"
	"regexp"
)

// clickUpTaskIDRe matches "**Source:** ClickUp <ID>" optionally followed by a URL.
var clickUpTaskIDRe = regexp.MustCompile(`\*\*Source:\*\*\s+ClickUp\s+(\S+)`)

// ParseClickUpTaskID extracts the ClickUp task ID from plan content that
// contains a "**Source:** ClickUp <ID>" line. Returns "" if not found.
func ParseClickUpTaskID(content string) string {
	m := clickUpTaskIDRe.FindStringSubmatch(content)
	if m == nil {
		return ""
	}
	// Strip a trailing parenthesised URL if present, e.g. "abc123 (https://...)"
	// The regex above captures up to whitespace, so the ID is already clean.
	return m[1]
}

// Commenter posts markdown progress comments to a ClickUp task via MCP.
type Commenter struct {
	client      MCPCaller
	workspaceID string
}

// NewCommenter creates a Commenter with the given MCP client.
func NewCommenter(client MCPCaller) *Commenter {
	return &Commenter{client: client}
}

// SetWorkspaceID sets the workspace ID to include in MCP tool calls.
// Mirrors the Importer pattern.
func (c *Commenter) SetWorkspaceID(id string) {
	c.workspaceID = id
}

// PostComment posts a markdown comment to the ClickUp task identified by taskID.
// It is nil-receiver safe: if c is nil, it returns nil immediately (no-op).
func (c *Commenter) PostComment(taskID, comment string) error {
	if c == nil {
		return nil
	}

	tool, found := c.client.FindTool("clickup_create_task_comment")
	if !found {
		return fmt.Errorf("clickup: no create_task_comment tool found in MCP server")
	}

	args := map[string]interface{}{
		"task_id":      taskID,
		"comment_text": comment,
	}
	if c.workspaceID != "" {
		args["workspace_id"] = c.workspaceID
	}

	_, err := c.client.CallTool(tool.Name, args)
	if err != nil {
		return fmt.Errorf("clickup: post comment to task %s: %w", taskID, err)
	}
	return nil
}
