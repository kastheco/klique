// Package tasktools registers MCP tools for interacting with the kasmos task
// store. Call Register once after constructing an mcpserver.Server to attach
// task_show, task_list, task_update_content, and task_create to the server's
// tool registry.
package tasktools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/internal/mcpserver"
)

// taskShowResponse is the JSON payload returned by the task_show tool.
type taskShowResponse struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// taskListEntry is one element in the JSON array returned by the task_list tool.
type taskListEntry struct {
	Filename    string `json:"filename"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Branch      string `json:"branch"`
	Topic       string `json:"topic"`
}

// taskUpdateContentResponse is the JSON payload returned by task_update_content.
// Warning is omitted when empty (clean success with valid plan structure).
type taskUpdateContentResponse struct {
	Filename string `json:"filename"`
	Warning  string `json:"warning,omitempty"`
}

// taskCreateResponse is the JSON payload returned by task_create on success.
type taskCreateResponse struct {
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Branch   string `json:"branch"`
}

// jsonResult marshals v to compact JSON and wraps it in a text tool result.
// If marshalling fails, a tool error result is returned instead so the caller
// always gets a valid *mcp.CallToolResult.
func jsonResult(v any) *mcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err))
	}
	return mcp.NewToolResultText(string(data))
}

// Register attaches the task_show, task_list, task_update_content, and
// task_create MCP tools to srv. It must be called after NewServer and before
// the HTTP server starts accepting requests.
func Register(srv *mcpserver.Server) {
	registerTaskShow(srv)
	registerTaskList(srv)
	registerTaskUpdateContent(srv)
	registerTaskCreate(srv)
}

// registerTaskShow adds the task_show tool. The tool accepts a "filename"
// argument (with or without .md suffix), loads the task from the store, and
// returns {"filename":"...","content":"..."} on success.
//
// Error results (IsError=true) are returned for:
//   - no store configured
//   - required argument missing
//   - task not found
//   - no content stored for the task
func registerTaskShow(srv *mcpserver.Server) {
	tool := mcp.NewTool("task_show",
		mcp.WithDescription("Get the content of a task by filename"),
		mcp.WithString("filename",
			mcp.Required(),
			mcp.Description("Task filename (with or without .md extension)"),
		),
	)
	srv.MCPServer().AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if srv.Store() == nil {
			return mcp.NewToolResultError("task store not configured"), nil
		}

		filename, err := req.RequireString("filename")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		filename = strings.TrimSuffix(filename, ".md")

		ps, err := taskstate.Load(srv.Store(), srv.Project(), "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("load task state: %v", err)), nil
		}

		if _, ok := ps.Entry(filename); !ok {
			return mcp.NewToolResultError(fmt.Sprintf("task not found: %s", filename)), nil
		}

		content, err := ps.GetContent(filename)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get content for %s: %v", filename, err)), nil
		}

		if strings.TrimSpace(content) == "" {
			return mcp.NewToolResultError(fmt.Sprintf("no content stored for %s", filename)), nil
		}

		return jsonResult(taskShowResponse{Filename: filename, Content: content}), nil
	})
}

// registerTaskList adds the task_list tool. An optional "status" argument
// filters results to tasks with that status. When the filter is empty, cancelled
// tasks are hidden (mirroring `kas task list` behaviour). Results are sorted by
// filename because taskstate.TaskState.List() guarantees lexicographic order.
func registerTaskList(srv *mcpserver.Server) {
	tool := mcp.NewTool("task_list",
		mcp.WithDescription("List tasks, optionally filtered by status. Cancelled tasks are hidden when no filter is given."),
		mcp.WithString("status",
			mcp.Description("Filter by status (ready, planning, implementing, reviewing, done, cancelled). Leave empty to hide cancelled tasks."),
		),
	)
	srv.MCPServer().AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if srv.Store() == nil {
			return mcp.NewToolResultError("task store not configured"), nil
		}

		statusFilter := req.GetString("status", "")

		ps, err := taskstate.Load(srv.Store(), srv.Project(), "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("load task state: %v", err)), nil
		}

		entries := make([]taskListEntry, 0)
		for _, info := range ps.List() {
			if statusFilter != "" && string(info.Status) != statusFilter {
				continue
			}
			if statusFilter == "" && info.Status == taskstate.StatusCancelled {
				continue
			}
			entries = append(entries, taskListEntry{
				Filename:    info.Filename,
				Status:      string(info.Status),
				Description: info.Description,
				Branch:      info.Branch,
				Topic:       info.Topic,
			})
		}

		return jsonResult(entries), nil
	})
}

// registerTaskUpdateContent adds the task_update_content tool. The tool requires
// "filename" (with or without .md suffix) and "content" (non-empty markdown).
// It mirrors the warning path from cmd/task.go: a parse failure after a
// successful store write returns IsError=false with a "warning" field; hard
// errors (task not found, store write failures) return IsError=true.
func registerTaskUpdateContent(srv *mcpserver.Server) {
	tool := mcp.NewTool("task_update_content",
		mcp.WithDescription("Replace the stored plan content for a task"),
		mcp.WithString("filename",
			mcp.Required(),
			mcp.Description("Task filename (with or without .md extension)"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("New markdown content for the task"),
		),
	)
	srv.MCPServer().AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if srv.Store() == nil {
			return mcp.NewToolResultError("task store not configured"), nil
		}

		filename, err := req.RequireString("filename")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		filename = strings.TrimSuffix(filename, ".md")

		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if strings.TrimSpace(content) == "" {
			return mcp.NewToolResultError("no content provided; pipe plan content via stdin: cat plan.md | kas task update-content <plan>"), nil
		}

		ps, err := taskstate.Load(srv.Store(), srv.Project(), "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("load task state: %v", err)), nil
		}

		var warn *taskstate.IngestWarning
		if err := ps.IngestContent(filename, content); err != nil {
			if errors.As(err, &warn) {
				// Content was stored successfully; only metadata parsing failed.
				// Mirror cmd/task.go warning path: return success with warning field.
				return jsonResult(taskUpdateContentResponse{
					Filename: filename,
					Warning:  warn.Error(),
				}), nil
			}
			return mcp.NewToolResultError(fmt.Errorf("update content for %s: %w", filename, err).Error()), nil
		}

		return jsonResult(taskUpdateContentResponse{Filename: filename}), nil
	})
}

// registerTaskCreate adds the task_create tool. The tool requires "name" and
// accepts optional "description", "branch", "topic", and "content". It mirrors
// cmd/task.go executeTaskCreate: branch defaults to "plan/<name>"; when content
// is non-empty CreateWithContent is used, otherwise Create is used. IngestContent
// is NOT called here, matching the CLI behaviour where create and ingest are
// separate operations.
func registerTaskCreate(srv *mcpserver.Server) {
	tool := mcp.NewTool("task_create",
		mcp.WithDescription("Create a new task entry in the store"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Task name/slug (used as filename; .md is not appended)"),
		),
		mcp.WithString("description",
			mcp.Description("Human-readable task description"),
		),
		mcp.WithString("branch",
			mcp.Description("Git branch name (default: plan/<name>)"),
		),
		mcp.WithString("topic",
			mcp.Description("Topic group for the task"),
		),
		mcp.WithString("content",
			mcp.Description("Initial plan content (markdown); stored but not parsed"),
		),
	)
	srv.MCPServer().AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if srv.Store() == nil {
			return mcp.NewToolResultError("task store not configured"), nil
		}

		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		description := req.GetString("description", "")
		branch := req.GetString("branch", "")
		topic := req.GetString("topic", "")
		content := req.GetString("content", "")

		if branch == "" {
			branch = "plan/" + name
		}

		ps, err := taskstate.Load(srv.Store(), srv.Project(), "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("load task state: %v", err)), nil
		}

		createdAt := time.Now()
		if content != "" {
			if err := ps.CreateWithContent(name, description, branch, topic, createdAt, content); err != nil {
				return mcp.NewToolResultError(fmt.Errorf("create task %s: %w", name, err).Error()), nil
			}
		} else {
			if err := ps.Create(name, description, branch, topic, createdAt); err != nil {
				return mcp.NewToolResultError(fmt.Errorf("create task %s: %w", name, err).Error()), nil
			}
		}

		return jsonResult(taskCreateResponse{
			Filename: name,
			Status:   "ready",
			Branch:   branch,
		}), nil
	})
}
