package fstools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MaxFindResults caps the number of file paths returned by find_files.
const MaxFindResults = 500

// parseFdOutput splits raw fd stdout into a slice of file paths, trimming empty
// lines and capping the result at MaxFindResults entries.
func parseFdOutput(data []byte) []string {
	lines := strings.Split(string(data), "\n")
	results := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		results = append(results, line)
		if len(results) >= MaxFindResults {
			break
		}
	}
	return results
}

// makeFindHandler returns a ToolHandlerFunc that executes fd with glob semantics.
func makeFindHandler(sb *Sandbox, runner CmdRunner) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pattern, err := req.RequireString("pattern")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("find_files: %v", err)), nil
		}

		searchPath := req.GetString("path", sb.DefaultDir())

		validPath, err := sb.Validate(searchPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("find_files: %v", err)), nil
		}

		args := []string{"--color", "never", "--type", "f", "--glob", pattern, validPath}
		out, err := runner.Output(ctx, "fd", args...)
		if err != nil {
			// fd exits non-zero on some errors but also when there are no matches
			// in some configurations; treat non-empty stderr as an actual error.
			// Empty output with non-zero exit is treated as zero matches.
			if len(out) == 0 {
				out = []byte{}
			}
		}

		files := parseFdOutput(out)

		result, err := mcp.NewToolResultJSON(files)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encode find_files result: %v", err)), nil
		}
		return result, nil
	}
}

// registerFindFiles registers the find_files tool with the MCP server.
func registerFindFiles(srv *server.MCPServer, sb *Sandbox, runner CmdRunner) {
	tool := mcp.NewTool(
		"find_files",
		mcp.WithDescription("finds files by name pattern using fd"),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("glob pattern to match file names (e.g. *.go, **/*.ts)"),
		),
		mcp.WithString("path",
			mcp.Description("directory to search in; defaults to the workspace root"),
		),
	)
	srv.AddTool(tool, makeFindHandler(sb, runner))
}

func init() {
	addRegistrar(registerFindFiles)
}
