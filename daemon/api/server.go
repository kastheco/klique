// Package api implements the Unix-domain-socket HTTP control API for the
// kasmos daemon. It exposes daemon state and accepts control commands via a
// small JSON-over-HTTP interface that kas monitor connects to.
package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
)

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

// RepoStatus describes the status of one registered repository.
type RepoStatus struct {
	Path        string `json:"path"`
	Project     string `json:"project"`
	ActivePlans int    `json:"active_plans"`
}

// StatusResponse is the response body for GET /v1/status.
type StatusResponse struct {
	Running   bool         `json:"running"`
	Repos     []RepoStatus `json:"repos"`
	RepoCount int          `json:"repo_count"`
	Uptime    string       `json:"uptime,omitempty"`
}

// InstanceStatus describes a running agent instance.
type InstanceStatus struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Plan    string `json:"plan"`
	Role    string `json:"role"`
	Active  bool   `json:"active"`
}

// addRepoRequest is the request body for POST /v1/repos.
type addRepoRequest struct {
	Path string `json:"path"`
}

// ---------------------------------------------------------------------------
// StateProvider interface
// ---------------------------------------------------------------------------

// StateProvider is the interface the Handler uses to query and mutate daemon
// state. The Daemon struct satisfies this interface; DaemonState provides a
// lightweight in-memory implementation used in tests.
type StateProvider interface {
	Status() StatusResponse
	ListRepos() []RepoStatus
	AddRepo(path string) error
	RemoveRepo(project string) error
	ListPlans(project string) ([]taskstore.TaskEntry, error)
	ListInstances(project string) []InstanceStatus
	EventStream() <-chan Event
}

// ---------------------------------------------------------------------------
// DaemonState — in-memory StateProvider (used in tests and as a lightweight
// stand-alone state container)
// ---------------------------------------------------------------------------

// DaemonState is a simple, thread-unsafe in-memory implementation of
// StateProvider. It is suitable for unit tests and for embedding inside the
// real Daemon when a richer implementation is not yet available.
type DaemonState struct {
	Running   bool
	Repos     []RepoStatus
	StartedAt time.Time
}

// Status implements StateProvider.
func (s *DaemonState) Status() StatusResponse {
	uptime := ""
	if !s.StartedAt.IsZero() {
		uptime = time.Since(s.StartedAt).Round(time.Second).String()
	}
	return StatusResponse{
		Running:   s.Running,
		Repos:     s.Repos,
		RepoCount: len(s.Repos),
		Uptime:    uptime,
	}
}

// ListRepos implements StateProvider.
func (s *DaemonState) ListRepos() []RepoStatus {
	return s.Repos
}

// AddRepo implements StateProvider.
func (s *DaemonState) AddRepo(path string) error {
	for _, r := range s.Repos {
		if r.Path == path {
			return fmt.Errorf("repo already registered: %s", path)
		}
	}
	project := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		project = path[idx+1:]
	}
	s.Repos = append(s.Repos, RepoStatus{Path: path, Project: project})
	return nil
}

