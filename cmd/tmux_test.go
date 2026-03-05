package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers -----------------------------------------------------------------

// fakeTmuxOutput builds a pipe-separated tmux ls -F line for testing.
func fakeTmuxLine(name string, epochSecs int64, windows int, attached bool, width, height int) string {
	att := "0"
	if attached {
		att = "1"
	}
	return fmt.Sprintf("%s|%d|%d|%s|%d|%d",
		name, epochSecs, windows, att, width, height)
}

// mockExecOutput returns a MockCmdExec that returns the given output on Output().
func mockExecOutput(output string) *cmd_test.MockCmdExec {
	m := cmd_test.NewMockExecutor()
	m.OutputFunc = func(_ *exec.Cmd) ([]byte, error) {
		return []byte(output), nil
	}
	return m
}

// mockExecExitError returns a MockCmdExec whose Output() returns an *exec.ExitError.
func mockExecExitError() *cmd_test.MockCmdExec {
	m := cmd_test.NewMockExecutor()
	m.OutputFunc = func(_ *exec.Cmd) ([]byte, error) {
		return nil, &exec.ExitError{}
	}
	return m
}

// --- TestRelativeAge_Buckets -------------------------------------------------

func TestRelativeAge_Buckets(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		created time.Time
		want    string
	}{
		{"seconds", now.Add(-30 * time.Second), "30s ago"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-48 * time.Hour), "2d ago"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := relativeAge(now, tc.created)
			assert.Equal(t, tc.want, got)
		})
	}
}

// --- TestDiscoverKasSessions_ParsesAndSkipsMalformed ------------------------

func TestDiscoverKasSessions_ParsesAndSkipsMalformed(t *testing.T) {
	epoch := int64(1741084800) // fixed epoch for deterministic test
	lines := strings.Join([]string{
		fakeTmuxLine("kas_foo", epoch, 2, false, 220, 50),
		fakeTmuxLine("kas_bar", epoch, 1, true, 200, 40),
		fakeTmuxLine("other_session", epoch, 1, false, 80, 24), // non-kas_ — should be ignored
		"malformed-line-without-pipes",                         // malformed — should be skipped
		"kas_missing|fields",                                   // fewer than 6 fields — skipped
	}, "\n")

	exec := mockExecOutput(lines)
	known := map[string]struct{}{
		"kas_foo": {},
	}

	rows, err := discoverKasSessions(exec, known)
	require.NoError(t, err)
	require.Len(t, rows, 2, "only kas_ sessions with 6+ fields should be returned")

	assert.Equal(t, "kas_foo", rows[0].Name)
	assert.Equal(t, "foo", rows[0].Title)
	assert.Equal(t, 2, rows[0].Windows)
	assert.False(t, rows[0].Attached)
	assert.Equal(t, 220, rows[0].Width)
	assert.Equal(t, 50, rows[0].Height)
	assert.True(t, rows[0].Managed, "kas_foo is in known set")

	assert.Equal(t, "kas_bar", rows[1].Name)
	assert.Equal(t, "bar", rows[1].Title)
	assert.True(t, rows[1].Attached)
	assert.False(t, rows[1].Managed, "kas_bar is NOT in known set")

	// Verify epoch parsing
	assert.Equal(t, time.Unix(epoch, 0), rows[0].Created)
}

// --- TestDiscoverKasSessions_ExitErrorIsEmpty --------------------------------

func TestDiscoverKasSessions_ExitErrorIsEmpty(t *testing.T) {
	exec := mockExecExitError()
	rows, err := discoverKasSessions(exec, nil)
	require.NoError(t, err, "ExitError from tmux ls should be treated as empty list")
	assert.Empty(t, rows)
}

// --- TestExecuteTmuxList_FiltersManaged --------------------------------------

