package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestInstance creates a minimal session.Instance for use in audit tests.
func newTestInstance(title string) (*session.Instance, error) {
	return session.NewInstance(session.InstanceOptions{
		Title:   title,
		Path:    "/tmp",
		Program: "opencode",
	})
}

// newTestHomeWithToast creates a test home with a toastManager for tests that
// trigger error paths (e.g. pause/resume on unstarted instances).
func newTestHomeWithToast() *home {
	h := newTestHome()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	h.toastManager = overlay.NewToastManager(&sp)
	return h
}

func TestAuditEmit_PlanTransition(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:     auditlog.EventPlanTransition,
		Project:  "test",
		PlanFile: "plan.md",
		Message:  "ready → implementing",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventPlanTransition},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Contains(t, events[0].Message, "implementing")
}

func TestAuditEmit_WaveStarted(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:       auditlog.EventWaveStarted,
		Project:    "test",
		PlanFile:   "plan.md",
		WaveNumber: 1,
		Message:    "wave 1 started: 3 task(s)",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventWaveStarted},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, 1, events[0].WaveNumber)
	assert.Contains(t, events[0].Message, "wave 1")
}

func TestAuditEmit_WaveCompleted(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:       auditlog.EventWaveCompleted,
		Project:    "test",
		PlanFile:   "plan.md",
		WaveNumber: 2,
		Message:    "wave 2 complete: 3/3 tasks",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventWaveCompleted},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, 2, events[0].WaveNumber)
}

func TestAuditEmit_WaveFailed(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:       auditlog.EventWaveFailed,
		Project:    "test",
		PlanFile:   "plan.md",
		WaveNumber: 1,
		Message:    "wave 1: 1/2 tasks failed",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventWaveFailed},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Contains(t, events[0].Message, "failed")
}

func TestAuditEmit_PlanMerged(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:     auditlog.EventPlanMerged,
		Project:  "test",
		PlanFile: "plan.md",
		Message:  "plan merged to main",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventPlanMerged},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Contains(t, events[0].Message, "merged")
}

func TestAuditEmit_PlanCancelled(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:     auditlog.EventPlanCancelled,
		Project:  "test",
		PlanFile: "plan.md",
		Message:  "plan cancelled by user",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventPlanCancelled},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Contains(t, events[0].Message, "cancelled")
}

// TestAuditHomeEmit_PlanTransition verifies that the home.audit() helper
// correctly emits plan transition events through the audit logger.
func TestAuditHomeEmit_PlanTransition(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	h := newTestHome()
	h.auditLogger = logger
	h.planStoreProject = "myproject"

	h.audit(auditlog.EventPlanTransition, "ready → implementing",
		auditlog.WithPlan("my-plan.md"),
	)

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "myproject",
		Kinds:   []auditlog.EventKind{auditlog.EventPlanTransition},
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "myproject", events[0].Project)
	assert.Equal(t, "my-plan.md", events[0].PlanFile)
	assert.Contains(t, events[0].Message, "implementing")
}

// TestAuditEmit_AgentSpawned verifies that the SQLiteLogger correctly stores and
// retrieves agent spawned events with all lifecycle fields populated.
func TestAuditEmit_AgentSpawned(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	// Simulate what spawnPlanAgent does
	logger.Emit(auditlog.Event{
		Kind:          auditlog.EventAgentSpawned,
		Project:       "test",
		PlanFile:      "plan.md",
		InstanceTitle: "plan-coder",
		AgentType:     "coder",
		WaveNumber:    1,
		TaskNumber:    2,
		Message:       "spawned coder for wave 1 task 2",
	})

	events, err := logger.Query(auditlog.QueryFilter{Project: "test", Limit: 10})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "coder", events[0].AgentType)
}

// TestAuditHomeEmit_AgentSpawned verifies that spawnAdHocAgent emits an
// EventAgentSpawned event through the home.audit() helper.
func TestAuditHomeEmit_AgentSpawned(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	h := newTestHome()
	h.auditLogger = logger
	h.planStoreProject = "myproject"

	// spawnAdHocAgent should emit EventAgentSpawned
	h.spawnAdHocAgent("my-fixer", "", "")

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "myproject",
		Kinds:   []auditlog.EventKind{auditlog.EventAgentSpawned},
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1, "spawnAdHocAgent must emit EventAgentSpawned")
	assert.Equal(t, "fixer", events[0].AgentType)
	assert.Equal(t, "my-fixer", events[0].InstanceTitle)
}

// TestAuditHomeEmit_AgentKilled verifies that executeContextAction("kill_instance")
// emits an EventAgentKilled event through the home.audit() helper.
func TestAuditHomeEmit_AgentKilled(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	h := newTestHome()
	h.auditLogger = logger
	h.planStoreProject = "myproject"

	// Add an instance to kill and select it
	inst, err := newTestInstance("my-agent")
	require.NoError(t, err)
	_ = h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	h.executeContextAction("kill_instance")

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "myproject",
		Kinds:   []auditlog.EventKind{auditlog.EventAgentKilled},
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1, "kill_instance must emit EventAgentKilled")
	assert.Equal(t, "my-agent", events[0].InstanceTitle)
}

func TestAuditHomeEmit_AgentKilled_KeybindK(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	h := newTestHome()
	h.auditLogger = logger
	h.planStoreProject = "myproject"
	h.keySent = true

	inst, err := newTestInstance("my-agent")
	require.NoError(t, err)
	inst.MarkStartedForTest()
	inst.SetStatus(session.Running)
	_ = h.nav.AddInstance(inst)
	h.nav.SelectInstance(inst)

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "myproject",
		Kinds:   []auditlog.EventKind{auditlog.EventAgentKilled},
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1, "k keybind must emit EventAgentKilled")
	assert.Equal(t, "my-agent", events[0].InstanceTitle)
	assert.Contains(t, events[0].Message, "killed instance")
}

