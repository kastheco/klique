package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// instanceTestData mirrors session.InstanceData fields needed for constructing
// test states without importing the session package (which would create an import
// cycle via session/tmux → cmd → session).
type instanceTestData struct {
	Title     string `json:"title"`
	Status    int    `json:"status"` // 0=Running,1=Ready,2=Loading,3=Paused
	Branch    string `json:"branch"`
	Program   string `json:"program"`
	TaskFile  string `json:"task_file,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

// newTestStateFromRaw returns a State pre-populated with the given instance records.
func newTestStateFromRaw(t *testing.T, instances []instanceTestData) *config.State {
	t.Helper()
	raw, err := json.Marshal(instances)
	require.NoError(t, err)
	s := config.DefaultState()
	s.InstancesData = raw
	return s
}

// newTestStateFromRecords returns a State pre-populated with full instanceRecord values.
// Use this helper when the test needs to exercise round-trip field preservation.
func newTestStateFromRecords(t *testing.T, records []instanceRecord) *config.State {
	t.Helper()
	raw, err := json.Marshal(records)
	require.NoError(t, err)
	s := config.DefaultState()
	s.InstancesData = raw
	return s
}

func TestInstanceList_Text(t *testing.T) {
	state := newTestStateFromRaw(t, []instanceTestData{
		{Title: "planner-foo", Status: 0 /* Running */, Branch: "plan/foo", Program: "claude", TaskFile: "foo.md"},
		{Title: "coder-bar", Status: 1 /* Ready */, Branch: "plan/bar", Program: "opencode", TaskFile: "bar.md"},
		{Title: "solo-baz", Status: 3 /* Paused */, Branch: "plan/baz", Program: "claude"},
	})

	output := executeInstanceList(state, "text")
	assert.Contains(t, output, "planner-foo")
	assert.Contains(t, output, "coder-bar")
	assert.Contains(t, output, "solo-baz")
	assert.Contains(t, output, "running")
	assert.Contains(t, output, "ready")
	assert.Contains(t, output, "paused")
}

func TestInstanceList_JSON(t *testing.T) {
	instances := []instanceTestData{
		{Title: "agent-1", Status: 0 /* Running */, Branch: "plan/agent", Program: "claude"},
	}
	state := newTestStateFromRaw(t, instances)

	output := executeInstanceList(state, "json")
	var parsed []map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed, 1)
	assert.Equal(t, "agent-1", parsed[0]["title"])
}

func TestInstanceList_Empty(t *testing.T) {
	state := newTestStateFromRaw(t, []instanceTestData{})
	output := executeInstanceList(state, "text")
	assert.Equal(t, "no instances\n", output)
}

func TestInstanceList_StatusFilter(t *testing.T) {
	state := newTestStateFromRaw(t, []instanceTestData{
		{Title: "running-1", Status: 0 /* Running */, Program: "claude"},
		{Title: "paused-1", Status: 3 /* Paused */, Program: "claude"},
	})

	output := executeInstanceList(state, "text", "paused")
	assert.Contains(t, output, "paused-1")
	assert.NotContains(t, output, "running-1")
}

func TestFindInstanceData_Found(t *testing.T) {
	records := []instanceRecord{
		{Title: "alpha", Status: instanceRunning},
		{Title: "beta", Status: instancePaused},
	}
	found, err := findInstanceData(records, "beta")
	require.NoError(t, err)
	assert.Equal(t, "beta", found.Title)
	assert.Equal(t, instancePaused, found.Status)
}

func TestFindInstanceData_NotFound(t *testing.T) {
	records := []instanceRecord{
		{Title: "alpha", Status: instanceRunning},
	}
	_, err := findInstanceData(records, "missing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindInstanceData_FuzzyMatch(t *testing.T) {
	records := []instanceRecord{
		{Title: "planner-my-feature", Status: instanceRunning},
		{Title: "coder-my-feature-task-1", Status: instanceRunning},
	}
	// Exact match should work
	found, err := findInstanceData(records, "planner-my-feature")
	require.NoError(t, err)
	assert.Equal(t, "planner-my-feature", found.Title)

	// Substring match when no exact match
	found, err = findInstanceData(records, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "coder-my-feature-task-1", found.Title)
}

func TestFindInstanceData_AmbiguousSubstring(t *testing.T) {
	records := []instanceRecord{
		{Title: "planner-foo", Status: instanceRunning},
		{Title: "coder-foo", Status: instanceRunning},
	}
	_, err := findInstanceData(records, "foo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestValidateInstanceStatus_Kill(t *testing.T) {
	// Kill should work on any status
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceRunning}, "kill"))
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceReady}, "kill"))
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instancePaused}, "kill"))
}

func TestValidateInstanceStatus_Pause(t *testing.T) {
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceRunning}, "pause"))
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceReady}, "pause"))
	assert.Error(t, validateStatusForAction(instanceRecord{Status: instancePaused}, "pause"))
}

func TestValidateInstanceStatus_Resume(t *testing.T) {
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instancePaused}, "resume"))
	assert.Error(t, validateStatusForAction(instanceRecord{Status: instanceRunning}, "resume"))
}

func TestValidateInstanceStatus_Send(t *testing.T) {
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceRunning}, "send"))
	assert.NoError(t, validateStatusForAction(instanceRecord{Status: instanceReady}, "send"))
	assert.Error(t, validateStatusForAction(instanceRecord{Status: instancePaused}, "send"))
}

// fullInstanceRecord returns an instanceRecord with every field set to a
// non-zero/non-default value.  Used by round-trip tests to verify that
// removeInstanceFromState and updateInstanceInState do not silently drop any
// field when they deserialise and re-serialise the state JSON.
func fullInstanceRecord(title string) instanceRecord {
	return instanceRecord{
		Title:                  title,
		Path:                   "/worktrees/" + title,
		Branch:                 "feat/" + title,
		Status:                 instanceRunning,
		Height:                 48,
		Width:                  220,
		CreatedAt:              time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:              time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
		Program:                "claude",
		AutoYes:                true,
		SkipPermissions:        true,
		TaskFile:               title + ".md",
		AgentType:              "coder",
		TaskNumber:             3,
		WaveNumber:             2,
		PeerCount:              4,
		IsReviewer:             false,
		ImplementationComplete: true,
		SoloAgent:              false,
		QueuedPrompt:           "hello from " + title,
		ReviewCycle:            2,
		Worktree: instanceWorktree{
			RepoPath:      "/repo",
			WorktreePath:  "/repo/.worktrees/" + title,
			SessionName:   title,
			BranchName:    "feat/" + title,
			BaseCommitSHA: "deadbeef" + title,
		},
		DiffStats: instanceDiffStats{
			Added:   10,
			Removed: 5,
			Content: "some diff for " + title,
		},
	}
}

// assertRecordFieldsEqual asserts that all fields of want and got are equal.
// This is the canonical checker used by round-trip tests.
func assertRecordFieldsEqual(t *testing.T, want, got instanceRecord) {
	t.Helper()
	assert.Equal(t, want.Title, got.Title, "Title")
	assert.Equal(t, want.Path, got.Path, "Path")
	assert.Equal(t, want.Branch, got.Branch, "Branch")
	assert.Equal(t, want.Status, got.Status, "Status")
	assert.Equal(t, want.Height, got.Height, "Height")
	assert.Equal(t, want.Width, got.Width, "Width")
	assert.True(t, want.CreatedAt.Equal(got.CreatedAt), "CreatedAt: want %v got %v", want.CreatedAt, got.CreatedAt)
	assert.True(t, want.UpdatedAt.Equal(got.UpdatedAt), "UpdatedAt: want %v got %v", want.UpdatedAt, got.UpdatedAt)
	assert.Equal(t, want.Program, got.Program, "Program")
	assert.Equal(t, want.AutoYes, got.AutoYes, "AutoYes")
	assert.Equal(t, want.SkipPermissions, got.SkipPermissions, "SkipPermissions")
	assert.Equal(t, want.TaskFile, got.TaskFile, "TaskFile")
	assert.Equal(t, want.AgentType, got.AgentType, "AgentType")
	assert.Equal(t, want.TaskNumber, got.TaskNumber, "TaskNumber")
	assert.Equal(t, want.WaveNumber, got.WaveNumber, "WaveNumber")
	assert.Equal(t, want.PeerCount, got.PeerCount, "PeerCount")
	assert.Equal(t, want.IsReviewer, got.IsReviewer, "IsReviewer")
	assert.Equal(t, want.ImplementationComplete, got.ImplementationComplete, "ImplementationComplete")
	assert.Equal(t, want.SoloAgent, got.SoloAgent, "SoloAgent")
	assert.Equal(t, want.QueuedPrompt, got.QueuedPrompt, "QueuedPrompt")
	assert.Equal(t, want.ReviewCycle, got.ReviewCycle, "ReviewCycle")
	assert.Equal(t, want.Worktree.RepoPath, got.Worktree.RepoPath, "Worktree.RepoPath")
	assert.Equal(t, want.Worktree.WorktreePath, got.Worktree.WorktreePath, "Worktree.WorktreePath")
	assert.Equal(t, want.Worktree.SessionName, got.Worktree.SessionName, "Worktree.SessionName")
	assert.Equal(t, want.Worktree.BranchName, got.Worktree.BranchName, "Worktree.BranchName")
	assert.Equal(t, want.Worktree.BaseCommitSHA, got.Worktree.BaseCommitSHA, "Worktree.BaseCommitSHA")
	assert.Equal(t, want.DiffStats.Added, got.DiffStats.Added, "DiffStats.Added")
	assert.Equal(t, want.DiffStats.Removed, got.DiffStats.Removed, "DiffStats.Removed")
	assert.Equal(t, want.DiffStats.Content, got.DiffStats.Content, "DiffStats.Content")
}

// TestRemoveInstanceFromState_PreservesFields verifies that removing one instance
// does not corrupt any fields of the remaining instances during the JSON round-trip.
// This is the regression test for the data-loss bug where instanceRecord was
// incomplete and silently dropped ~15 fields from every surviving instance.
func TestRemoveInstanceFromState_PreservesFields(t *testing.T) {
	keeper := fullInstanceRecord("keeper")
	goner := fullInstanceRecord("goner")

	state := newTestStateFromRecords(t, []instanceRecord{keeper, goner})

	err := removeInstanceFromState(state, "goner")
	require.NoError(t, err)

	remaining, err := loadInstanceRecords(state)
	require.NoError(t, err)
	require.Len(t, remaining, 1, "expected exactly one instance to remain after removal")

	assertRecordFieldsEqual(t, keeper, remaining[0])
}

// TestRemoveInstanceFromState_RemovesOnlyTarget verifies that removeInstanceFromState
// removes only the named instance and leaves all others intact (titles only).
func TestRemoveInstanceFromState_RemovesOnlyTarget(t *testing.T) {
	state := newTestStateFromRecords(t, []instanceRecord{
		{Title: "alpha", Status: instanceRunning, Program: "claude"},
		{Title: "beta", Status: instanceRunning, Program: "claude"},
		{Title: "gamma", Status: instanceRunning, Program: "claude"},
	})

	err := removeInstanceFromState(state, "beta")
	require.NoError(t, err)

	remaining, err := loadInstanceRecords(state)
	require.NoError(t, err)
	require.Len(t, remaining, 2)
	assert.Equal(t, "alpha", remaining[0].Title)
	assert.Equal(t, "gamma", remaining[1].Title)
}

// TestUpdateInstanceInState_PreservesFields verifies that updating one field of one
// instance does not corrupt any fields of the other instances during the JSON
// round-trip.  This is the regression test for the data-loss bug in pause/resume.
func TestUpdateInstanceInState_PreservesFields(t *testing.T) {
	untouched := fullInstanceRecord("untouched")
	target := fullInstanceRecord("target")
	target.Status = instancePaused // start paused so we can "resume" it

	state := newTestStateFromRecords(t, []instanceRecord{untouched, target})

	// Simulate what pause does: sets status to paused and clears the worktree path.
	err := updateInstanceInState(state, "target", func(r *instanceRecord) error {
		r.Status = instancePaused
		r.Worktree.WorktreePath = ""
		return nil
	})
	require.NoError(t, err)

	records, err := loadInstanceRecords(state)
	require.NoError(t, err)
	require.Len(t, records, 2)

	// untouched instance must have ALL its fields preserved.
	var gotUntouched, gotTarget instanceRecord
	for _, r := range records {
		switch r.Title {
		case "untouched":
			gotUntouched = r
		case "target":
			gotTarget = r
		}
	}

	assertRecordFieldsEqual(t, untouched, gotUntouched)

	// target must have only Status and Worktree.WorktreePath changed.
	assert.Equal(t, instancePaused, gotTarget.Status, "target.Status should be paused")
	assert.Empty(t, gotTarget.Worktree.WorktreePath, "target.Worktree.WorktreePath should be cleared")
	// All other fields of target should be preserved.
	assert.Equal(t, target.Height, gotTarget.Height, "target.Height")
	assert.Equal(t, target.Width, gotTarget.Width, "target.Width")
	assert.Equal(t, target.AutoYes, gotTarget.AutoYes, "target.AutoYes")
	assert.Equal(t, target.SkipPermissions, gotTarget.SkipPermissions, "target.SkipPermissions")
	assert.Equal(t, target.TaskNumber, gotTarget.TaskNumber, "target.TaskNumber")
	assert.Equal(t, target.WaveNumber, gotTarget.WaveNumber, "target.WaveNumber")
	assert.Equal(t, target.PeerCount, gotTarget.PeerCount, "target.PeerCount")
	assert.Equal(t, target.ImplementationComplete, gotTarget.ImplementationComplete, "target.ImplementationComplete")
	assert.Equal(t, target.QueuedPrompt, gotTarget.QueuedPrompt, "target.QueuedPrompt")
	assert.Equal(t, target.ReviewCycle, gotTarget.ReviewCycle, "target.ReviewCycle")
	assert.Equal(t, target.Worktree.RepoPath, gotTarget.Worktree.RepoPath, "target.Worktree.RepoPath")
	assert.Equal(t, target.Worktree.SessionName, gotTarget.Worktree.SessionName, "target.Worktree.SessionName")
	assert.Equal(t, target.Worktree.BranchName, gotTarget.Worktree.BranchName, "target.Worktree.BranchName")
	assert.Equal(t, target.Worktree.BaseCommitSHA, gotTarget.Worktree.BaseCommitSHA, "target.Worktree.BaseCommitSHA")
	assert.Equal(t, target.DiffStats.Added, gotTarget.DiffStats.Added, "target.DiffStats.Added")
	assert.Equal(t, target.DiffStats.Removed, gotTarget.DiffStats.Removed, "target.DiffStats.Removed")
	assert.Equal(t, target.DiffStats.Content, gotTarget.DiffStats.Content, "target.DiffStats.Content")
}

// TestUpdateInstanceInState_NotFound verifies that an error is returned when the
// named instance does not exist.
func TestUpdateInstanceInState_NotFound(t *testing.T) {
	state := newTestStateFromRecords(t, []instanceRecord{
		{Title: "alpha", Status: instanceRunning, Program: "claude"},
	})
	err := updateInstanceInState(state, "missing", func(r *instanceRecord) error {
		r.Status = instancePaused
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestInstanceRecord_PlanFileMigration verifies that the legacy "plan_file" JSON
// key is migrated to "task_file" when unmarshalling, matching the behaviour of
// session.InstanceData.UnmarshalJSON.
func TestInstanceRecord_PlanFileMigration(t *testing.T) {
	raw := `[{"title":"legacy","status":0,"program":"claude","plan_file":"old-plan.md"}]`
	s := config.DefaultState()
	s.InstancesData = []byte(raw)

	records, err := loadInstanceRecords(s)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "old-plan.md", records[0].TaskFile, "plan_file should be migrated to task_file")
}

// TestKasTmuxName verifies the session name prefix logic replicates what
// session/tmux.ToKasTmuxNamePublic produces.
func TestKasTmuxName(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"asdf", "kas_asdf"},
		{"my-instance", "kas_my-instance"},
		// dots are replaced by underscores (tmux does this natively)
		{"a.b.c", "kas_a_b_c"},
		// whitespace is stripped
		{"a b c", "kas_abc"},
	}
	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			assert.Equal(t, tc.want, kasTmuxName(tc.title))
		})
	}
}

// TestKillCmd_SetsInstanceToPaused verifies that the kill command updates the
// instance to paused status instead of removing it from state.
func TestKillCmd_SetsInstanceToPaused(t *testing.T) {
	rec := fullInstanceRecord("kill-target")
	rec.Status = instanceRunning
	other := fullInstanceRecord("other")
	state := newTestStateFromRecords(t, []instanceRecord{rec, other})

	// Simulate what the new kill logic should do: update to paused, not remove.
	err := updateInstanceInState(state, "kill-target", func(r *instanceRecord) error {
		r.Status = instancePaused
		r.Worktree.WorktreePath = ""
		return nil
	})
	require.NoError(t, err)

	records, err := loadInstanceRecords(state)
	require.NoError(t, err)
	require.Len(t, records, 2, "kill should NOT remove instance from state")

	var target instanceRecord
	for _, r := range records {
		if r.Title == "kill-target" {
			target = r
		}
	}
	assert.Equal(t, instancePaused, target.Status, "killed instance should be paused")
	assert.Empty(t, target.Worktree.WorktreePath, "worktree path should be cleared")
}

// TestBuildResumeCommand_BasicClaude verifies that a basic Claude instance gets
// KASMOS_MANAGED=1 prepended and no extra flags for default settings.
func TestBuildResumeCommand_BasicClaude(t *testing.T) {
	rec := instanceRecord{
		Title:   "my-coder",
		Program: "claude",
	}
	got := buildResumeCommand(rec, "/worktrees/my-coder")
	assert.Equal(t, "KASMOS_MANAGED=1 claude", got)
}

// TestBuildResumeCommand_SkipPermissions verifies --dangerously-skip-permissions is appended for Claude.
func TestBuildResumeCommand_SkipPermissions(t *testing.T) {
	rec := instanceRecord{
		Title:           "my-coder",
		Program:         "claude",
		SkipPermissions: true,
	}
	got := buildResumeCommand(rec, "/worktrees/my-coder")
	assert.Contains(t, got, "--dangerously-skip-permissions")
	assert.Contains(t, got, "KASMOS_MANAGED=1")
}

// TestBuildResumeCommand_AgentType verifies --agent flag is injected.
func TestBuildResumeCommand_AgentType(t *testing.T) {
	rec := instanceRecord{
		Title:     "my-coder",
		Program:   "claude",
		AgentType: "coder",
	}
	got := buildResumeCommand(rec, "/worktrees/my-coder")
	assert.Contains(t, got, "--agent coder")
}

// TestBuildResumeCommand_OpencodeWithAgent verifies that opencode log redirection
// is included even when --agent is appended (regression: the suffix check must
// use the unmodified base program, not the local variable).
func TestBuildResumeCommand_OpencodeWithAgent(t *testing.T) {
	workdir := t.TempDir()
	rec := instanceRecord{
		Title:     "my-planner",
		Program:   "opencode",
		AgentType: "planner",
	}
	got := buildResumeCommand(rec, workdir)
	assert.Contains(t, got, "--agent planner")
	assert.Contains(t, got, "--print-logs")
	assert.Contains(t, got, "KASMOS_MANAGED=1")
}

// TestBuildResumeCommand_TaskEnvVars verifies task identity env vars are prepended.
func TestBuildResumeCommand_TaskEnvVars(t *testing.T) {
	rec := instanceRecord{
		Title:      "my-coder",
		Program:    "claude",
		TaskNumber: 3,
		WaveNumber: 2,
		PeerCount:  4,
	}
	got := buildResumeCommand(rec, "/worktrees/my-coder")
	assert.Contains(t, got, "KASMOS_TASK=3")
	assert.Contains(t, got, "KASMOS_WAVE=2")
	assert.Contains(t, got, "KASMOS_PEERS=4")
}

// TestSummarizeInstanceStatus_Mixed verifies aggregation across all known and
// unknown status values including instanceLoading (which counts as running).
func TestSummarizeInstanceStatus_Mixed(t *testing.T) {
	records := []instanceRecord{
		{Title: "r1", Status: instanceRunning},
		{Title: "r2", Status: instanceLoading}, // loading → running bucket
		{Title: "r3", Status: instanceReady},
		{Title: "r4", Status: instancePaused},
		{Title: "r5", Status: instanceStatus(99)}, // unknown → killed bucket
	}
	summary := summarizeInstanceStatus(records)
	assert.Equal(t, 2, summary.Running, "running should include loading")
	assert.Equal(t, 1, summary.Ready)
	assert.Equal(t, 1, summary.Paused)
	assert.Equal(t, 1, summary.Killed, "unknown status should go to killed")
}

// TestExecuteInstanceStatus_Empty verifies the empty-state output.
func TestExecuteInstanceStatus_Empty(t *testing.T) {
	state := newTestStateFromRaw(t, []instanceTestData{})
	out, err := executeInstanceStatus(state)
	require.NoError(t, err)
	assert.Equal(t, "no instances found\n", out)
}

// TestExecuteInstanceStatus_OutputOrder verifies deterministic row order in the
// tabwriter output: running, ready, paused, killed.
func TestExecuteInstanceStatus_OutputOrder(t *testing.T) {
	state := newTestStateFromRaw(t, []instanceTestData{
		{Title: "a", Status: 3 /* Paused */},
		{Title: "b", Status: 0 /* Running */},
		{Title: "c", Status: 1 /* Ready */},
	})
	out, err := executeInstanceStatus(state)
	require.NoError(t, err)

	// Verify header and row presence
	assert.Contains(t, out, "STATE")
	assert.Contains(t, out, "COUNT")

	// Verify deterministic order: running before ready before paused before killed
	runIdx := strings.Index(out, "running")
	readyIdx := strings.Index(out, "ready")
	pausedIdx := strings.Index(out, "paused")
	killedIdx := strings.Index(out, "killed")
	assert.Greater(t, readyIdx, runIdx, "ready should come after running")
	assert.Greater(t, pausedIdx, readyIdx, "paused should come after ready")
	assert.Greater(t, killedIdx, pausedIdx, "killed should come after paused")
}

// TestExecuteInstanceStatus_ParseError verifies that invalid JSON in state
// returns an error containing "parse instances".
func TestExecuteInstanceStatus_ParseError(t *testing.T) {
	s := config.DefaultState()
	s.InstancesData = []byte(`not-valid-json`)
	_, err := executeInstanceStatus(s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse instances")
}

// TestNewInstanceCmd_HasStatusSubcommand verifies the status subcommand is
// registered on the instance command tree.
func TestNewInstanceCmd_HasStatusSubcommand(t *testing.T) {
	cmd, _, err := NewInstanceCmd().Find([]string{"status"})
	require.NoError(t, err)
	assert.Equal(t, "status", cmd.Name())
}

// TestNewRootCmd_HasInstanceStatus verifies the full path kas instance status
// resolves correctly from the root command.
func TestNewRootCmd_HasInstanceStatus(t *testing.T) {
	cmd, _, err := NewRootCmd().Find([]string{"instance", "status"})
	require.NoError(t, err)
	assert.Equal(t, "status", cmd.Name())
}

// TestSummarizeInstanceStatus_UnknownStatus verifies that an unknown status
// value (e.g. instanceStatus(99)) is counted in the Killed bucket.
func TestSummarizeInstanceStatus_UnknownStatus(t *testing.T) {
	records := []instanceRecord{{Title: "stale", Status: instanceStatus(99)}}
	summary := summarizeInstanceStatus(records)
	assert.Equal(t, 1, summary.Killed)
	assert.Equal(t, 0, summary.Running)
	assert.Equal(t, 0, summary.Ready)
	assert.Equal(t, 0, summary.Paused)
}
