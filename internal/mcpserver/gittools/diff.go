package gittools

import (
	"context"
	"fmt"
	"strings"

	"github.com/kastheco/kasmos/internal/mcpserver/fstools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// makeGitDiffHandler returns a ToolHandlerFunc that runs git diff for the
// given sandbox-validated path. When staged is true, --cached is appended.
// When file is non-empty, -- <file> is appended so filenames cannot be parsed
// as flags.
func makeGitDiffHandler(sb *fstools.Sandbox, runner fstools.CmdRunner) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", sb.DefaultDir())
		staged := req.GetBool("staged", false)
		file := req.GetString("file", "")

		resolvedPath, err := sb.Validate(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("git_diff: %v", err)), nil
		}

		args := []string{"-C", resolvedPath, "diff"}
		if staged {
			args = append(args, "--cached")
		}
		if file != "" {
			args = append(args, "--", file)
		}

		out, err := runner.Output(ctx, "git", args...)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("git_diff: git failed: %v", err)), nil
		}

		if strings.TrimSpace(string(out)) == "" {
			return mcp.NewToolResultText("no changes"), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

// registerGitDiff registers the git_diff tool with the MCP server.
func registerGitDiff(srv *server.MCPServer, sb *fstools.Sandbox, runner fstools.CmdRunner) {
	tool := mcp.NewTool(
		"git_diff",
		mcp.WithDescription("shows changes between commits, commit and working tree, etc."),
		mcp.WithString("path",
			mcp.Description("path to the git repository; defaults to the workspace root"),
		),
		mcp.WithBoolean("staged",
			mcp.Description("when true, shows staged changes (--cached); defaults to false"),
		),
		mcp.WithString("file",
			mcp.Description("restrict diff to a specific file path"),
		),
	)
	srv.AddTool(tool, makeGitDiffHandler(sb, runner))
}

func init() {
	addRegistrar(registerGitDiff)
}
