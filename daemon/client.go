package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/kastheco/kasmos/daemon/api"
)

// SocketClient is a client for the daemon control socket API. It communicates
// with the daemon over a Unix domain socket using JSON-over-HTTP.
type SocketClient struct {
	socketPath string
	http       *http.Client
}

// NewSocketClient creates a SocketClient that connects to the daemon's Unix
// domain socket at the given path.
func NewSocketClient(socketPath string) *SocketClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &SocketClient{
		socketPath: socketPath,
		http: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
	}
}

// Status queries GET /v1/status and returns the daemon's current status.
func (c *SocketClient) Status() (api.StatusResponse, error) {
	var resp api.StatusResponse
	if err := c.get("/v1/status", &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// ListRepos queries GET /v1/repos and returns the list of registered repos.
func (c *SocketClient) ListRepos() ([]api.RepoStatus, error) {
	var repos []api.RepoStatus
	if err := c.get("/v1/repos", &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// AddRepo sends POST /v1/repos to register a new repository path.
func (c *SocketClient) AddRepo(path string) error {
	body := struct {
		Path string `json:"path"`
	}{Path: path}
	return c.post("/v1/repos", body, nil)
}

// RemoveRepo sends DELETE /v1/repos/{project} to unregister a repository.
func (c *SocketClient) RemoveRepo(project string) error {
	req, err := http.NewRequest(http.MethodDelete, c.url("/v1/repos/"+project), nil)
	if err != nil {
		return fmt.Errorf("client: build request: %w", err)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("client: DELETE /v1/repos/%s: %w", project, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("client: DELETE /v1/repos/%s: status %d", project, res.StatusCode)
	}
	return nil
}

// StartPlan requests that the daemon spawn a planner for the given plan.
func (c *SocketClient) StartPlan(project, filename, prompt, program string) error {
	body := struct {
		Prompt  string `json:"prompt"`
		Program string `json:"program"`
	}{Prompt: prompt, Program: program}
	return c.post("/v1/repos/"+project+"/plans/"+filename+"/plan", body, nil)
}

// url returns the full HTTP URL for the given path, routed through the Unix socket.
// The host component is a placeholder since actual routing goes through the socket.
func (c *SocketClient) url(path string) string {
	return "http://daemon" + path
}

// get performs a GET request and decodes the JSON response into v.
func (c *SocketClient) get(path string, v any) error {
	res, err := c.http.Get(c.url(path))
	if err != nil {
		return fmt.Errorf("client: GET %s: %w", path, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("client: GET %s: status %d", path, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// post performs a POST request with a JSON body and optionally decodes the response.
func (c *SocketClient) post(path string, body any, v any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("client: marshal body: %w", err)
	}
	res, err := c.http.Post(c.url(path), "application/json", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("client: POST %s: %w", path, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("client: POST %s: status %d", path, res.StatusCode)
	}
	if v != nil {
		return json.NewDecoder(res.Body).Decode(v)
	}
	return nil
}
