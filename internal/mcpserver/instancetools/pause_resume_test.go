package instancetools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// In-memory StateManager for mutation tests
// ---------------------------------------------------------------------------

// mockStateManager is an in-memory config.StateManager implementation used for
// tests that exercise state mutation (e.g. pause/resume). It does not write to
// disk, so tests remain hermetic.
type mockStateManager struct {
	data json.RawMessage
}

func (m *mockStateManager) SaveInstances(instancesJSON json.RawMessage) error {
	m.data = instancesJSON
	return nil
}

func (m *mockStateManager) GetInstances() json.RawMessage {
	return m.data
}

func (m *mockStateManager) DeleteAllInstances() error {
	m.data = json.RawMessage("[]")
	return nil
}

func (m *mockStateManager) GetHelpScreensSeen() uint32        { return 0 }
func (m *mockStateManager) SetHelpScreensSeen(_ uint32) error { return nil }

// seedMutable returns a StateLoader backed by an in-memory mockStateManager.
// Unlike seedInstances, mutations via SaveInstances are visible on subsequent
// loadState() calls without touching disk.
func seedMutable(records ...instanceRecord) StateLoader {
	state := &mockStateManager{}
	raw, _ := json.Marshal(records)
	state.data = json.RawMessage(raw)
	return func() config.StateManager { return state }
}

// ---------------------------------------------------------------------------
// Helpers for capturing Run invocations
// ---------------------------------------------------------------------------

// captureRuns returns a mockRunner that records each Run invocation as a
// []string where element 0 is the binary name and the rest are args.
func captureRuns() (*mockRunner, *[][]string) {
	var calls [][]string
	runner := &mockRunner{
		runFn: func(_ context.Context, name string, args ...string) error {
			call := append([]string{name}, args...)
			calls = append(calls, call)
			return nil
		},
	}
	return runner, &calls
}

// hasCall reports whether calls contains an invocation matching all of wantArgs exactly.
func hasCall(calls [][]string, wantArgs ...string) bool {
	for _, call := range calls {
		if len(call) != len(wantArgs) {
			continue
		}
		match := true
		for i, a := range wantArgs {
			if call[i] != a {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// anyCallContains reports whether any call in calls, when joined with spaces,
// contains the given substring.
func anyCallContains(calls [][]string, substr string) bool {
	for _, call := range calls {
		if strings.Contains(strings.Join(call, " "), substr) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// instance_pause tests
// ---------------------------------------------------------------------------

// TestInstancePause_Success verifies that pausing a running instance with
// worktree metadata issues the expected tmux and git commands, then updates
// the stored record to instancePaused with an empty WorktreePath.
func TestInstancePause_Success(t *testing.T) {
	const (
		title        = "my-agent"
		repoPath     = "/repo"
		worktreePath = "/worktrees/my-agent"
		branchName   = "feat/my-agent"
	)

	loader := seedMutable(instanceRecord{
		Title:  title,
		Status: instanceRunning,
		Worktree: instanceWorktree{
			RepoPath:     repoPath,
			WorktreePath: worktreePath,
			BranchName:   branchName,
		},
	})

	runner, calls := captureRuns()
	handler := makeInstancePauseHandler(loader, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"title": title}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "expected success, got: %v", textResult(t, result))

	// Assert the expected commands were issued.
	assert.True(t, hasCall(*calls, "tmux", "kill-session", "-t", kasTmuxName(title)),
		"expected tmux kill-session call; calls: %v", *calls)
	assert.True(t, hasCall(*calls, "git", "-C", repoPath, "worktree", "remove", "--force", worktreePath),
		"expected git worktree remove call; calls: %v", *calls)
	assert.True(t, hasCall(*calls, "git", "-C", repoPath, "worktree", "prune"),
		"expected git worktree prune call; calls: %v", *calls)

	// Reload state and verify the record was updated.
	records, err := loadRecords(loader)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, instancePaused, records[0].Status)
	assert.Empty(t, records[0].Worktree.WorktreePath)
}

// TestInstancePause_NoWorktree verifies that when an instance has no worktree
// metadata, the pause handler does not issue any "worktree remove" command.
func TestInstancePause_NoWorktree(t *testing.T) {
	const title = "no-worktree-agent"

	loader := seedMutable(instanceRecord{
		Title:  title,
		Status: instanceRunning,
		// Empty worktree metadata — no RepoPath or WorktreePath.
	})

	runner, calls := captureRuns()
	handler := makeInstancePauseHandler(loader, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"title": title}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// Must not attempt to remove a worktree when there is no path metadata.
	assert.False(t, anyCallContains(*calls, "worktree remove"),
		"expected no worktree remove call; calls: %v", *calls)
}

// ---------------------------------------------------------------------------
// instance_resume tests
// ---------------------------------------------------------------------------

// TestInstanceResume_Success verifies that resuming a paused instance re-adds
// the git worktree on the preserved branch and starts a tmux session whose
// program string begins with KASMOS_MANAGED=1.
func TestInstanceResume_Success(t *testing.T) {
	const (
		title      = "paused-agent"
		repoPath   = "/repo"
		branchName = "feat/paused-agent"
		agentPath  = "/worktrees/paused-agent"
		program    = "claude"
		agentType  = "coder"
	)

	loader := seedMutable(instanceRecord{
		Title:           title,
		Status:          instancePaused,
		Program:         program,
		Path:            agentPath,
		AgentType:       agentType,
		SkipPermissions: true,
		Worktree: instanceWorktree{
			RepoPath:   repoPath,
			BranchName: branchName,
			// WorktreePath is empty (cleared during pause).
		},
	})

	runner, calls := captureRuns()
	handler := makeInstanceResumeHandler(loader, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"title": title}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "expected success, got: %v", textResult(t, result))

	// Assert git worktree add uses the preserved branch.
	assert.True(t, hasCall(*calls, "git", "-C", repoPath, "worktree", "add", agentPath, branchName),
		"expected git worktree add call with branch; calls: %v", *calls)

	// Find the tmux new-session call and verify the program begins with KASMOS_MANAGED=1.
	var foundTmuxCall bool
	for _, call := range *calls {
		if len(call) >= 8 && call[0] == "tmux" && call[1] == "new-session" {
			foundTmuxCall = true
			programArg := call[len(call)-1]
			assert.True(t, strings.HasPrefix(programArg, "KASMOS_MANAGED=1"),
				"program arg should begin with KASMOS_MANAGED=1, got: %q", programArg)
		}
	}
	assert.True(t, foundTmuxCall, "expected tmux new-session call; calls: %v", *calls)

	// Reload state and verify the record was updated to running with the restored worktree path.
	records, err := loadRecords(loader)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, instanceRunning, records[0].Status)
	assert.Equal(t, agentPath, records[0].Worktree.WorktreePath)
}

// TestInstanceResume_NoWorktreeMetadata verifies that resume fails with the
// exact phrase "no stored worktree metadata" when RepoPath or BranchName is absent.
func TestInstanceResume_NoWorktreeMetadata(t *testing.T) {
	const title = "no-meta-agent"

	loader := seedInstances(instanceRecord{
		Title:  title,
		Status: instancePaused,
		// No Worktree.RepoPath or Worktree.BranchName.
	})

	handler := makeInstanceResumeHandler(loader, &mockRunner{})
	result, err := handler(context.Background(), mockReq(map[string]any{"title": title}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "no stored worktree metadata")
}
