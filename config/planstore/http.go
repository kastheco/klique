package planstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPStore is a Store implementation that talks to a remote plan store server
// over HTTP. Connection errors are wrapped with "plan store unreachable" so
// callers can detect and surface them gracefully.
type HTTPStore struct {
	baseURL string
	project string
	client  *http.Client
}

// NewHTTPStore creates a new HTTPStore client pointing at baseURL.
// project is the default project name used when routing requests.
// The underlying http.Client has a 5-second timeout.
func NewHTTPStore(baseURL, project string) *HTTPStore {
	return &HTTPStore{
		baseURL: strings.TrimRight(baseURL, "/"),
		project: project,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// planURL builds the base URL for a project's plans endpoint.
func (s *HTTPStore) planURL(project string) string {
	return fmt.Sprintf("%s/v1/projects/%s/plans", s.baseURL, url.PathEscape(project))
}

// planItemURL builds the URL for a specific plan entry.
func (s *HTTPStore) planItemURL(project, filename string) string {
	return fmt.Sprintf("%s/v1/projects/%s/plans/%s", s.baseURL, url.PathEscape(project), url.PathEscape(filename))
}

// planContentURL builds the URL for a specific plan's content endpoint.
func (s *HTTPStore) planContentURL(project, filename string) string {
	return fmt.Sprintf("%s/v1/projects/%s/plans/%s/content", s.baseURL, url.PathEscape(project), url.PathEscape(filename))
}

// topicURL builds the base URL for a project's topics endpoint.
func (s *HTTPStore) topicURL(project string) string {
	return fmt.Sprintf("%s/v1/projects/%s/topics", s.baseURL, url.PathEscape(project))
}

// do executes an HTTP request and returns the response body.
// It wraps connection errors with "plan store unreachable".
func (s *HTTPStore) do(req *http.Request) (*http.Response, error) {
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plan store unreachable: %w", err)
	}
	return resp, nil
}

// decodeError reads an error response body and returns a formatted error.
func decodeError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var errResp struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return fmt.Errorf("plan store: %s (status %d)", errResp.Error, resp.StatusCode)
	}
	return fmt.Errorf("plan store: unexpected status %d", resp.StatusCode)
}

// Create adds a new plan entry to the remote store.
func (s *HTTPStore) Create(project string, entry PlanEntry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("plan store: marshal entry: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, s.planURL(project), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("plan store: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return decodeError(resp)
	}
	return nil
}

// Get retrieves a single plan entry by filename.
func (s *HTTPStore) Get(project, filename string) (PlanEntry, error) {
	req, err := http.NewRequest(http.MethodGet, s.planItemURL(project, filename), nil)
	if err != nil {
		return PlanEntry{}, fmt.Errorf("plan store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return PlanEntry{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return PlanEntry{}, fmt.Errorf("plan store: plan not found: %s", filename)
	}
	if resp.StatusCode != http.StatusOK {
		return PlanEntry{}, decodeError(resp)
	}

	var entry PlanEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return PlanEntry{}, fmt.Errorf("plan store: decode response: %w", err)
	}
	return entry, nil
}

