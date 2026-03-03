package taskstore

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// NewHandler returns an http.Handler that exposes the Store over HTTP.
// It uses Go 1.22+ ServeMux pattern matching for method+path routing.
func NewHandler(store Store) http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, r *http.Request) {
		if err := store.Ping(); err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// List plans (with optional ?status= and ?topic= filters)
	mux.HandleFunc("GET /v1/projects/{project}/plans", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		statusFilters := r.URL.Query()["status"]
		topicFilter := r.URL.Query().Get("topic")

		var (
			plans []TaskEntry
			err   error
		)
		switch {
		case topicFilter != "":
			plans, err = store.ListByTopic(project, topicFilter)
		case len(statusFilters) > 0:
			statuses := make([]Status, len(statusFilters))
			for i, s := range statusFilters {
				statuses[i] = Status(s)
			}
			plans, err = store.ListByStatus(project, statuses...)
		default:
			plans, err = store.List(project)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, plans)
	})

	// Create plan
	mux.HandleFunc("POST /v1/projects/{project}/plans", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		var entry TaskEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		if err := store.Create(project, entry); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, entry)
	})

	// Get plan
	mux.HandleFunc("GET /v1/projects/{project}/plans/{filename}", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		filename := r.PathValue("filename")
		entry, err := store.Get(project, filename)
		if err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "plan not found: "+filename)
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, entry)
	})

	// Update plan
	mux.HandleFunc("PUT /v1/projects/{project}/plans/{filename}", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		filename := r.PathValue("filename")
		var entry TaskEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		if err := store.Update(project, filename, entry); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "plan not found: "+filename)
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, entry)
	})

	// Get plan content
	mux.HandleFunc("GET /v1/projects/{project}/plans/{filename}/content", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		filename := r.PathValue("filename")
		content, err := store.GetContent(project, filename)
		if err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "plan not found: "+filename)
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	})

	// Set plan content
	mux.HandleFunc("PUT /v1/projects/{project}/plans/{filename}/content", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		filename := r.PathValue("filename")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read request body: "+err.Error())
			return
		}
		if err := store.SetContent(project, filename, string(body)); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "plan not found: "+filename)
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Set ClickUp task ID
	mux.HandleFunc("PUT /v1/projects/{project}/plans/{filename}/clickup-task-id", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		filename := r.PathValue("filename")
		var req struct {
			ClickUpTaskID string `json:"clickup_task_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		if err := store.SetClickUpTaskID(project, filename, req.ClickUpTaskID); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "plan not found: "+filename)
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Increment review cycle
	mux.HandleFunc("POST /v1/projects/{project}/plans/{filename}/increment-review-cycle", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		filename := r.PathValue("filename")
		if err := store.IncrementReviewCycle(project, filename); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "plan not found: "+filename)
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Rename plan
	mux.HandleFunc("POST /v1/projects/{project}/plans/{filename}/rename", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		filename := r.PathValue("filename")
		var req struct {
			NewFilename string `json:"new_filename"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		if req.NewFilename == "" {
			writeError(w, http.StatusBadRequest, "new_filename is required")
			return
		}
		if err := store.Rename(project, filename, req.NewFilename); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "plan not found: "+filename)
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// List topics
	mux.HandleFunc("GET /v1/projects/{project}/topics", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		topics, err := store.ListTopics(project)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, topics)
	})

	// Create topic
	mux.HandleFunc("POST /v1/projects/{project}/topics", func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		var entry TopicEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		if err := store.CreateTopic(project, entry); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, entry)
	})

	return mux
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response with the given status code and message.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// isNotFound returns true if the error indicates a missing resource.
// Store implementations return errors containing "not found" for missing plans.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}
