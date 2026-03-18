package gittools

import (
	"context"
	"fmt"

	"github.com/kastheco/kasmos/internal/mcpserver/fstools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// makeGitStatusHandler returns a ToolHandlerFunc that runs git status
// for the given sandbox-validated path.
func makeGitStatusHandler(sb *fstools.Sandbox, runner fstools.CmdRunner) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", sb.DefaultDir())

		resolvedPath, err := sb.Validate(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("git_status: %v", err)), nil
		}

		args := []string{"-C", resolvedPath, "status", "--short", "--branch"}
		out, err := runner.Output(ctx, "git", args...)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("git_status: git failed: %v", err)), nil
		}

		return mcp.NewToolResultText(string(out)), nil
	}
}

// registerGitStatus registers the git_status tool with the MCP server.
func registerGitStatus(srv *server.MCPServer, sb *fstools.Sandbox, runner fstools.CmdRunner) {
	tool := mcp.NewTool(
		"git_status",
		mcp.WithDescription("shows the working tree status of a git repository"),
		mcp.WithString("path",
			mcp.Description("path to the git repository; defaults to the workspace root"),
		),
	)
	srv.AddTool(tool, makeGitStatusHandler(sb, runner))
}

func init() {
	addRegistrar(registerGitStatus)
}
