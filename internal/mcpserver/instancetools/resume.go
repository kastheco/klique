package instancetools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func init() {
	addRegistrar(registerInstanceResume)
}

// buildResumeProgram reconstructs the tmux program command string for a resumed
// instance. It mirrors the env-var and flag injection performed by
// cmd.buildResumeCommand and session/tmux.TmuxSession.Start so that the resumed
// agent is indistinguishable from a freshly started one.
func buildResumeProgram(rec instanceRecord, worktreePath string) string {
	program := rec.Program

	// Append --dangerously-skip-permissions for Claude programs if originally enabled.
	if rec.SkipPermissions && strings.HasSuffix(program, "claude") {
		program += " --dangerously-skip-permissions"
	}

	// Append --agent flag for typed roles (planner, coder, reviewer, fixer).
	if rec.AgentType != "" && !strings.Contains(program, "--agent") {
		program += " --agent " + rec.AgentType
	}

	// Append opencode log redirection so debug logs are preserved.
	// Use rec.Program (the unmodified base) for the suffix check, matching
	// TmuxSession.Start() which uses t.program — the local `program` variable
	// may already have --agent appended, changing the suffix.
	if strings.HasSuffix(rec.Program, "opencode") {
		logDir := filepath.Join(worktreePath, ".kasmos", "logs")
		if err := os.MkdirAll(logDir, 0o755); err == nil {
			logFile := filepath.Join(logDir, kasTmuxName(rec.Title)+".log")
			program += " --print-logs 2>>'" + logFile + "'"
		}
	}

	// Prepend KASMOS_MANAGED=1 so the agent knows it is managed by kasmos.
	program = "KASMOS_MANAGED=1 " + program

	// Prepend task identity env vars for parallel wave execution.
	if rec.TaskNumber > 0 {
		program = fmt.Sprintf("KASMOS_TASK=%d KASMOS_WAVE=%d KASMOS_PEERS=%d %s",
			rec.TaskNumber, rec.WaveNumber, rec.PeerCount, program)
	}

	return program
}

// makeInstanceResumeHandler returns a ToolHandlerFunc that resumes a paused
// agent instance. It re-adds the git worktree on the preserved branch, starts
// a new tmux session, and updates the instance state to running.
//
// The handler returns a tool error (not a Go error) for all user-facing failures
// so that MCP callers receive a structured error response.
func makeInstanceResumeHandler(loadState StateLoader, runner CmdRunner) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("missing required argument 'title': %v", err)), nil
		}

		records, err := loadRecords(loadState)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_resume: load instances: %v", err)), nil
		}

		rec, err := findRecord(records, title)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_resume: %v", err)), nil
		}

		if err := validateAction(rec, "resume"); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_resume: %v", err)), nil
		}

		// Both RepoPath and BranchName must be present to recreate the worktree.
		if rec.Worktree.RepoPath == "" || rec.Worktree.BranchName == "" {
			return mcp.NewToolResultError(fmt.Sprintf(
				"instance_resume: no stored worktree metadata for %q", rec.Title)), nil
		}

		// Use the stored WorktreePath when available, otherwise fall back to rec.Path.
		worktreePath := rec.Worktree.WorktreePath
		if worktreePath == "" {
			worktreePath = rec.Path
		}

		// Re-add the git worktree on the preserved branch.
		if err := runner.Run(ctx, "git", "-C", rec.Worktree.RepoPath,
			"worktree", "add", worktreePath, rec.Worktree.BranchName); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_resume: recreate worktree: %v", err)), nil
		}

		// Reconstruct the full program command (env vars + flags) matching the original session.
		program := buildResumeProgram(rec, worktreePath)

		// Start a new detached tmux session using the kas_-prefixed name.
		sessionName := kasTmuxName(rec.Title)
		if err := runner.Run(ctx, "tmux", "new-session", "-d", "-s", sessionName,
			"-c", worktreePath, program); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_resume: start tmux session: %v", err)), nil
		}

		// Update state: mark as running and store the restored worktree path.
		if err := updateRecord(loadState, rec.Title, func(r *instanceRecord) error {
			r.Status = instanceRunning
			r.Worktree.WorktreePath = worktreePath
			return nil
		}); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_resume: update state: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("resumed: %s", rec.Title)), nil
	}
}

// registerInstanceResume registers the instance_resume tool with the MCP server.
func registerInstanceResume(srv *server.MCPServer, loadState StateLoader, runner CmdRunner, _ string) {
	tool := mcp.NewTool(
		"instance_resume",
		mcp.WithDescription("resume a paused agent instance: re-adds the git worktree on the preserved branch and starts a new tmux session"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("title of the paused instance to resume"),
		),
	)
	srv.AddTool(tool, makeInstanceResumeHandler(loadState, runner))
}