func TestExecuteTmuxList_FiltersManaged(t *testing.T) {
	epoch := int64(1741084800)
	// kas_orphan is unmanaged, kas_managed is managed.
	lines := strings.Join([]string{
		fakeTmuxLine("kas_orphan", epoch, 1, false, 200, 40),
		fakeTmuxLine("kas_managed", epoch, 2, true, 220, 50),
	}, "\n")

	exec := mockExecOutput(lines)

	// kas_managed exists in state as a known instance.
	state := newTestStateFromRecords(t, []instanceRecord{
		{Title: "managed", Program: "claude", Status: instanceRunning},
	})

	out, err := executeTmuxList(state, exec)
	require.NoError(t, err)

	// orphan row should appear
	assert.Contains(t, out, "kas_orphan")
	// managed row should NOT appear in orphan list
	assert.NotContains(t, out, "kas_managed")
	// header should be present
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "TITLE")
	assert.Contains(t, out, "WINDOWS")
}

// --- TestExecuteTmuxList_Empty -----------------------------------------------

func TestExecuteTmuxList_Empty(t *testing.T) {
	// No tmux sessions at all.
	exec := mockExecOutput("")
	state := newTestStateFromRecords(t, []instanceRecord{})

	out, err := executeTmuxList(state, exec)
	require.NoError(t, err)
	assert.Equal(t, "no orphan tmux sessions found\n", out)
}

// TestExecuteTmuxList_EmptyWhenAllManaged verifies that when all kas_ sessions
// are managed, the output is "no orphan tmux sessions found".
func TestExecuteTmuxList_EmptyWhenAllManaged(t *testing.T) {
	epoch := int64(1741084800)
	lines := fakeTmuxLine("kas_managed", epoch, 1, false, 200, 40)

	exec := mockExecOutput(lines)
	state := newTestStateFromRecords(t, []instanceRecord{
		{Title: "managed", Program: "claude", Status: instanceRunning},
	})

	out, err := executeTmuxList(state, exec)
	require.NoError(t, err)
	assert.Equal(t, "no orphan tmux sessions found\n", out)
}

// --- TestExecuteTmuxAdopt_Validation -----------------------------------------

