package instancetools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func init() {
	addRegistrar(registerInstancePause)
}

// makeInstancePauseHandler returns a ToolHandlerFunc that pauses a running agent
// instance. It kills the tmux session (best-effort), removes the git worktree
// (best-effort when repo/worktree metadata exists), and authoritatively updates
// the stored instance state to paused with an empty WorktreePath.
//
// Unlike the CLI pause command, this handler intentionally skips the
// ensureCleanWorktree check: MCP tool callers are agents that may not be in a
// position to commit changes first, and the lighter-weight behaviour is sufficient
// for orchestration use.
func makeInstancePauseHandler(loadState StateLoader, runner CmdRunner) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("missing required argument 'title': %v", err)), nil
		}

		records, err := loadRecords(loadState)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_pause: load instances: %v", err)), nil
		}

		rec, err := findRecord(records, title)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_pause: %v", err)), nil
		}

		if err := validateAction(rec, "pause"); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_pause: %v", err)), nil
		}

		// Kill the tmux session using the kas_-prefixed name (best-effort).
		_ = runner.Run(ctx, "tmux", "kill-session", "-t", kasTmuxName(rec.Title))

		// Remove the worktree (best-effort) when repo and worktree metadata are both set.
		if rec.Worktree.RepoPath != "" && rec.Worktree.WorktreePath != "" {
			_ = runner.Run(ctx, "git", "-C", rec.Worktree.RepoPath,
				"worktree", "remove", "--force", rec.Worktree.WorktreePath)
			_ = runner.Run(ctx, "git", "-C", rec.Worktree.RepoPath, "worktree", "prune")
		}

		// Update state: mark as paused and clear the worktree path. This is authoritative.
		if err := updateRecord(loadState, rec.Title, func(r *instanceRecord) error {
			r.Status = instancePaused
			r.Worktree.WorktreePath = ""
			return nil
		}); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("instance_pause: update state: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("paused: %s", rec.Title)), nil
	}
}

// registerInstancePause registers the instance_pause tool with the MCP server.
func registerInstancePause(srv *server.MCPServer, loadState StateLoader, runner CmdRunner, _ string) {
	tool := mcp.NewTool(
		"instance_pause",
		mcp.WithDescription("pause a running agent instance: kills its tmux session and removes the git worktree while preserving the branch for later resume"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("title of the instance to pause"),
		),
	)
	srv.AddTool(tool, makeInstancePauseHandler(loadState, runner))
}
