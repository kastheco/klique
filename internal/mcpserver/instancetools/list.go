package instancetools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func init() {
	addRegistrar(registerInstanceList)
}

// instanceListEntry is the JSON-serialisable view of an instance record that
// the instance_list tool returns. It mirrors cmd/instance.go:203-211 so that
// callers get an identical shape regardless of whether they use the CLI or the
// MCP tool.
type instanceListEntry struct {
	Title     string `json:"title"`
	Status    string `json:"status"`
	Branch    string `json:"branch"`
	Program   string `json:"program"`
	TaskFile  string `json:"task_file,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// makeInstanceListHandler returns a ToolHandlerFunc that lists all instances.
// It accepts an optional "status" argument to filter by status label.
func makeInstanceListHandler(loadState StateLoader) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		records, err := loadRecords(loadState)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_list: load instances: %v", err)), nil
		}

		// Optional status filter.
		statusFilter := req.GetString("status", "")
		if statusFilter != "" {
			filtered := records[:0]
			for _, r := range records {
				if statusLabel(r.Status) == statusFilter {
					filtered = append(filtered, r)
				}
			}
			records = filtered
		}

		entries := make([]instanceListEntry, 0, len(records))
		for _, r := range records {
			var createdAt string
			if !r.CreatedAt.IsZero() {
				createdAt = r.CreatedAt.Format(time.RFC3339)
			}
			entries = append(entries, instanceListEntry{
				Title:     r.Title,
				Status:    statusLabel(r.Status),
				Branch:    r.Branch,
				Program:   r.Program,
				TaskFile:  r.TaskFile,
				AgentType: r.AgentType,
				CreatedAt: createdAt,
			})
		}

		result, err := mcp.NewToolResultJSON(entries)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encode instance_list result: %v", err)), nil
		}
		return result, nil
	}
}

// registerInstanceList registers the instance_list tool with the MCP server.
func registerInstanceList(srv *server.MCPServer, loadState StateLoader, _ CmdRunner, _ string) {
	tool := mcp.NewTool(
		"instance_list",
		mcp.WithDescription("list all kasmos agent instances; returns a JSON array of instance records"),
		mcp.WithString("status",
			mcp.Description("optional status filter: running, ready, loading, or paused"),
		),
	)
	srv.AddTool(tool, makeInstanceListHandler(loadState))
}
