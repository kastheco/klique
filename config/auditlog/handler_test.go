package auditlog_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ListEvents(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{
		Kind:     auditlog.EventAgentSpawned,
		Project:  "myproject",
		TaskFile: "plan-foo",
		Message:  "spawned coder",
	})
	logger.Emit(auditlog.Event{
		Kind:     auditlog.EventWaveStarted,
		Project:  "myproject",
		TaskFile: "plan-foo",
		Message:  "wave 1 started",
	})

	srv := httptest.NewServer(auditlog.NewHandler(logger))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/projects/myproject/audit-events")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var events []auditlog.Event
	err = json.NewDecoder(resp.Body).Decode(&events)
	require.NoError(t, err)
	assert.Len(t, events, 2)
	// newest first
	assert.Equal(t, auditlog.EventWaveStarted, events[0].Kind)
}

func TestHandler_FilterByKind(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentSpawned, Project: "p"})
	logger.Emit(auditlog.Event{Kind: auditlog.EventWaveStarted, Project: "p"})

	srv := httptest.NewServer(auditlog.NewHandler(logger))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/projects/p/audit-events?kind=wave_started")
	require.NoError(t, err)
	var events []auditlog.Event
	json.NewDecoder(resp.Body).Decode(&events)
	assert.Len(t, events, 1)
	assert.Equal(t, auditlog.EventWaveStarted, events[0].Kind)
}

func TestHandler_FilterByTaskFile(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentSpawned, Project: "p", TaskFile: "plan-a"})
	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentSpawned, Project: "p", TaskFile: "plan-b"})

	srv := httptest.NewServer(auditlog.NewHandler(logger))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/projects/p/audit-events?task=plan-a")
	require.NoError(t, err)
	var events []auditlog.Event
	json.NewDecoder(resp.Body).Decode(&events)
	assert.Len(t, events, 1)
	assert.Equal(t, "plan-a", events[0].TaskFile)
}

func TestHandler_EmptyResultsReturnsArray(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	srv := httptest.NewServer(auditlog.NewHandler(logger))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/projects/empty/audit-events")
	require.NoError(t, err)

	var events []auditlog.Event
	json.NewDecoder(resp.Body).Decode(&events)
	assert.NotNil(t, events)
	assert.Len(t, events, 0)
}

func TestHandler_InvalidLimitReturnsBadRequest(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	srv := httptest.NewServer(auditlog.NewHandler(logger))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/projects/p/audit-events?limit=bad")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"], "invalid limit")
}

func TestHandler_MultipleKindFilters(t *testing.T) {
	logger, err := auditlog.NewSQLiteLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	logger.Emit(auditlog.Event{Kind: auditlog.EventAgentSpawned, Project: "p"})
	logger.Emit(auditlog.Event{Kind: auditlog.EventWaveStarted, Project: "p"})
	logger.Emit(auditlog.Event{Kind: auditlog.EventPlanCreated, Project: "p"})

	srv := httptest.NewServer(auditlog.NewHandler(logger))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/projects/p/audit-events?kind=agent_spawned&kind=wave_started")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var events []auditlog.Event
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&events))
	assert.Len(t, events, 2)
	kinds := make([]auditlog.EventKind, len(events))
	for i, e := range events {
		kinds[i] = e.Kind
	}
	assert.Contains(t, kinds, auditlog.EventAgentSpawned)
	assert.Contains(t, kinds, auditlog.EventWaveStarted)
}
