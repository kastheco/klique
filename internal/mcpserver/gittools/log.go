package gittools

import (
	"context"
	"fmt"

	"github.com/kastheco/kasmos/internal/mcpserver/fstools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// makeGitLogHandler returns a ToolHandlerFunc that runs git log for the given
// sandbox-validated path. count is clamped to [1, 100]. oneline defaults to
// true. If file is non-empty, -- <file> is appended so filenames cannot be
// parsed as flags.
func makeGitLogHandler(sb *fstools.Sandbox, runner fstools.CmdRunner) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", sb.DefaultDir())
		count := req.GetInt("count", 20)
		oneline := req.GetBool("oneline", true)
		file := req.GetString("file", "")

		// Clamp count to [1, 100].
		if count > 100 {
			count = 100
		} else if count < 1 {
			count = 1
		}

		resolvedPath, err := sb.Validate(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("git_log: %v", err)), nil
		}

		args := []string{"-C", resolvedPath, "log", fmt.Sprintf("-%d", count)}
		if oneline {
			args = append(args, "--oneline")
		}
		if file != "" {
			args = append(args, "--", file)
		}

		out, err := runner.Output(ctx, "git", args...)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("git_log: git failed: %v", err)), nil
		}

		return mcp.NewToolResultText(string(out)), nil
	}
}

// registerGitLog registers the git_log tool with the MCP server.
func registerGitLog(srv *server.MCPServer, sb *fstools.Sandbox, runner fstools.CmdRunner) {
	tool := mcp.NewTool(
		"git_log",
		mcp.WithDescription("shows the commit log of a git repository"),
		mcp.WithString("path",
			mcp.Description("path to the git repository; defaults to the workspace root"),
		),
		mcp.WithNumber("count",
			mcp.Description("number of commits to show (1–100, defaults to 20)"),
		),
		mcp.WithBoolean("oneline",
			mcp.Description("when true (default), shows each commit on one line"),
		),
		mcp.WithString("file",
			mcp.Description("restrict log to commits touching this file path"),
		),
	)
	srv.AddTool(tool, makeGitLogHandler(sb, runner))
}

func init() {
	addRegistrar(registerGitLog)
}
