package clickup

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// sourceLineRe matches "**Source:** ClickUp <ID>" optionally followed by a URL.
	sourceLineRe = regexp.MustCompile(`\*\*Source:\*\*\s+ClickUp\s+(\S+)`)
	// clickUpURLRe matches "**ClickUp:** https://app.clickup.com/t/<ID>".
	clickUpURLRe = regexp.MustCompile(`\*\*ClickUp:\*\*\s+https://app\.clickup\.com/t/(\S+)`)
)

// ParseClickUpTaskID extracts the ClickUp task ID from plan content.
// Supports two formats:
//   - "**Source:** ClickUp <ID>" (kasmos native)
//   - "**ClickUp:** https://app.clickup.com/t/<ID>" (stub plans)
//
// If the extracted ID has a "CU-" prefix it is stripped — ClickUp custom task
// IDs use this prefix but the API expects the raw alphanumeric ID.
// Returns "" if no task ID is found.
func ParseClickUpTaskID(content string) string {
	// Try the Source line first (original format).
	if m := sourceLineRe.FindStringSubmatch(content); m != nil {
		return stripCUPrefix(m[1])
	}
	// Fall back to the ClickUp URL format.
	if m := clickUpURLRe.FindStringSubmatch(content); m != nil {
		return stripCUPrefix(m[1])
	}
	return ""
}

// stripCUPrefix removes the "CU-" prefix from a ClickUp task ID if present.
func stripCUPrefix(id string) string {
	return strings.TrimPrefix(id, "CU-")
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
