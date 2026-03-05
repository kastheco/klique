package taskstore

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

// HTTPStore is a Store implementation that talks to a remote task store server
// over HTTP. Connection errors are wrapped with "task store unreachable" so
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
func (s *HTTPStore) taskURL(project string) string {
	return fmt.Sprintf("%s/v1/projects/%s/tasks", s.baseURL, url.PathEscape(project))
}

// taskItemURL builds the URL for a specific task entry.
func (s *HTTPStore) taskItemURL(project, filename string) string {
	return fmt.Sprintf("%s/v1/projects/%s/tasks/%s", s.baseURL, url.PathEscape(project), url.PathEscape(filename))
}

// taskContentURL builds the URL for a specific task's content endpoint.
func (s *HTTPStore) taskContentURL(project, filename string) string {
	return fmt.Sprintf("%s/v1/projects/%s/tasks/%s/content", s.baseURL, url.PathEscape(project), url.PathEscape(filename))
}

// taskSubtasksURL builds the URL for a task's subtasks endpoint.
func (s *HTTPStore) taskSubtasksURL(project, filename string) string {
	return fmt.Sprintf("%s/v1/projects/%s/tasks/%s/subtasks", s.baseURL, url.PathEscape(project), url.PathEscape(filename))
}

// taskSubtaskStatusURL builds the URL for a specific task's subtask status endpoint.
func (s *HTTPStore) taskSubtaskStatusURL(project, filename string, taskNumber int) string {
	return fmt.Sprintf("%s/v1/projects/%s/tasks/%s/subtasks/%d/status", s.baseURL, url.PathEscape(project), url.PathEscape(filename), taskNumber)
}

// taskPhaseTimestampURL builds the URL for phase timestamp updates.
func (s *HTTPStore) taskPhaseTimestampURL(project, filename string) string {
	return fmt.Sprintf("%s/v1/projects/%s/tasks/%s/phase-timestamp", s.baseURL, url.PathEscape(project), url.PathEscape(filename))
}

// taskGoalURL builds the URL for a plan goal update.
func (s *HTTPStore) taskGoalURL(project, filename string) string {
	return fmt.Sprintf("%s/v1/projects/%s/tasks/%s/goal", s.baseURL, url.PathEscape(project), url.PathEscape(filename))
}

// topicURL builds the base URL for a project's topics endpoint.
func (s *HTTPStore) topicURL(project string) string {
	return fmt.Sprintf("%s/v1/projects/%s/topics", s.baseURL, url.PathEscape(project))
}

// do executes an HTTP request and returns the response body.
// It wraps connection errors with "task store unreachable".
func (s *HTTPStore) do(req *http.Request) (*http.Response, error) {
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("task store unreachable: %w", err)
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
		return fmt.Errorf("task store: %s (status %d)", errResp.Error, resp.StatusCode)
	}
	return fmt.Errorf("task store: unexpected status %d", resp.StatusCode)
}