// Update replaces an existing plan entry.
func (s *HTTPStore) Update(project, filename string, entry PlanEntry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("plan store: marshal entry: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, s.planItemURL(project, filename), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("plan store: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	return nil
}

// Rename renames a plan entry from oldFilename to newFilename.
func (s *HTTPStore) Rename(project, oldFilename, newFilename string) error {
	payload := struct {
		NewFilename string `json:"new_filename"`
	}{NewFilename: newFilename}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("plan store: marshal rename payload: %w", err)
	}
	renameURL := fmt.Sprintf("%s/rename", s.planItemURL(project, oldFilename))
	req, err := http.NewRequest(http.MethodPost, renameURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("plan store: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	return nil
}

// GetContent retrieves the raw markdown content for a plan.
func (s *HTTPStore) GetContent(project, filename string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, s.planContentURL(project, filename), nil)
	if err != nil {
		return "", fmt.Errorf("plan store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("plan store: plan not found: %s", filename)
	}
	if resp.StatusCode != http.StatusOK {
		return "", decodeError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("plan store: read content response: %w", err)
	}
	return string(body), nil
}

// SetContent replaces the raw markdown content for a plan.
func (s *HTTPStore) SetContent(project, filename, content string) error {
	req, err := http.NewRequest(http.MethodPut, s.planContentURL(project, filename), strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("plan store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("plan store: plan not found: %s", filename)
	}
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	return nil
}

// List returns all plan entries for the given project.
func (s *HTTPStore) List(project string) ([]PlanEntry, error) {
	req, err := http.NewRequest(http.MethodGet, s.planURL(project), nil)
	if err != nil {
		return nil, fmt.Errorf("plan store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var plans []PlanEntry
	if err := json.NewDecoder(resp.Body).Decode(&plans); err != nil {
		return nil, fmt.Errorf("plan store: decode response: %w", err)
	}
	return plans, nil
}

// ListByStatus returns plan entries filtered by one or more statuses.
func (s *HTTPStore) ListByStatus(project string, statuses ...Status) ([]PlanEntry, error) {
	u, err := url.Parse(s.planURL(project))
	if err != nil {
		return nil, fmt.Errorf("plan store: build URL: %w", err)
	}

	q := u.Query()
	for _, st := range statuses {
		q.Add("status", string(st))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("plan store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var plans []PlanEntry
	if err := json.NewDecoder(resp.Body).Decode(&plans); err != nil {
		return nil, fmt.Errorf("plan store: decode response: %w", err)
	}
	return plans, nil
}

// ListByTopic returns plan entries for a specific topic.
func (s *HTTPStore) ListByTopic(project, topic string) ([]PlanEntry, error) {
	u, err := url.Parse(s.planURL(project))
	if err != nil {
		return nil, fmt.Errorf("plan store: build URL: %w", err)
	}

	q := u.Query()
	q.Set("topic", topic)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("plan store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var plans []PlanEntry
	if err := json.NewDecoder(resp.Body).Decode(&plans); err != nil {
		return nil, fmt.Errorf("plan store: decode response: %w", err)
	}
	return plans, nil
}

// ListTopics returns all topic entries for the given project.
func (s *HTTPStore) ListTopics(project string) ([]TopicEntry, error) {
	req, err := http.NewRequest(http.MethodGet, s.topicURL(project), nil)
	if err != nil {
		return nil, fmt.Errorf("plan store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var topics []TopicEntry
	if err := json.NewDecoder(resp.Body).Decode(&topics); err != nil {
		return nil, fmt.Errorf("plan store: decode response: %w", err)
	}
	return topics, nil
}

// CreateTopic adds a new topic entry to the remote store.
func (s *HTTPStore) CreateTopic(project string, entry TopicEntry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("plan store: marshal topic: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, s.topicURL(project), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("plan store: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return decodeError(resp)
	}
	return nil
}

// SetClickUpTaskID sets the ClickUp task ID for an existing plan entry.
func (s *HTTPStore) SetClickUpTaskID(project, filename, taskID string) error {
	payload := struct {
		ClickUpTaskID string `json:"clickup_task_id"`
	}{ClickUpTaskID: taskID}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("plan store: marshal clickup task id: %w", err)
	}
	u := fmt.Sprintf("%s/clickup-task-id", s.planItemURL(project, filename))
	req, err := http.NewRequest(http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("plan store: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("plan store: plan not found: %s", filename)
	}
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	return nil
}

// Close is a no-op for HTTPStore — the HTTP client has no persistent connection
// to release. It exists to satisfy the Store interface.
func (s *HTTPStore) Close() error {
	return nil
}

// Ping checks connectivity to the remote store server.
// It uses a shorter 2-second timeout for health checks.
func (s *HTTPStore) Ping() error {
	pingClient := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodGet, s.baseURL+"/v1/ping", nil)
	if err != nil {
		return fmt.Errorf("plan store: build ping request: %w", err)
	}

	resp, err := pingClient.Do(req)
	if err != nil {
		return fmt.Errorf("plan store unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plan store: ping returned status %d", resp.StatusCode)
	}
	return nil
}