func TestAuditHomeEmit_PlanCreated(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "plan-state.json"), []byte(`{}`), 0o644))

	h := newTestHome()
	h.auditLogger = logger
	h.planStoreProject = "myproject"
	h.planStateDir = plansDir

	err = h.createPlanEntry("my cool plan", "description", "")
	require.NoError(t, err)

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "myproject",
		Kinds:   []auditlog.EventKind{auditlog.EventPlanCreated},
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1, "createPlanEntry must emit EventPlanCreated")
	assert.Contains(t, events[0].Message, "created plan")
	assert.NotEmpty(t, events[0].PlanFile)
}

// TestAuditHomeEmit_AgentPaused verifies that the audit helper correctly emits
// an EventAgentPaused event. We test via the audit() helper directly because
// executeContextAction("pause_instance") requires a started tmux session.
func TestAuditHomeEmit_AgentPaused(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	h := newTestHomeWithToast()
	h.auditLogger = logger
	h.planStoreProject = "myproject"

	h.audit(auditlog.EventAgentPaused, "agent paused",
		auditlog.WithInstance("my-agent"),
		auditlog.WithAgent("coder"),
		auditlog.WithPlan("plan.md"),
	)

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "myproject",
		Kinds:   []auditlog.EventKind{auditlog.EventAgentPaused},
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1, "audit() must emit EventAgentPaused")
	assert.Equal(t, "my-agent", events[0].InstanceTitle)
}

// TestAuditHomeEmit_AgentResumed verifies that the audit helper correctly emits
// an EventAgentResumed event. We test via the audit() helper directly because
// executeContextAction("resume_instance") requires a started tmux session.
func TestAuditHomeEmit_AgentResumed(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	h := newTestHomeWithToast()
	h.auditLogger = logger
	h.planStoreProject = "myproject"

	h.audit(auditlog.EventAgentResumed, "agent resumed",
		auditlog.WithInstance("my-agent"),
		auditlog.WithAgent("coder"),
		auditlog.WithPlan("plan.md"),
	)

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "myproject",
		Kinds:   []auditlog.EventKind{auditlog.EventAgentResumed},
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1, "audit() must emit EventAgentResumed")
	assert.Equal(t, "my-agent", events[0].InstanceTitle)
}

// TestAuditEmit_PromptSent verifies that prompt sent events are stored and retrieved correctly.
func TestAuditEmit_PromptSent(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:          auditlog.EventPromptSent,
		Project:       "test",
		InstanceTitle: "my-agent",
		Message:       "implement the feature",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventPromptSent},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "implement the feature", events[0].Message)
}

// TestAuditEmit_GitPush verifies that git push events are stored and retrieved correctly.
func TestAuditEmit_GitPush(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:          auditlog.EventGitPush,
		Project:       "test",
		InstanceTitle: "my-agent",
		Message:       "pushed branch plan/my-feature",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventGitPush},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Contains(t, events[0].Message, "plan/my-feature")
}

// TestAuditEmit_PRCreated verifies that PR created events are stored and retrieved correctly.
func TestAuditEmit_PRCreated(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:          auditlog.EventPRCreated,
		Project:       "test",
		InstanceTitle: "my-agent",
		Message:       "PR created",
		Detail:        "https://github.com/org/repo/pull/42",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventPRCreated},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "https://github.com/org/repo/pull/42", events[0].Detail)
}

// TestAuditEmit_PermissionDetected verifies that permission detected events are stored correctly.
func TestAuditEmit_PermissionDetected(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:          auditlog.EventPermissionDetected,
		Project:       "test",
		InstanceTitle: "my-agent",
		Message:       "permission prompt detected",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventPermissionDetected},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "my-agent", events[0].InstanceTitle)
}

// TestAuditEmit_PermissionAnswered verifies that permission answered events are stored correctly.
func TestAuditEmit_PermissionAnswered(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:          auditlog.EventPermissionAnswered,
		Project:       "test",
		InstanceTitle: "my-agent",
		Message:       "allow once",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventPermissionAnswered},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "allow once", events[0].Message)
}

// TestAuditEmit_Error verifies that error events are stored with the correct level.
func TestAuditEmit_Error(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:    auditlog.EventError,
		Project: "test",
		Message: "something went wrong",
		Level:   "error",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventError},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "error", events[0].Level)
	assert.Equal(t, "something went wrong", events[0].Message)
}

// TestAuditEmit_FSMError verifies that FSM error events are stored correctly.
func TestAuditEmit_FSMError(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:     auditlog.EventFSMError,
		Project:  "test",
		PlanFile: "plan.md",
		Message:  "invalid transition: ready → done",
		Level:    "error",
	})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "test",
		Kinds:   []auditlog.EventKind{auditlog.EventFSMError},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Contains(t, events[0].Message, "invalid transition")
}

// TestAuditHomeEmit_WaveStarted verifies wave started events are emitted correctly.
func TestAuditHomeEmit_WaveStarted(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	h := newTestHome()
	h.auditLogger = logger
	h.planStoreProject = "myproject"

	h.audit(auditlog.EventWaveStarted, "wave 1 started: 2 task(s)",
		auditlog.WithPlan("my-plan.md"),
		auditlog.WithWave(1, 0),
	)

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "myproject",
		Kinds:   []auditlog.EventKind{auditlog.EventWaveStarted},
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, 1, events[0].WaveNumber)
	assert.Contains(t, events[0].Message, "wave 1")
}
