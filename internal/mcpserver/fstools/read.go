package fstools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// DefaultReadLines is the number of lines returned when the caller does not
	// specify a limit.
	DefaultReadLines = 200
	// MaxReadLines is the hard upper bound on lines returned in a single call.
	MaxReadLines = 2000
)

func init() {
	addRegistrar(registerReadFile)
}

// readFileLines opens path and returns up to maxLines lines starting at line
// from (1-based). Returned lines are prefixed as "N: content". The function
// always counts the entire file so that the returned totalLines value reflects
// the real length of the file even when the window is smaller.
//
// Edge-case behaviour:
//   - from < 1 is clamped to 1.
//   - maxLines <= 0 is treated as DefaultReadLines.
//   - maxLines > MaxReadLines is capped at MaxReadLines.
//   - Requesting a window that starts past EOF returns an empty string with
//     the correct totalLines count, not an error.
func readFileLines(path string, from, maxLines int) (content string, totalLines int, err error) {
	if from < 1 {
		from = 1
	}
	if maxLines <= 0 {
		maxLines = DefaultReadLines
	}
	if maxLines > MaxReadLines {
		maxLines = MaxReadLines
	}

	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase the scanner buffer to handle source files with very long lines
	// (the default 64 KB can be exceeded by generated code or minified assets).
	scanner.Buffer(make([]byte, 0, 256*1024), 512*1024)

	var sb strings.Builder
	lineNum := 0
	end := from + maxLines - 1

	for scanner.Scan() {
		lineNum++
		if lineNum >= from && lineNum <= end {
			fmt.Fprintf(&sb, "%d: %s\n", lineNum, scanner.Text())
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return "", 0, fmt.Errorf("scan %q: %w", path, scanErr)
	}

	totalLines = lineNum
	return sb.String(), totalLines, nil
}

// makeReadFileHandler returns a ToolHandlerFunc that reads a file from the
// sandbox and returns its contents as numbered lines.
func makeReadFileHandler(sb *Sandbox) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rawPath, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("missing required argument 'path': %v", err)), nil
		}

		validatedPath, err := sb.Validate(rawPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("access denied: %v", err)), nil
		}

		info, err := os.Stat(validatedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("stat %q: %v", validatedPath, err)), nil
		}
		if info.IsDir() {
			return mcp.NewToolResultError(fmt.Sprintf("path is a directory: %s", validatedPath)), nil
		}

		from := req.GetInt("from", 1)
		lines := req.GetInt("lines", DefaultReadLines)

		body, total, err := readFileLines(validatedPath, from, lines)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("read %q: %v", validatedPath, err)), nil
		}

		// Determine the actual window that was returned.
		if from < 1 {
			from = 1
		}
		end := from + lines - 1
		if end > total {
			end = total
		}
		if from > total {
			// Past EOF — report an empty window.
			from = total + 1
			end = total
		}

		header := fmt.Sprintf("[%s] (lines %d-%d of %d)\n", validatedPath, from, end, total)
		return mcp.NewToolResultText(header + body), nil
	}
}

// registerReadFile registers the read_file tool with the MCP server. The
// runner parameter is accepted but ignored because read_file uses direct file
// I/O rather than shelling out.
func registerReadFile(srv *server.MCPServer, sb *Sandbox, _ CmdRunner) {
	tool := mcp.NewTool("read_file",
		mcp.WithDescription("Read a file and return its contents as numbered lines. "+
			"Use 'from' and 'lines' to read a specific range."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Absolute or relative path to the file to read."),
		),
		mcp.WithNumber("from",
			mcp.Description("1-based line number to start reading from (default: 1)."),
		),
		mcp.WithNumber("lines",
			mcp.Description(fmt.Sprintf(
				"Maximum number of lines to return (default: %d, max: %d).",
				DefaultReadLines, MaxReadLines,
			)),
		),
	)
	srv.AddTool(tool, makeReadFileHandler(sb))
}
