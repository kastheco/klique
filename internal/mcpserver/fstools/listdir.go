package fstools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// DefaultListDepth is the default directory traversal depth for list_dir.
	DefaultListDepth = 2
	// MaxListDepth is the maximum allowed traversal depth for list_dir.
	MaxListDepth = 10
	// MaxListEntries is the maximum number of entries returned by list_dir.
	MaxListEntries = 500
)

// DirEntry represents a single filesystem entry returned by list_dir.
type DirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// parseFdListOutput parses line-separated fd output into a slice of DirEntry.
// Each non-blank line is expected to be an absolute path. Entries are capped
// at MaxListEntries. Paths that cannot be stat'd are silently skipped.
// Directory sizes are always reported as 0 for determinism.
// Name is computed relative to baseDir; if the relation cannot be computed,
// the raw path is used.
func parseFdListOutput(data []byte, baseDir string) []DirEntry {
	lines := strings.Split(string(data), "\n")
	entries := make([]DirEntry, 0, len(lines))
	for _, line := range lines {
		rawPath := strings.TrimSpace(line)
		if rawPath == "" {
			continue
		}
		if len(entries) >= MaxListEntries {
			break
		}
		info, err := os.Stat(rawPath)
		if err != nil {
			continue
		}
		name, err := filepath.Rel(baseDir, rawPath)
		if err != nil {
			name = rawPath
		}
		size := int64(0)
		if !info.IsDir() {
			size = info.Size()
		}
		entries = append(entries, DirEntry{
			Name:  name,
			IsDir: info.IsDir(),
			Size:  size,
		})
	}
	return entries
}

// makeListDirHandler returns a ToolHandlerFunc that lists directory contents
// using fd, restricted to paths allowed by sb.
func makeListDirHandler(sb *Sandbox, runner CmdRunner) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		searchPath := req.GetString("path", "")
		if searchPath == "" {
			return mcp.NewToolResultError("list_dir: path is required"), nil
		}

		depth := req.GetInt("depth", DefaultListDepth)
		if depth < 1 {
			depth = 1
		}
		if depth > MaxListDepth {
			depth = MaxListDepth
		}

		validPath, err := sb.Validate(searchPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list_dir: %v", err)), nil
		}

		info, err := os.Stat(validPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list_dir: stat %q: %v", validPath, err)), nil
		}
		if !info.IsDir() {
			return mcp.NewToolResultError(fmt.Sprintf("list_dir: %q is not a directory", validPath)), nil
		}

		args := []string{"--color", "never", "--max-depth", strconv.Itoa(depth), ".", validPath}
		out, err := runner.Output(ctx, "fd", args...)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list_dir: fd: %v", err)), nil
		}

		entries := parseFdListOutput(out, validPath)
		result, err := mcp.NewToolResultJSON(entries)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encode list_dir result: %v", err)), nil
		}
		return result, nil
	}
}

// registerListDir registers the list_dir tool with srv.
func registerListDir(srv *server.MCPServer, sb *Sandbox, runner CmdRunner) {
	tool := mcp.NewTool("list_dir",
		mcp.WithDescription("List directory contents using fd (respects .gitignore). Returns JSON array of {name, is_dir, size}."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Absolute path to the directory to list."),
		),
		mcp.WithNumber("depth",
			mcp.Description(fmt.Sprintf("Traversal depth (1-%d, default %d).", MaxListDepth, DefaultListDepth)),
		),
	)
	srv.AddTool(tool, makeListDirHandler(sb, runner))
}

func init() {
	addRegistrar(registerListDir)
}