// Create adds a new task entry to the remote store.
func (s *HTTPStore) Create(project string, entry TaskEntry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("task store: marshal entry: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, s.taskURL(project), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
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

// Get retrieves a single task entry by filename.
func (s *HTTPStore) Get(project, filename string) (TaskEntry, error) {
	req, err := http.NewRequest(http.MethodGet, s.taskItemURL(project, filename), nil)
	if err != nil {
		return TaskEntry{}, fmt.Errorf("task store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return TaskEntry{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return TaskEntry{}, fmt.Errorf("task store: plan not found: %s", filename)
	}
	if resp.StatusCode != http.StatusOK {
		return TaskEntry{}, decodeError(resp)
	}

	var entry TaskEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return TaskEntry{}, fmt.Errorf("task store: decode response: %w", err)
	}
	return entry, nil
}

// Update replaces an existing task entry.
func (s *HTTPStore) Update(project, filename string, entry TaskEntry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("task store: marshal entry: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, s.taskItemURL(project, filename), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
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

// Rename renames a task entry from oldFilename to newFilename.
func (s *HTTPStore) Rename(project, oldFilename, newFilename string) error {
	payload := struct {
		NewFilename string `json:"new_filename"`
	}{NewFilename: newFilename}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("task store: marshal rename payload: %w", err)
	}
	renameURL := fmt.Sprintf("%s/rename", s.taskItemURL(project, oldFilename))
	req, err := http.NewRequest(http.MethodPost, renameURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
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

// GetContent retrieves the raw markdown content for a task.
func (s *HTTPStore) GetContent(project, filename string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, s.taskContentURL(project, filename), nil)
	if err != nil {
		return "", fmt.Errorf("task store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("task store: plan not found: %s", filename)
	}
	if resp.StatusCode != http.StatusOK {
		return "", decodeError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("task store: read content response: %w", err)
	}
	return string(body), nil
}

// SetContent replaces the raw markdown content for a task.
func (s *HTTPStore) SetContent(project, filename, content string) error {
	req, err := http.NewRequest(http.MethodPut, s.taskContentURL(project, filename), strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("task store: plan not found: %s", filename)
	}
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	return nil
}

// GetSubtasks is currently not implemented over HTTP.
func (s *HTTPStore) GetSubtasks(project, filename string) ([]SubtaskEntry, error) {
	req, err := http.NewRequest(http.MethodGet, s.taskSubtasksURL(project, filename), nil)
	if err != nil {
		return nil, fmt.Errorf("task store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var subtasks []SubtaskEntry
	if err := json.NewDecoder(resp.Body).Decode(&subtasks); err != nil {
		return nil, fmt.Errorf("task store: decode subtasks: %w", err)
	}
	return subtasks, nil
}

// SetSubtasks is currently not implemented over HTTP.
func (s *HTTPStore) SetSubtasks(project, filename string, subtasks []SubtaskEntry) error {
	if subtasks == nil {
		subtasks = []SubtaskEntry{}
	}
	body, err := json.Marshal(subtasks)
	if err != nil {
		return fmt.Errorf("task store: marshal subtasks: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, s.taskSubtasksURL(project, filename), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
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

// UpdateSubtaskStatus is currently not implemented over HTTP.
func (s *HTTPStore) UpdateSubtaskStatus(project, filename string, taskNumber int, status SubtaskStatus) error {
	body, err := json.Marshal(struct {
		Status SubtaskStatus `json:"status"`
	}{Status: status})
	if err != nil {
		return fmt.Errorf("task store: marshal subtask status payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, s.taskSubtaskStatusURL(project, filename, taskNumber), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
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

// SetPhaseTimestamp is currently not implemented over HTTP.
func (s *HTTPStore) SetPhaseTimestamp(project, filename, phase string, ts time.Time) error {
	body, err := json.Marshal(struct {
		Phase string    `json:"phase"`
		TS    time.Time `json:"timestamp,omitempty"`
	}{Phase: phase, TS: ts})
	if err != nil {
		return fmt.Errorf("task store: marshal phase timestamp payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, s.taskPhaseTimestampURL(project, filename), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
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

// List returns all task entries for the given project.
func (s *HTTPStore) List(project string) ([]TaskEntry, error) {
	req, err := http.NewRequest(http.MethodGet, s.taskURL(project), nil)
	if err != nil {
		return nil, fmt.Errorf("task store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var plans []TaskEntry
	if err := json.NewDecoder(resp.Body).Decode(&plans); err != nil {
		return nil, fmt.Errorf("task store: decode response: %w", err)
	}
	return plans, nil
}

// ListByStatus returns task entries filtered by one or more statuses.
func (s *HTTPStore) ListByStatus(project string, statuses ...Status) ([]TaskEntry, error) {
	u, err := url.Parse(s.taskURL(project))
	if err != nil {
		return nil, fmt.Errorf("task store: build URL: %w", err)
	}

	q := u.Query()
	for _, st := range statuses {
		q.Add("status", string(st))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("task store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var plans []TaskEntry
	if err := json.NewDecoder(resp.Body).Decode(&plans); err != nil {
		return nil, fmt.Errorf("task store: decode response: %w", err)
	}
	return plans, nil
}

// ListByTopic returns task entries for a specific topic.
func (s *HTTPStore) ListByTopic(project, topic string) ([]TaskEntry, error) {
	u, err := url.Parse(s.taskURL(project))
	if err != nil {
		return nil, fmt.Errorf("task store: build URL: %w", err)
	}

	q := u.Query()
	q.Set("topic", topic)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("task store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(resp)
	}

	var plans []TaskEntry
	if err := json.NewDecoder(resp.Body).Decode(&plans); err != nil {
		return nil, fmt.Errorf("task store: decode response: %w", err)
	}
	return plans, nil
}

// ListTopics returns all topic entries for the given project.
func (s *HTTPStore) ListTopics(project string) ([]TopicEntry, error) {
	req, err := http.NewRequest(http.MethodGet, s.topicURL(project), nil)
	if err != nil {
		return nil, fmt.Errorf("task store: build request: %w", err)
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
		return nil, fmt.Errorf("task store: decode response: %w", err)
	}
	return topics, nil
}

// CreateTopic adds a new topic entry to the remote store.
func (s *HTTPStore) CreateTopic(project string, entry TopicEntry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("task store: marshal topic: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, s.topicURL(project), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
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

// SetClickUpTaskID sets the ClickUp task ID for an existing task entry.
func (s *HTTPStore) SetClickUpTaskID(project, filename, taskID string) error {
	payload := struct {
		ClickUpTaskID string `json:"clickup_task_id"`
	}{ClickUpTaskID: taskID}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("task store: marshal clickup task id: %w", err)
	}
	u := fmt.Sprintf("%s/clickup-task-id", s.taskItemURL(project, filename))
	req, err := http.NewRequest(http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("task store: plan not found: %s", filename)
	}
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	return nil
}

// IncrementReviewCycle increments the review cycle counter for an existing task entry.
func (s *HTTPStore) IncrementReviewCycle(project, filename string) error {
	u := fmt.Sprintf("%s/increment-review-cycle", s.taskItemURL(project, filename))
	req, err := http.NewRequest(http.MethodPost, u, nil)
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
	}

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("task store: plan not found: %s", filename)
	}
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	return nil
}

// SetPlanGoal is currently not implemented over HTTP.
func (s *HTTPStore) SetPlanGoal(project, filename, goal string) error {
	body, err := json.Marshal(struct {
		Goal string `json:"goal"`
	}{Goal: goal})
	if err != nil {
		return fmt.Errorf("task store: marshal plan goal payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, s.taskGoalURL(project, filename), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("task store: build request: %w", err)
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
		return fmt.Errorf("task store: build ping request: %w", err)
	}

	resp, err := pingClient.Do(req)
	if err != nil {
		return fmt.Errorf("task store unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("task store: ping returned status %d", resp.StatusCode)
	}
	return nil
}
