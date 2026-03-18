package fstools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func init() { addRegistrar(registerGrep) }

// MaxGrepMatches is the maximum number of grep matches the tool will return.
const MaxGrepMatches = 200

// GrepMatch holds information about a single ripgrep match.
type GrepMatch struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Text      string `json:"text"`
	MatchText string `json:"match_text"`
}

// rgLine is the top-level structure of a single rg --json output line.
type rgLine struct {
	Type string `json:"type"`
	Data rgData `json:"data"`
}

// rgData holds the "data" portion of an rg JSON event line.
type rgData struct {
	Path       rgText       `json:"path"`
	Lines      rgText       `json:"lines"`
	LineNumber int          `json:"line_number"`
	Submatches []rgSubmatch `json:"submatches"`
}

// rgText holds a text value from rg JSON output.
type rgText struct {
	Text string `json:"text"`
}

// rgSubmatch holds a single submatch within an rg match event.
type rgSubmatch struct {
	Match rgText `json:"match"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// parseRgJSON parses the NDJSON output of `rg --json` into a slice of GrepMatch.
// It ignores begin, end, summary, and context events and stops appending once
// len(matches) == MaxGrepMatches.
func parseRgJSON(data []byte) ([]GrepMatch, error) {
	matches := make([]GrepMatch, 0)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Increase buffer for long rg JSON lines (matches transport_http.go:82).
	scanner.Buffer(make([]byte, 0, 256*1024), 512*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rl rgLine
		if err := json.Unmarshal(line, &rl); err != nil {
			return nil, fmt.Errorf("parse rg JSON: %w", err)
		}

		if rl.Type != "match" {
			continue
		}

		// Hard cap: stop once we have reached MaxGrepMatches.
		if len(matches) == MaxGrepMatches {
			break
		}

		col := 1
		matchText := ""
		if len(rl.Data.Submatches) > 0 {
			col = rl.Data.Submatches[0].Start + 1
			matchText = rl.Data.Submatches[0].Match.Text
		}

		matches = append(matches, GrepMatch{
			File:      rl.Data.Path.Text,
			Line:      rl.Data.LineNumber,
			Column:    col,
			Text:      strings.TrimRight(rl.Data.Lines.Text, "\n"),
			MatchText: matchText,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan rg output: %w", err)
	}
	return matches, nil
}

// makeGrepHandler returns a ToolHandlerFunc that implements the grep tool using
// rg --json under the given sandbox and runner.
func makeGrepHandler(sb *Sandbox, runner CmdRunner) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pattern, err := req.RequireString("pattern")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("grep: %v", err)), nil
		}

		searchPath := req.GetString("path", sb.DefaultDir())
		glob := req.GetString("glob", "")
		contextLines := req.GetInt("context_lines", 0)

		resolvedPath, err := sb.Validate(searchPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("grep: %v", err)), nil
		}

		args := []string{"--json"}
		if glob != "" {
			args = append(args, "--glob", glob)
		}
		if contextLines > 0 {
			args = append(args, "-C", strconv.Itoa(contextLines))
		}
		args = append(args, pattern, resolvedPath)

		out, err := runner.Output(ctx, "rg", args...)
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
				// Exit code 1 means rg found no matches — not an error condition.
				result, encErr := mcp.NewToolResultJSON([]GrepMatch{})
				if encErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("encode grep result: %v", encErr)), nil
				}
				return result, nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("grep: rg failed: %v", err)), nil
		}

		matches, err := parseRgJSON(out)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("grep: parse output: %v", err)), nil
		}

		result, err := mcp.NewToolResultJSON(matches)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encode grep result: %v", err)), nil
		}
		return result, nil
	}
}

// registerGrep registers the grep tool with the MCP server.
func registerGrep(srv *server.MCPServer, sb *Sandbox, runner CmdRunner) {
	tool := mcp.NewTool("grep",
		mcp.WithDescription("Search file contents using ripgrep (rg). Returns structured match objects with file, line, column, and context."),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Regular expression pattern to search for"),
		),
		mcp.WithString("path",
			mcp.Description("Directory or file to search (defaults to workspace root)"),
		),
		mcp.WithString("glob",
			mcp.Description("Glob pattern to restrict which files are searched (e.g. '*.go')"),
		),
		mcp.WithNumber("context_lines",
			mcp.Description("Number of context lines to include before and after each match"),
		),
	)
	srv.AddTool(tool, makeGrepHandler(sb, runner))
}
