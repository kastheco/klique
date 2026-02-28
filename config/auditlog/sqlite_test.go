package auditlog_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/planstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteLogger_EmitAndQuery(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:          auditlog.EventAgentSpawned,
		Project:       "testproj",
		PlanFile:      "plan.md",
		InstanceTitle: "plan-coder",
		AgentType:     "coder",
		Message:       "spawned coder agent",
	})

	events, err := logger.Query(auditlog.QueryFilter{Project: "testproj", Limit: 10})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, auditlog.EventAgentSpawned, events[0].Kind)
	assert.Equal(t, "plan-coder", events[0].InstanceTitle)
	assert.False(t, events[0].Timestamp.IsZero())
}

func TestSQLiteLogger_QueryFilterByPlan(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentSpawned, Project: "p", PlanFile: "a.md"})
	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentSpawned, Project: "p", PlanFile: "b.md"})

	events, err := logger.Query(auditlog.QueryFilter{Project: "p", PlanFile: "a.md", Limit: 10})
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestSQLiteLogger_QueryFilterByKind(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentSpawned, Project: "p"})
	logger.Emit(auditlog.Event{Kind: auditlog.EventPlanTransition, Project: "p"})

	events, err := logger.Query(auditlog.QueryFilter{
		Project: "p",
		Kinds:   []auditlog.EventKind{auditlog.EventPlanTransition},
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, auditlog.EventPlanTransition, events[0].Kind)
}

func TestSQLiteLogger_QueryOrderDesc(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentSpawned, Project: "p", Message: "first"})
	time.Sleep(time.Millisecond)
	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentFinished, Project: "p", Message: "second"})

	events, err := logger.Query(auditlog.QueryFilter{Project: "p", Limit: 10})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "second", events[0].Message) // newest first
}

func TestSQLiteLogger_SharedDB(t *testing.T) {
	// Verify the logger can be opened on the same DB path as planstore
	// (separate table, no conflicts)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "store.db")

	store, err := planstore.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	logger, err := auditlog.NewSQLiteLogger(dbPath)
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentSpawned, Project: "p", Message: "test"})
	events, err := logger.Query(auditlog.QueryFilter{Project: "p", Limit: 1})
	require.NoError(t, err)
	assert.Len(t, events, 1)
}
