package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kastheco/kasmos/config/taskstore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type startPlanStub struct {
	DaemonState
	project  string
	filename string
	prompt   string
	program  string
}

func (s *startPlanStub) ListPlans(_ string) ([]taskstore.TaskEntry, error) { return nil, nil }
func (s *startPlanStub) ListInstances(_ string) []InstanceStatus           { return nil }
func (s *startPlanStub) EventStream() <-chan Event                         { return make(chan Event) }
func (s *startPlanStub) StartPlan(project, filename, prompt, program string) error {
	s.project = project
	s.filename = filename
	s.prompt = prompt
	s.program = program
	return nil
}

func TestHandler_Status(t *testing.T) {
	state := &DaemonState{
		Running: true,
		Repos:   []RepoStatus{{Path: "/tmp/test", Project: "test", ActivePlans: 0}},
	}
	h := NewHandler(state)

	req := httptest.NewRequest("GET", "/v1/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp StatusResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp.Running)
	assert.Len(t, resp.Repos, 1)
}

func TestHandler_ListRepos(t *testing.T) {
	state := &DaemonState{
		Running: true,
		Repos:   []RepoStatus{{Path: "/tmp/a", Project: "a"}, {Path: "/tmp/b", Project: "b"}},
	}
	h := NewHandler(state)

	req := httptest.NewRequest("GET", "/v1/repos", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var repos []RepoStatus
	require.NoError(t, json.NewDecoder(w.Body).Decode(&repos))
	assert.Len(t, repos, 2)
}

func TestHandler_StartPlan(t *testing.T) {
	state := &startPlanStub{}
	h := NewHandler(state)

	body := bytes.NewBufferString(`{"prompt":"plan prompt","program":"opencode --model x"}`)
	req := httptest.NewRequest("POST", "/v1/repos/cms/plans/api-response-logging/plan", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, "cms", state.project)
	assert.Equal(t, "api-response-logging", state.filename)
	assert.Equal(t, "plan prompt", state.prompt)
	assert.Equal(t, "opencode --model x", state.program)
}
