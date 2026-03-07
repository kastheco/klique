package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