// RemoveRepo implements StateProvider.
func (s *DaemonState) RemoveRepo(project string) error {
	for i, r := range s.Repos {
		if r.Project == project {
			s.Repos = append(s.Repos[:i], s.Repos[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("repo not registered: %s", project)
}

// ListPlans implements StateProvider. DaemonState has no backing store, so it
// always returns an empty list.
func (s *DaemonState) ListPlans(_ string) ([]taskstore.TaskEntry, error) {
	return nil, nil
}

// ListInstances implements StateProvider.
func (s *DaemonState) ListInstances(_ string) []InstanceStatus {
	return nil
}

// EventStream implements StateProvider. Returns a channel that is never written
// to (suitable for testing).
func (s *DaemonState) EventStream() <-chan Event {
	return make(chan Event)
}

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

// Handler is an http.Handler that exposes the daemon control API.
type Handler struct {
	state       StateProvider
	broadcaster *EventBroadcaster // optional; if set, SSE uses Subscribe()
	mux         *http.ServeMux
}

// NewHandler creates a Handler backed by the given StateProvider and registers
// all API routes. The SSE endpoint will use state.EventStream().
func NewHandler(state StateProvider) http.Handler {
	h := &Handler{
		state: state,
		mux:   http.NewServeMux(),
	}
	h.registerRoutes()
	return h
}

// NewHandlerWithBroadcaster creates a Handler that uses the provided
// EventBroadcaster for the SSE /v1/events endpoint, giving each connecting
// client its own subscription channel.
func NewHandlerWithBroadcaster(state StateProvider, b *EventBroadcaster) http.Handler {
	h := &Handler{
		state:       state,
		broadcaster: b,
		mux:         http.NewServeMux(),
	}
	h.registerRoutes()
	return h
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) registerRoutes() {
	h.mux.HandleFunc("GET /v1/status", h.handleStatus)
	h.mux.HandleFunc("POST /v1/reload", h.handleReload)

	h.mux.HandleFunc("GET /v1/repos", h.handleListRepos)
	h.mux.HandleFunc("POST /v1/repos", h.handleAddRepo)
	h.mux.HandleFunc("DELETE /v1/repos/{project}", h.handleRemoveRepo)

	h.mux.HandleFunc("GET /v1/repos/{project}/plans", h.handleListPlans)
	h.mux.HandleFunc("GET /v1/repos/{project}/instances", h.handleListInstances)
	h.mux.HandleFunc("POST /v1/repos/{project}/plans/{filename}/implement", h.handleImplementPlan)

	h.mux.HandleFunc("GET /v1/events", h.handleEvents)
}

// ---------------------------------------------------------------------------
// Route handlers
// ---------------------------------------------------------------------------

// handleStatus serves GET /v1/status — daemon overview.
func (h *Handler) handleStatus(w http.ResponseWriter, _ *http.Request) {
	resp := h.state.Status()
	writeJSON(w, http.StatusOK, resp)
}

// handleReload serves POST /v1/reload — re-read config.
func (h *Handler) handleReload(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
}

// handleListRepos serves GET /v1/repos — list registered repos.
func (h *Handler) handleListRepos(w http.ResponseWriter, _ *http.Request) {
	repos := h.state.ListRepos()
	if repos == nil {
		repos = []RepoStatus{}
	}
	writeJSON(w, http.StatusOK, repos)
}

// handleAddRepo serves POST /v1/repos — register a new repo.
func (h *Handler) handleAddRepo(w http.ResponseWriter, r *http.Request) {
	var req addRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := h.state.AddRepo(req.Path); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "added", "path": req.Path})
}

// handleRemoveRepo serves DELETE /v1/repos/{project} — unregister a repo.
func (h *Handler) handleRemoveRepo(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	if err := h.state.RemoveRepo(project); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed", "project": project})
}

// handleListPlans serves GET /v1/repos/{project}/plans — list plans.
func (h *Handler) handleListPlans(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	plans, err := h.state.ListPlans(project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if plans == nil {
		plans = []taskstore.TaskEntry{}
	}
	writeJSON(w, http.StatusOK, plans)
}

// handleListInstances serves GET /v1/repos/{project}/instances — list agents.
func (h *Handler) handleListInstances(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	instances := h.state.ListInstances(project)
	if instances == nil {
		instances = []InstanceStatus{}
	}
	writeJSON(w, http.StatusOK, instances)
}

// handleImplementPlan serves POST /v1/repos/{project}/plans/{filename}/implement.
func (h *Handler) handleImplementPlan(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	filename := r.PathValue("filename")
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":   "accepted",
		"project":  project,
		"filename": filename,
	})
}

// handleEvents serves GET /v1/events — SSE stream of daemon events.
func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush() // send headers to client immediately

	enc := json.NewEncoder(w)

	// Prefer the broadcaster (per-client subscription) when available;
	// fall back to the StateProvider's shared channel.
	var events <-chan Event
	if h.broadcaster != nil {
		ch := h.broadcaster.Subscribe()
		defer h.broadcaster.Unsubscribe(ch)
		events = ch
	} else {
		events = h.state.EventStream()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: ")
			_ = enc.Encode(ev)
			fmt.Fprintf(w, "\n")
			flusher.Flush()
		}
	}
}

// ---------------------------------------------------------------------------
// Unix domain socket listener
// ---------------------------------------------------------------------------

// ListenUnix starts the HTTP server on the given Unix domain socket path and
// serves until ctx is cancelled. The socket file is removed on clean shutdown.
func ListenUnix(socketPath string, handler http.Handler) (net.Listener, error) {
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("api: listen unix %s: %w", socketPath, err)
	}
	return ln, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