func TestExecuteTmuxAdopt_Validation(t *testing.T) {
	epoch := int64(1741084800)
	orphanLine := fakeTmuxLine("kas_orphan", epoch, 1, false, 200, 40)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	t.Run("empty title rejected", func(t *testing.T) {
		exec := mockExecOutput(orphanLine)
		state := newTestStateFromRecords(t, nil)
		err := executeTmuxAdopt(state, "kas_orphan", "   ", "/repo", now, exec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title must not be empty")
	})

	t.Run("duplicate title rejected", func(t *testing.T) {
		exec := mockExecOutput(orphanLine)
		state := newTestStateFromRecords(t, []instanceRecord{
			{Title: "existing", Program: "claude", Status: instanceRunning},
		})
		err := executeTmuxAdopt(state, "kas_orphan", "existing", "/repo", now, exec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instance title already exists: existing")
	})

	t.Run("session not found rejects", func(t *testing.T) {
		exec := mockExecOutput(orphanLine)
		state := newTestStateFromRecords(t, nil)
		err := executeTmuxAdopt(state, "kas_nonexistent", "newtitle", "/repo", now, exec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "orphan tmux session not found: kas_nonexistent")
	})

	t.Run("managed session not adoptable", func(t *testing.T) {
		exec := mockExecOutput(orphanLine)
		// kas_orphan is managed (has a matching instance record "orphan")
		state := newTestStateFromRecords(t, []instanceRecord{
			{Title: "orphan", Program: "claude", Status: instanceRunning},
		})
		err := executeTmuxAdopt(state, "kas_orphan", "newtitle", "/repo", now, exec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "orphan tmux session not found: kas_orphan")
	})
}

// --- TestExecuteTmuxAdopt_PreservesExistingFields ----------------------------

func TestExecuteTmuxAdopt_PreservesExistingFields(t *testing.T) {
	epoch := int64(1741084800)
	orphanLine := fakeTmuxLine("kas_orphan", epoch, 1, false, 200, 40)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	// Seed state with a full-field record so we can assert it survives the round-trip.
	existing := fullInstanceRecord("existing-keeper")
	state := newTestStateFromRecords(t, []instanceRecord{existing})

	exec := mockExecOutput(orphanLine)
	err := executeTmuxAdopt(state, "kas_orphan", "my-adopted", "/my/repo", now, exec)
	require.NoError(t, err)

	records, err := loadInstanceRecords(state)
	require.NoError(t, err)
	require.Len(t, records, 2, "existing + newly adopted record")

	// Existing record must be completely unchanged.
	var keeper, adopted instanceRecord
	for _, r := range records {
		if r.Title == "existing-keeper" {
			keeper = r
		}
		if r.Title == "my-adopted" {
			adopted = r
		}
	}
	assertRecordFieldsEqual(t, existing, keeper)

	// Newly adopted record should have the seeded fields.
	assert.Equal(t, "my-adopted", adopted.Title)
	assert.Equal(t, "/my/repo", adopted.Path)
	assert.Equal(t, instanceReady, adopted.Status)
	assert.Equal(t, "unknown", adopted.Program)
	assert.True(t, adopted.CreatedAt.Equal(now), "CreatedAt should be seeded now")
	assert.True(t, adopted.UpdatedAt.Equal(now), "UpdatedAt should be seeded now")
}

// --- TestExecuteTmuxKill_WrapsExecutorError ----------------------------------

func TestExecuteTmuxKill_WrapsExecutorError(t *testing.T) {
	epoch := int64(1741084800)
	orphanLine := fakeTmuxLine("kas_orphan", epoch, 1, false, 200, 40)

	// Executor: Output returns orphan session; Run returns an error.
	m := cmd_test.NewMockExecutor()
	m.OutputFunc = func(_ *exec.Cmd) ([]byte, error) {
		return []byte(orphanLine), nil
	}
	killErr := errors.New("tmux: session not found")
	m.RunFunc = func(cmd *exec.Cmd) error {
		return killErr
	}

	state := newTestStateFromRecords(t, nil) // no managed instances -> kas_orphan is orphan

	err := executeTmuxKill(state, "kas_orphan", m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kill tmux session kas_orphan")
	assert.ErrorIs(t, err, killErr)
}

func TestExecuteTmuxKill_CorrectCommandInvoked(t *testing.T) {
	epoch := int64(1741084800)
	orphanLine := fakeTmuxLine("kas_orphan", epoch, 1, false, 200, 40)

	var capturedCmd *exec.Cmd
	m := cmd_test.NewMockExecutor()
	m.OutputFunc = func(_ *exec.Cmd) ([]byte, error) {
		return []byte(orphanLine), nil
	}
	m.RunFunc = func(cmd *exec.Cmd) error {
		capturedCmd = cmd
		return nil
	}

	state := newTestStateFromRecords(t, nil)

	err := executeTmuxKill(state, "kas_orphan", m)
	require.NoError(t, err)
	require.NotNil(t, capturedCmd)
	assert.Equal(t, "tmux kill-session -t kas_orphan", ToString(capturedCmd))
}

func TestExecuteTmuxKill_SessionNotFound(t *testing.T) {
	// No sessions at all
	exec := mockExecOutput("")
	state := newTestStateFromRecords(t, nil)

	err := executeTmuxKill(state, "kas_nonexistent", exec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "orphan tmux session not found: kas_nonexistent")
}

// --- TestRootCmd_TmuxSubcommands ---------------------------------------------

func TestRootCmd_TmuxSubcommands(t *testing.T) {
	root := NewRootCmd()

	listCmd, _, err := root.Find([]string{"tmux", "list"})
	require.NoError(t, err)
	require.NotNil(t, listCmd)
	assert.Equal(t, "list", listCmd.Name())

	adoptCmd, _, err := root.Find([]string{"tmux", "adopt"})
	require.NoError(t, err)
	require.NotNil(t, adoptCmd)
	assert.Equal(t, "adopt", adoptCmd.Name())

	killCmd, _, err := root.Find([]string{"tmux", "kill"})
	require.NoError(t, err)
	require.NotNil(t, killCmd)
	assert.Equal(t, "kill", killCmd.Name())
}

// --- TestExecuteTmuxList_AgeFormatting ---------------------------------------

func TestExecuteTmuxList_AgeFormatting(t *testing.T) {
	// Use a fixed "now" to control age display; we call executeTmuxList which
	// uses time.Now internally, so we use a very recent epoch to get "s ago".
	now := time.Now()
	recentEpoch := now.Add(-10 * time.Second).Unix()
	orphanLine := fakeTmuxLine("kas_orphan", recentEpoch, 1, false, 200, 40)

	exec := mockExecOutput(orphanLine)
	state := newTestStateFromRecords(t, nil)

	out, err := executeTmuxList(state, exec)
	require.NoError(t, err)
	assert.Contains(t, out, "ago")
}

// --- TestExecuteTmuxAdopt_NewRecordFields ------------------------------------

func TestExecuteTmuxAdopt_NewRecordFields(t *testing.T) {
	epoch := int64(1741084800)
	orphanLine := fakeTmuxLine("kas_newone", epoch, 1, false, 200, 40)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	exec := mockExecOutput(orphanLine)
	state := newTestStateFromRecords(t, nil)

	err := executeTmuxAdopt(state, "kas_newone", "newone", "/path/to/repo", now, exec)
	require.NoError(t, err)

	records, err := loadInstanceRecords(state)
	require.NoError(t, err)
	require.Len(t, records, 1)

	r := records[0]
	assert.Equal(t, "newone", r.Title)
	assert.Equal(t, "/path/to/repo", r.Path)
	assert.Equal(t, instanceReady, r.Status)
	assert.Equal(t, "unknown", r.Program)
	assert.True(t, r.CreatedAt.Equal(now))
	assert.True(t, r.UpdatedAt.Equal(now))
}

// --- TestDiscoverKasSessions_AttachedParsing ---------------------------------

func TestDiscoverKasSessions_AttachedParsing(t *testing.T) {
	epoch := int64(1741084800)
	lines := strings.Join([]string{
		fakeTmuxLine("kas_attached", epoch, 1, true, 200, 40),
		fakeTmuxLine("kas_detached", epoch, 1, false, 200, 40),
	}, "\n")

	exec := mockExecOutput(lines)
	rows, err := discoverKasSessions(exec, nil)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.True(t, rows[0].Attached)
	assert.False(t, rows[1].Attached)
}

// --- TestExecuteTmuxList_ShowsAttached ---------------------------------------

func TestExecuteTmuxList_ShowsAttached(t *testing.T) {
	epoch := int64(1741084800)
	lines := strings.Join([]string{
		fakeTmuxLine("kas_orphan_att", epoch, 2, true, 200, 40),
	}, "\n")

	exec := mockExecOutput(lines)
	state := newTestStateFromRecords(t, nil)

	out, err := executeTmuxList(state, exec)
	require.NoError(t, err)
	assert.Contains(t, out, "true")
	assert.Contains(t, out, "2")
}

// --- TestDiscoverKasSessions_NonExitErrorPropagates --------------------------

func TestDiscoverKasSessions_NonExitErrorPropagates(t *testing.T) {
	// A plain error (not *exec.ExitError) should propagate.
	m := cmd_test.NewMockExecutor()
	m.OutputFunc = func(_ *exec.Cmd) ([]byte, error) {
		return nil, errors.New("connection refused")
	}

	_, err := discoverKasSessions(m, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

// --- JSON round-trip check via marshal/unmarshal of state --------------------

func TestTmuxAdopt_StateJSONRoundtrip(t *testing.T) {
	epoch := int64(1741084800)
	orphanLine := fakeTmuxLine("kas_roundtrip", epoch, 1, false, 200, 40)
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	exec := mockExecOutput(orphanLine)

	// State starts with a full-field record.
	keeper := fullInstanceRecord("roundtrip-keeper")
	state := newTestStateFromRecords(t, []instanceRecord{keeper})

	err := executeTmuxAdopt(state, "kas_roundtrip", "roundtrip-adopted", "/repo", now, exec)
	require.NoError(t, err)

	// Re-read state and verify the raw JSON preserved all keeper fields.
	raw := state.GetInstances()
	var decoded []instanceRecord
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded, 2)

	var gotKeeper instanceRecord
	for _, r := range decoded {
		if r.Title == "roundtrip-keeper" {
			gotKeeper = r
		}
	}
	assertRecordFieldsEqual(t, keeper, gotKeeper)
}
