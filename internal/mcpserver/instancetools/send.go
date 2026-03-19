package instancetools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func init() {
	addRegistrar(registerInstanceSend)
}

// makeInstanceSendHandler returns a ToolHandlerFunc that sends a prompt to a
// running agent instance via tmux send-keys. Both "title" and "prompt"
// arguments are required; missing either returns a tool error immediately.
func makeInstanceSendHandler(loadState StateLoader, runner CmdRunner) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("missing required argument 'title': %v", err)), nil
		}

		prompt, err := req.RequireString("prompt")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("missing required argument 'prompt': %v", err)), nil
		}

		records, err := loadRecords(loadState)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_send: load instances: %v", err)), nil
		}

		rec, err := findRecord(records, title)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_send: %v", err)), nil
		}

		if err := validateAction(rec, "send"); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_send: %v", err)), nil
		}

		// Use kas_-prefixed session name to match how the session was created.
		sessionName := kasTmuxName(rec.Title)
		if err := runner.Run(ctx, "tmux", "send-keys", "-t", sessionName, prompt, "Enter"); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("send keys: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("sent to %s", rec.Title)), nil
	}
}

// registerInstanceSend registers the instance_send tool with the MCP server.
func registerInstanceSend(srv *server.MCPServer, loadState StateLoader, runner CmdRunner, _ string) {
	tool := mcp.NewTool(
		"instance_send",
		mcp.WithDescription("send a prompt to a running agent instance via tmux send-keys"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("title of the instance to send the prompt to"),
		),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("prompt text to send to the instance"),
		),
	)
	srv.AddTool(tool, makeInstanceSendHandler(loadState, runner))
}
