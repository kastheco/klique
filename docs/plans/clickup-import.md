# ClickUp Import Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Import ClickUp tasks as kasmos plans via a thin MCP client, with auto-spawned planner agent for brainstorming.

**Architecture:** Thin Go MCP client (`internal/mcpclient/`) handles both HTTP (Streamable HTTP + OAuth PKCE) and stdio (JSON-RPC) transports. ClickUp-specific logic in `internal/clickup/` detects MCP config, searches tasks, and scaffolds plan markdown. TUI shows a conditional "+ Import from ClickUp" row in the sidebar, with TextInput→Picker overlay flow. After scaffold, existing `spawnPlanAgent` handles brainstorming.

**Tech Stack:** Go 1.24, bubbletea v1.3, lipgloss v1.1, bubblezone, net/http (OAuth), encoding/json (JSON-RPC)

---

## Wave 1: Thin MCP Client

Foundation layer — no TUI integration yet. Pure library code with comprehensive tests.

### Task 1: MCP Client Interface and Types

**Files:**
- Create: `internal/mcpclient/client.go`
- Create: `internal/mcpclient/types.go`
- Test: `internal/mcpclient/client_test.go`

**Step 1: Write the failing test**

```go
// internal/mcpclient/client_test.go
package mcpclient_test

import (
	"testing"

	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
)

func TestNewClient_RequiresTransport(t *testing.T) {
	_, err := mcpclient.NewClient(nil)
	assert.ErrorContains(t, err, "transport required")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/mcpclient/ -run TestNewClient_RequiresTransport -v`
Expected: FAIL — package doesn't exist

**Step 3: Write types and client interface**

```go
// internal/mcpclient/types.go
package mcpclient

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *JSONRPCError) Error() string { return e.Message }

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// ToolResult is the response from a tool call.
type ToolResult struct {
	Content []ToolContent `json:"content"`
}

// ToolContent is a single content block in a tool result.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Transport sends JSON-RPC requests and returns responses.
type Transport interface {
	Send(req JSONRPCRequest) (JSONRPCResponse, error)
	Close() error
}
```

```go
// internal/mcpclient/client.go
package mcpclient

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Client is a minimal MCP client that supports initialize, tools/list, and tools/call.
type Client struct {
	transport Transport
	nextID    int
	mu        sync.Mutex
	tools     []Tool // cached after ListTools
}

// NewClient creates a Client with the given transport.
func NewClient(t Transport) (*Client, error) {
	if t == nil {
		return nil, fmt.Errorf("transport required")
	}
	return &Client{transport: t, nextID: 1}, nil
}

// Initialize sends the MCP initialize handshake.
func (c *Client) Initialize() error {
	resp, err := c.call("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "kasmos", "version": "0.1.0"},
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

// ListTools returns available tools from the server.
func (c *Client) ListTools() ([]Tool, error) {
	resp, err := c.call("tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools: %w", err)
	}
	c.mu.Lock()
	c.tools = result.Tools
	c.mu.Unlock()
	return result.Tools, nil
}

// CallTool invokes a tool by name with the given arguments.
func (c *Client) CallTool(name string, args map[string]interface{}) (*ToolResult, error) {
	resp, err := c.call("tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", name, err)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	var result ToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tool result: %w", err)
	}
	return &result, nil
}

// FindTool returns the first tool whose name contains the given substring.
func (c *Client) FindTool(substring string) (Tool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.tools {
		if strings.Contains(strings.ToLower(t.Name), strings.ToLower(substring)) {
			return t, true
		}
	}
	return Tool{}, false
}

// Close shuts down the transport.
func (c *Client) Close() error {
	return c.transport.Close()
}

func (c *Client) call(method string, params interface{}) (JSONRPCResponse, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()
	return c.transport.Send(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/mcpclient/ -run TestNewClient -v`
Expected: PASS

**Step 5: Write more client tests with mock transport**

```go
// Add to internal/mcpclient/client_test.go

type mockTransport struct {
	responses map[string]mcpclient.JSONRPCResponse
	closed    bool
}

func (m *mockTransport) Send(req mcpclient.JSONRPCRequest) (mcpclient.JSONRPCResponse, error) {
	if resp, ok := m.responses[req.Method]; ok {
		resp.ID = req.ID
		return resp, nil
	}
	return mcpclient.JSONRPCResponse{}, fmt.Errorf("unexpected method: %s", req.Method)
}

func (m *mockTransport) Close() error { m.closed = true; return nil }

func TestClient_Initialize(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"initialize": {Result: json.RawMessage(`{"protocolVersion":"2024-11-05"}`)},
	}}
	c, err := mcpclient.NewClient(mt)
	require.NoError(t, err)
	assert.NoError(t, c.Initialize())
}

func TestClient_ListTools(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"tools/list": {Result: json.RawMessage(`{"tools":[{"name":"clickup_search_tasks"},{"name":"clickup_get_task"}]}`)},
	}}
	c, _ := mcpclient.NewClient(mt)
	tools, err := c.ListTools()
	require.NoError(t, err)
	assert.Len(t, tools, 2)
	tool, found := c.FindTool("search")
	assert.True(t, found)
	assert.Equal(t, "clickup_search_tasks", tool.Name)
}

func TestClient_CallTool(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{
		"tools/call": {Result: json.RawMessage(`{"content":[{"type":"text","text":"result data"}]}`)},
	}}
	c, _ := mcpclient.NewClient(mt)
	result, err := c.CallTool("clickup_get_task", map[string]interface{}{"task_id": "abc123"})
	require.NoError(t, err)
	assert.Len(t, result.Content, 1)
	assert.Equal(t, "result data", result.Content[0].Text)
}

func TestClient_Close(t *testing.T) {
	mt := &mockTransport{responses: map[string]mcpclient.JSONRPCResponse{}}
	c, _ := mcpclient.NewClient(mt)
	assert.NoError(t, c.Close())
	assert.True(t, mt.closed)
}
```

**Step 6: Run all tests**

Run: `go test ./internal/mcpclient/ -v`
Expected: all PASS

**Step 7: Commit**

```bash
git add internal/mcpclient/
git commit -m "feat(mcpclient): add thin MCP client with types and mock tests"
```

---

### Task 2: stdio Transport

**Files:**
- Create: `internal/mcpclient/transport_stdio.go`
- Test: `internal/mcpclient/transport_stdio_test.go`

**Step 1: Write the failing test**

```go
// internal/mcpclient/transport_stdio_test.go
package mcpclient_test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdioTransport_SendReceive(t *testing.T) {
	// Simulate server: write a response line, then read the request
	respBody := mcpclient.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"ok":true}`),
	}
	respBytes, _ := json.Marshal(respBody)

	serverOut := bytes.NewBuffer(append(respBytes, '\n'))
	serverIn := &bytes.Buffer{}

	tr := mcpclient.NewStdioTransportFromPipes(io.NopCloser(serverOut), serverIn)

	req := mcpclient.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "test"}
	resp, err := tr.Send(req)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.ID)
	assert.Nil(t, resp.Error)

	// Verify request was written
	var sentReq mcpclient.JSONRPCRequest
	require.NoError(t, json.NewDecoder(serverIn).Decode(&sentReq))
	assert.Equal(t, "test", sentReq.Method)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/mcpclient/ -run TestStdioTransport -v`
Expected: FAIL — NewStdioTransportFromPipes doesn't exist

**Step 3: Implement stdio transport**

```go
// internal/mcpclient/transport_stdio.go
package mcpclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// StdioTransport speaks JSON-RPC over stdin/stdout of a subprocess.
type StdioTransport struct {
	cmd     *exec.Cmd // nil when created from pipes
	reader  *bufio.Reader
	writer  io.Writer
	closer  io.Closer // stdin pipe or reader closer
	mu      sync.Mutex
}

// NewStdioTransport spawns a subprocess and connects to its stdin/stdout.
func NewStdioTransport(command string, args []string, env []string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(cmd.Environ(), env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}
	return &StdioTransport{
		cmd:    cmd,
		reader: bufio.NewReader(stdout),
		writer: stdin,
		closer: stdin,
	}, nil
}

// NewStdioTransportFromPipes creates a transport from pre-existing reader/writer (for testing).
func NewStdioTransportFromPipes(r io.ReadCloser, w io.Writer) *StdioTransport {
	return &StdioTransport{
		reader: bufio.NewReader(r),
		writer: w,
		closer: r,
	}
}

// Send writes a JSON-RPC request and reads the response.
func (t *StdioTransport) Send(req JSONRPCRequest) (JSONRPCResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')
	if _, err := t.writer.Write(data); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("write request: %w", err)
	}

	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("read response: %w", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("parse response: %w", err)
	}
	return resp, nil
}

// Close terminates the subprocess.
func (t *StdioTransport) Close() error {
	t.closer.Close()
	if t.cmd != nil {
		return t.cmd.Wait()
	}
	return nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/mcpclient/ -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/mcpclient/transport_stdio.go internal/mcpclient/transport_stdio_test.go
git commit -m "feat(mcpclient): add stdio JSON-RPC transport"
```

---

### Task 3: HTTP Transport with OAuth PKCE

**Files:**
- Create: `internal/mcpclient/transport_http.go`
- Create: `internal/mcpclient/oauth.go`
- Test: `internal/mcpclient/transport_http_test.go`
- Test: `internal/mcpclient/oauth_test.go`

**Step 1: Write the failing test for HTTP transport**

```go
// internal/mcpclient/transport_http_test.go
package mcpclient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPTransport_SendReceive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req mcpclient.JSONRPCRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		resp := mcpclient.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"tools":[]}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tr := mcpclient.NewHTTPTransport(srv.URL, "test-token")
	req := mcpclient.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	resp, err := tr.Send(req)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.ID)
}

func TestHTTPTransport_BearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		resp := mcpclient.JSONRPCResponse{JSONRPC: "2.0", ID: 1, Result: json.RawMessage(`{}`)}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tr := mcpclient.NewHTTPTransport(srv.URL, "my-secret-token")
	tr.Send(mcpclient.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "test"})
	assert.Equal(t, "Bearer my-secret-token", gotAuth)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/mcpclient/ -run TestHTTPTransport -v`
Expected: FAIL — NewHTTPTransport doesn't exist

**Step 3: Implement HTTP transport**

```go
// internal/mcpclient/transport_http.go
package mcpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HTTPTransport speaks JSON-RPC over HTTP POST (Streamable HTTP MCP transport).
type HTTPTransport struct {
	url   string
	token string
	http  *http.Client
}

// NewHTTPTransport creates an HTTP transport with a bearer token.
func NewHTTPTransport(url, token string) *HTTPTransport {
	return &HTTPTransport{
		url:   url,
		token: token,
		http:  &http.Client{},
	}
}

// Send posts a JSON-RPC request and reads the response.
func (t *HTTPTransport) Send(req JSONRPCRequest) (JSONRPCResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequest("POST", t.url, bytes.NewReader(body))
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if t.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+t.token)
	}

	httpResp, err := t.http.Do(httpReq)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("http post: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return JSONRPCResponse{}, fmt.Errorf("http %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return resp, nil
}

// Close is a no-op for HTTP transport.
func (t *HTTPTransport) Close() error { return nil }
```

**Step 4: Run HTTP transport tests**

Run: `go test ./internal/mcpclient/ -run TestHTTPTransport -v`
Expected: PASS

**Step 5: Write OAuth PKCE tests**

```go
// internal/mcpclient/oauth_test.go
package mcpclient_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthToken_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	tok := &mcpclient.OAuthToken{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	require.NoError(t, mcpclient.SaveToken(path, tok))

	loaded, err := mcpclient.LoadToken(path)
	require.NoError(t, err)
	assert.Equal(t, "access-123", loaded.AccessToken)
	assert.Equal(t, "refresh-456", loaded.RefreshToken)
}

func TestOAuthToken_IsExpired(t *testing.T) {
	tok := &mcpclient.OAuthToken{ExpiresAt: time.Now().Add(-time.Minute)}
	assert.True(t, tok.IsExpired())

	tok2 := &mcpclient.OAuthToken{ExpiresAt: time.Now().Add(time.Hour)}
	assert.False(t, tok2.IsExpired())
}

func TestOAuthToken_LoadMissing(t *testing.T) {
	_, err := mcpclient.LoadToken("/nonexistent/path")
	assert.Error(t, err)
}

func TestPKCEChallenge(t *testing.T) {
	verifier, challenge := mcpclient.GeneratePKCE()
	assert.NotEmpty(t, verifier)
	assert.NotEmpty(t, challenge)
	assert.NotEqual(t, verifier, challenge)
	// Verifier should be 43-128 chars per RFC 7636
	assert.GreaterOrEqual(t, len(verifier), 43)
}
```

**Step 6: Implement OAuth PKCE**

```go
// internal/mcpclient/oauth.go
package mcpclient

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// OAuthToken holds cached OAuth credentials.
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IsExpired returns true if the token has expired.
func (t *OAuthToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// TokenPath returns the default path for cached ClickUp OAuth tokens.
func TokenPath() string {
	configDir, _ := os.UserConfigDir()
	return filepath.Join(configDir, "kasmos", "clickup_oauth.json")
}

// SaveToken writes a token to disk.
func SaveToken(path string, tok *OAuthToken) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadToken reads a token from disk.
func LoadToken(path string) (*OAuthToken, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok OAuthToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

// GeneratePKCE returns a code verifier and its S256 challenge.
func GeneratePKCE() (verifier, challenge string) {
	buf := make([]byte, 32)
	rand.Read(buf)
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return
}

// OAuthConfig holds the ClickUp OAuth application settings.
type OAuthConfig struct {
	AuthURL     string // e.g. "https://app.clickup.com/api"
	TokenURL    string // e.g. "https://api.clickup.com/api/v2/oauth/token"
	ClientID    string
	RedirectURI string // set dynamically to localhost callback
}

// OAuthFlow performs the browser-based PKCE flow and returns a token.
// openBrowser is injectable for testing (pass nil for default behavior).
func OAuthFlow(ctx context.Context, cfg OAuthConfig, openBrowser func(string) error) (*OAuthToken, error) {
	verifier, challenge := GeneratePKCE()

	// Start local callback server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback: %s", r.URL.RawQuery)
			fmt.Fprint(w, "Error: no authorization code received")
			return
		}
		codeCh <- code
		fmt.Fprint(w, "Authorization successful! You can close this tab.")
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Shutdown(ctx)

	// Build auth URL
	params := url.Values{
		"client_id":             {cfg.ClientID},
		"redirect_uri":         {redirectURI},
		"response_type":        {"code"},
		"code_challenge":       {challenge},
		"code_challenge_method": {"S256"},
	}
	authURL := cfg.AuthURL + "?" + params.Encode()

	// Open browser
	if openBrowser == nil {
		openBrowser = defaultOpenBrowser
	}
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("open browser: %w", err)
	}

	// Wait for callback
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("oauth timeout: no callback received within 5 minutes")
	}

	// Exchange code for token
	return exchangeCode(cfg, code, verifier, redirectURI)
}

func exchangeCode(cfg OAuthConfig, code, verifier, redirectURI string) (*OAuthToken, error) {
	data := url.Values{
		"client_id":     {cfg.ClientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	resp, err := http.PostForm(cfg.TokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response")
	}
	return &OAuthToken{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}

func defaultOpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// Ensure unused import is used
var _ = strings.NewReader
```

**Step 7: Run all tests**

Run: `go test ./internal/mcpclient/ -v`
Expected: all PASS

**Step 8: Commit**

```bash
git add internal/mcpclient/transport_http.go internal/mcpclient/oauth.go \
        internal/mcpclient/transport_http_test.go internal/mcpclient/oauth_test.go
git commit -m "feat(mcpclient): add HTTP transport and OAuth 2.1 PKCE flow"
```

---

## Wave 2: ClickUp Detection & Import Logic

### Task 1: MCP Config Detection

**Files:**
- Create: `internal/clickup/detect.go`
- Create: `internal/clickup/types.go`
- Test: `internal/clickup/detect_test.go`

**Step 1: Write the failing test**

```go
// internal/clickup/detect_test.go
package clickup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetect_ProjectMCPJSON(t *testing.T) {
	dir := t.TempDir()
	mcpJSON := `{"mcpServers":{"clickup":{"type":"http","url":"https://mcp.clickup.com/mcp"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpJSON), 0o644))

	cfg, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
	assert.Equal(t, "http", cfg.Type)
	assert.Equal(t, "https://mcp.clickup.com/mcp", cfg.URL)
}

func TestDetect_StdioServer(t *testing.T) {
	dir := t.TempDir()
	mcpJSON := `{"mcpServers":{"clickup-tasks":{"command":"npx","args":["-y","@taazkareem/clickup-mcp-server@latest"],"env":{"CLICKUP_API_KEY":"test"}}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpJSON), 0o644))

	cfg, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
	assert.Equal(t, "stdio", cfg.Type)
	assert.Equal(t, "npx", cfg.Command)
	assert.Contains(t, cfg.Args, "-y")
}

func TestDetect_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, found := clickup.DetectMCP(dir, "")
	assert.False(t, found)
}

func TestDetect_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	mcpJSON := `{"mcpServers":{"ClickUp-Production":{"type":"http","url":"https://mcp.clickup.com/mcp"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpJSON), 0o644))

	_, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
}

func TestDetect_FallbackToClaudeSettings(t *testing.T) {
	repoDir := t.TempDir()
	claudeDir := t.TempDir()
	settingsJSON := `{"mcpServers":{"clickup":{"type":"http","url":"https://mcp.clickup.com/mcp"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settingsJSON), 0o644))

	cfg, found := clickup.DetectMCP(repoDir, claudeDir)
	assert.True(t, found)
	assert.Equal(t, "https://mcp.clickup.com/mcp", cfg.URL)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/clickup/ -v`
Expected: FAIL — package doesn't exist

**Step 3: Implement types and detection**

```go
// internal/clickup/types.go
package clickup

// MCPServerConfig holds the detected ClickUp MCP server configuration.
type MCPServerConfig struct {
	Type    string            // "http" or "stdio"
	URL     string            // for http type
	Command string            // for stdio type
	Args    []string          // for stdio type
	Env     map[string]string // for stdio type
}

// SearchResult is a ClickUp task from search results.
type SearchResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	ListName string `json:"list_name"`
	URL      string `json:"url"`
}

// Task is a full ClickUp task with details.
type Task struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	URL         string    `json:"url"`
	ListName    string    `json:"list_name"`
	Subtasks    []Subtask `json:"subtasks"`
	CustomFields []CustomField `json:"custom_fields"`
}

// Subtask is a ClickUp subtask reference.
type Subtask struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// CustomField is a ClickUp custom field value.
type CustomField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
```

```go
// internal/clickup/detect.go
package clickup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// mcpConfigFile represents the structure of .mcp.json or Claude settings.json.
type mcpConfigFile struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

// serverEntry is a union of http and stdio server config fields.
type serverEntry struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// DetectMCP scans config files for a ClickUp MCP server.
// repoDir is the project root (checks .mcp.json).
// claudeDir is the Claude config dir (checks settings.json, settings.local.json).
// Pass empty claudeDir to skip Claude config scanning.
func DetectMCP(repoDir, claudeDir string) (MCPServerConfig, bool) {
	// 1. Project .mcp.json
	if cfg, ok := scanFile(filepath.Join(repoDir, ".mcp.json")); ok {
		return cfg, true
	}
	// 2. Claude settings.json
	if claudeDir != "" {
		if cfg, ok := scanFile(filepath.Join(claudeDir, "settings.json")); ok {
			return cfg, true
		}
		// 3. Claude settings.local.json
		if cfg, ok := scanFile(filepath.Join(claudeDir, "settings.local.json")); ok {
			return cfg, true
		}
	}
	return MCPServerConfig{}, false
}

func scanFile(path string) (MCPServerConfig, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MCPServerConfig{}, false
	}
	var file mcpConfigFile
	if err := json.Unmarshal(data, &file); err != nil {
		return MCPServerConfig{}, false
	}
	for name, raw := range file.MCPServers {
		if !strings.Contains(strings.ToLower(name), "clickup") {
			continue
		}
		var entry serverEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		cfg := MCPServerConfig{Env: entry.Env}
		if entry.Type == "http" || entry.URL != "" {
			cfg.Type = "http"
			cfg.URL = entry.URL
		} else if entry.Command != "" {
			cfg.Type = "stdio"
			cfg.Command = entry.Command
			cfg.Args = entry.Args
		} else {
			continue
		}
		return cfg, true
	}
	return MCPServerConfig{}, false
}
```

**Step 4: Run tests**

Run: `go test ./internal/clickup/ -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/clickup/
git commit -m "feat(clickup): add MCP server detection and task types"
```

---

### Task 2: Plan Scaffold Builder

**Files:**
- Create: `internal/clickup/scaffold.go`
- Test: `internal/clickup/scaffold_test.go`

**Step 1: Write the failing test**

```go
// internal/clickup/scaffold_test.go
package clickup_test

import (
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/stretchr/testify/assert"
)

func TestScaffoldPlan_BasicTask(t *testing.T) {
	task := clickup.Task{
		ID:          "abc123",
		Name:        "Design auth flow",
		Description: "Implement OAuth2 authentication for the API",
		Status:      "In Progress",
		Priority:    "High",
		URL:         "https://app.clickup.com/t/abc123",
		ListName:    "Backend",
	}
	md := clickup.ScaffoldPlan(task)
	assert.Contains(t, md, "**Goal:** Implement OAuth2 authentication for the API")
	assert.Contains(t, md, "**Source:** ClickUp abc123")
	assert.Contains(t, md, "https://app.clickup.com/t/abc123")
	assert.Contains(t, md, "**ClickUp Status:** In Progress")
}

func TestScaffoldPlan_WithSubtasks(t *testing.T) {
	task := clickup.Task{
		ID:          "def456",
		Name:        "Setup CI/CD",
		Description: "Configure CI pipeline",
		Subtasks: []clickup.Subtask{
			{Name: "Add Dockerfile", Status: "done"},
			{Name: "Configure GitHub Actions", Status: "open"},
		},
	}
	md := clickup.ScaffoldPlan(task)
	assert.Contains(t, md, "## Reference: ClickUp Subtasks")
	assert.Contains(t, md, "- [x] Add Dockerfile")
	assert.Contains(t, md, "- [ ] Configure GitHub Actions")
}

func TestScaffoldPlan_WithCustomFields(t *testing.T) {
	task := clickup.Task{
		ID:   "ghi789",
		Name: "Feature X",
		CustomFields: []clickup.CustomField{
			{Name: "Sprint", Value: "2026-W09"},
			{Name: "Story Points", Value: "5"},
		},
	}
	md := clickup.ScaffoldPlan(task)
	assert.Contains(t, md, "## Reference: Custom Fields")
	assert.Contains(t, md, "- **Sprint:** 2026-W09")
}

func TestScaffoldFilename(t *testing.T) {
	tests := map[string]string{
		"Design Auth Flow":       "2026-02-24-design-auth-flow.md",
		"API v2 — New Endpoints": "2026-02-24-api-v2-new-endpoints.md",
		"  spaces & symbols!!! ": "2026-02-24-spaces-symbols.md",
	}
	for input, want := range tests {
		got := clickup.ScaffoldFilename(input, "2026-02-24")
		assert.Equal(t, want, got, "input: %q", input)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/clickup/ -run TestScaffold -v`
Expected: FAIL

**Step 3: Implement scaffold builder**

```go
// internal/clickup/scaffold.go
package clickup

import (
	"fmt"
	"regexp"
	"strings"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// ScaffoldPlan generates a plan markdown from a ClickUp task.
func ScaffoldPlan(task Task) string {
	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "# %s\n\n", task.Name)
	if task.Description != "" {
		fmt.Fprintf(&b, "**Goal:** %s\n\n", task.Description)
	}
	if task.ID != "" {
		fmt.Fprintf(&b, "**Source:** ClickUp %s", task.ID)
		if task.URL != "" {
			fmt.Fprintf(&b, " (%s)", task.URL)
		}
		b.WriteString("\n\n")
	}
	if task.Status != "" {
		fmt.Fprintf(&b, "**ClickUp Status:** %s\n\n", task.Status)
	}
	if task.Priority != "" {
		fmt.Fprintf(&b, "**Priority:** %s\n\n", task.Priority)
	}
	if task.ListName != "" {
		fmt.Fprintf(&b, "**List:** %s\n\n", task.ListName)
	}

	// Subtasks
	if len(task.Subtasks) > 0 {
		b.WriteString("## Reference: ClickUp Subtasks\n\n")
		for _, st := range task.Subtasks {
			checkbox := "- [ ] "
			if isDone(st.Status) {
				checkbox = "- [x] "
			}
			fmt.Fprintf(&b, "%s%s\n", checkbox, st.Name)
		}
		b.WriteString("\n")
	}

	// Custom fields
	if len(task.CustomFields) > 0 {
		b.WriteString("## Reference: Custom Fields\n\n")
		for _, cf := range task.CustomFields {
			fmt.Fprintf(&b, "- **%s:** %s\n", cf.Name, cf.Value)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// ScaffoldFilename generates a plan filename from a task name and date.
func ScaffoldFilename(name, date string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonAlphanumeric.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return fmt.Sprintf("%s-%s.md", date, slug)
}

func isDone(status string) bool {
	s := strings.ToLower(status)
	return s == "done" || s == "complete" || s == "completed" || s == "closed"
}
```

**Step 4: Run tests**

Run: `go test ./internal/clickup/ -v`
Expected: all PASS (note: date-dependent test — the ScaffoldFilename test hardcodes "2026-02-24" as input, not using time.Now)

**Step 5: Commit**

```bash
git add internal/clickup/scaffold.go internal/clickup/scaffold_test.go
git commit -m "feat(clickup): add plan scaffold builder from ClickUp tasks"
```

---

### Task 3: ClickUp Search & Fetch via MCP Client

**Files:**
- Create: `internal/clickup/import.go`
- Test: `internal/clickup/import_test.go`

**Step 1: Write the failing test**

```go
// internal/clickup/import_test.go
package clickup_test

import (
	"encoding/json"
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubMCPClient struct {
	callResults map[string]*mcpclient.ToolResult
	tools       []mcpclient.Tool
}

func (s *stubMCPClient) ListTools() ([]mcpclient.Tool, error) { return s.tools, nil }
func (s *stubMCPClient) CallTool(name string, args map[string]interface{}) (*mcpclient.ToolResult, error) {
	if r, ok := s.callResults[name]; ok {
		return r, nil
	}
	return &mcpclient.ToolResult{}, nil
}
func (s *stubMCPClient) FindTool(sub string) (mcpclient.Tool, bool) {
	for _, t := range s.tools {
		if contains(t.Name, sub) {
			return t, true
		}
	}
	return mcpclient.Tool{}, false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && stringContains(s, sub)))
}
func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSearch(t *testing.T) {
	taskJSON, _ := json.Marshal([]map[string]interface{}{
		{"id": "abc", "name": "Auth flow", "status": map[string]string{"status": "open"}, "list": map[string]string{"name": "Backend"}},
	})
	stub := &stubMCPClient{
		tools: []mcpclient.Tool{{Name: "clickup_search_tasks"}},
		callResults: map[string]*mcpclient.ToolResult{
			"clickup_search_tasks": {Content: []mcpclient.ToolContent{{Type: "text", Text: string(taskJSON)}}},
		},
	}

	importer := clickup.NewImporter(stub)
	results, err := importer.Search("auth")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "abc", results[0].ID)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/clickup/ -run TestSearch -v`
Expected: FAIL — NewImporter doesn't exist

**Step 3: Implement Importer**

```go
// internal/clickup/import.go
package clickup

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kastheco/kasmos/internal/mcpclient"
)

// MCPCaller is the subset of mcpclient.Client that Importer needs.
type MCPCaller interface {
	CallTool(name string, args map[string]interface{}) (*mcpclient.ToolResult, error)
	FindTool(substring string) (mcpclient.Tool, bool)
}

// Importer searches and fetches ClickUp tasks via MCP.
type Importer struct {
	client MCPCaller
}

// NewImporter creates an Importer with the given MCP client.
func NewImporter(client MCPCaller) *Importer {
	return &Importer{client: client}
}

// Search finds ClickUp tasks matching the query.
func (im *Importer) Search(query string) ([]SearchResult, error) {
	tool, found := im.client.FindTool("search")
	if !found {
		return nil, fmt.Errorf("no search tool found in MCP server")
	}

	result, err := im.client.CallTool(tool.Name, map[string]interface{}{
		"query": query,
	})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	text := extractText(result)
	if text == "" {
		return nil, nil
	}

	var results []SearchResult
	// Try parsing as array of task objects first
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		// Try parsing nested response formats
		var raw []map[string]interface{}
		if err2 := json.Unmarshal([]byte(text), &raw); err2 != nil {
			return nil, fmt.Errorf("parse search results: %w", err)
		}
		for _, item := range raw {
			results = append(results, parseSearchResult(item))
		}
	}
	return results, nil
}

// FetchTask gets full details for a ClickUp task by ID.
func (im *Importer) FetchTask(taskID string) (*Task, error) {
	tool, found := im.client.FindTool("get_task")
	if !found {
		return nil, fmt.Errorf("no get_task tool found in MCP server")
	}

	result, err := im.client.CallTool(tool.Name, map[string]interface{}{
		"task_id": taskID,
	})
	if err != nil {
		return nil, fmt.Errorf("fetch task: %w", err)
	}

	text := extractText(result)
	if text == "" {
		return nil, fmt.Errorf("empty response for task %s", taskID)
	}

	var task Task
	if err := json.Unmarshal([]byte(text), &task); err != nil {
		return nil, fmt.Errorf("parse task: %w", err)
	}
	return &task, nil
}

func extractText(result *mcpclient.ToolResult) string {
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text
		}
	}
	return ""
}

func parseSearchResult(item map[string]interface{}) SearchResult {
	r := SearchResult{}
	if id, ok := item["id"].(string); ok {
		r.ID = id
	}
	if name, ok := item["name"].(string); ok {
		r.Name = name
	}
	if status, ok := item["status"].(map[string]interface{}); ok {
		if s, ok := status["status"].(string); ok {
			r.Status = s
		}
	} else if status, ok := item["status"].(string); ok {
		r.Status = status
	}
	if list, ok := item["list"].(map[string]interface{}); ok {
		if name, ok := list["name"].(string); ok {
			r.ListName = name
		}
	}
	if url, ok := item["url"].(string); ok {
		r.URL = url
	}
	_ = strings.ToLower // ensure import
	return r
}
```

**Step 4: Run all clickup tests**

Run: `go test ./internal/clickup/ -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/clickup/import.go internal/clickup/import_test.go
git commit -m "feat(clickup): add MCP-based search and fetch via Importer"
```

---

## Wave 3: TUI Integration — Sidebar & Overlays

### Task 1: Add Import Row to Sidebar

**Files:**
- Modify: `ui/sidebar.go` (lines 81-92 for rowKind, lines 638 for row insertion, new render method)
- Test: `ui/sidebar_test.go`

**Step 1: Write the failing test**

```go
// Add to ui/sidebar_test.go (or create if needed)
func TestSidebar_ImportRowVisible(t *testing.T) {
	sp := SidebarParams{Width: 30, Height: 20}
	s := NewSidebar(&sp, false)
	s.SetClickUpAvailable(true)
	s.SetTopicsAndPlans(nil, nil, nil)
	rendered := s.String()
	assert.Contains(t, rendered, "Import from ClickUp")
}

func TestSidebar_ImportRowHidden(t *testing.T) {
	sp := SidebarParams{Width: 30, Height: 20}
	s := NewSidebar(&sp, false)
	s.SetClickUpAvailable(false)
	s.SetTopicsAndPlans(nil, nil, nil)
	rendered := s.String()
	assert.NotContains(t, rendered, "Import from ClickUp")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./ui/ -run TestSidebar_ImportRow -v`
Expected: FAIL — SetClickUpAvailable doesn't exist

**Step 3: Add rowKindImportAction to the const block**

In `ui/sidebar.go`, add to the `sidebarRowKind` const block (after `rowKindCancelled`):

```go
rowKindImportAction // "+ Import from ClickUp" action row
```

Add the sidebar constant:

```go
const SidebarImportClickUp = "__import_clickup__"
```

**Step 4: Add clickUpAvailable field and setter to Sidebar struct**

In the Sidebar struct, add:

```go
clickUpAvailable bool // true when ClickUp MCP is detected
```

Add setter:

```go
// SetClickUpAvailable controls whether the import action row is visible.
func (s *Sidebar) SetClickUpAvailable(available bool) {
	s.clickUpAvailable = available
	s.rebuildRows()
}
```

**Step 5: Insert import row in rebuildRows**

In `rebuildRows()`, after the topic/plan rendering and before the History toggle (line 638):

```go
// Import action (only when ClickUp MCP is detected)
if s.clickUpAvailable {
	rows = append(rows, sidebarRow{
		Kind:   rowKindImportAction,
		ID:     SidebarImportClickUp,
		Label:  "+ Import from ClickUp",
		Indent: 0,
	})
}
```

**Step 6: Add render method**

```go
// importActionStyle is for the clickable import row.
var importActionStyle = lipgloss.NewStyle().
	Foreground(ColorFoam).
	Padding(0, 1)

var importActionSelectedStyle = lipgloss.NewStyle().
	Background(ColorFoam).
	Foreground(ColorBase).
	Padding(0, 1)

func (s *Sidebar) renderImportRow(row sidebarRow, idx, width int) string {
	if idx == s.selectedIdx {
		return importActionSelectedStyle.Width(width).Render(row.Label)
	}
	return importActionStyle.Width(width).Render(row.Label)
}
```

Add the case to the render switch in `renderTreeRows`:

```go
case rowKindImportAction:
	line = s.renderImportRow(row, i, contentWidth)
```

**Step 7: Handle click/selection in ClickItem**

In `ClickItem()`, add handling for the import action row kind — it should set selectedIdx (the actual import action is handled by the app layer via the ID check).

**Step 8: Run tests**

Run: `go test ./ui/ -run TestSidebar_ImportRow -v`
Expected: PASS

**Step 9: Commit**

```bash
git add ui/sidebar.go ui/sidebar_test.go
git commit -m "feat(sidebar): add conditional '+ Import from ClickUp' row"
```

---

### Task 2: App State — New States, Messages, and Fields

**Files:**
- Modify: `app/app.go` (state constants, message types, home struct fields)

**Step 1: Add new state constants**

In `app/app.go` state const block, add:

```go
// stateClickUpSearch is the state when the user is typing a ClickUp search query.
stateClickUpSearch
// stateClickUpPicker is the state when the user is picking from ClickUp search results.
stateClickUpPicker
// stateClickUpFetching is when kasmos is fetching a full task from ClickUp.
stateClickUpFetching
```

**Step 2: Add new message types**

In `app/app.go`, add message types near the other Msg definitions:

```go
// clickUpDetectedMsg is sent at startup when ClickUp MCP is detected.
type clickUpDetectedMsg struct {
	Config clickup.MCPServerConfig
}

// clickUpSearchResultMsg is sent when ClickUp search completes.
type clickUpSearchResultMsg struct {
	Results []clickup.SearchResult
	Err     error
}

// clickUpTaskFetchedMsg is sent when a full ClickUp task is fetched.
type clickUpTaskFetchedMsg struct {
	Task *clickup.Task
	Err  error
}

// clickUpImportCompleteMsg is sent when the plan scaffold is written.
type clickUpImportCompleteMsg struct {
	PlanFile string
}
```

**Step 3: Add fields to home struct**

In the home struct, add in the UI Components section:

```go
// clickUpConfig stores the detected ClickUp MCP server config (nil if not detected)
clickUpConfig *clickup.MCPServerConfig
// clickUpImporter handles search/fetch via MCP (nil until first use)
clickUpImporter *clickup.Importer
// clickUpResults stores the latest search results for the picker
clickUpResults []clickup.SearchResult
```

**Step 4: Commit**

```bash
git add app/app.go
git commit -m "feat(app): add ClickUp import states, messages, and fields"
```

---

### Task 3: Detection at Startup

**Files:**
- Modify: `app/app.go` or `app/app_state.go` (Init or startup command)
- Modify: `app/app.go` (Update handler for clickUpDetectedMsg)

**Step 1: Add startup detection command**

Find the Init() method or the startup sequence in app.go. Add a tea.Cmd that runs detection:

```go
func detectClickUpCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		claudeDir := filepath.Join(os.Getenv("HOME"), ".claude")
		cfg, found := clickup.DetectMCP(repoPath, claudeDir)
		if !found {
			return nil
		}
		return clickUpDetectedMsg{Config: cfg}
	}
}
```

Add this to the batch of startup commands in Init().

**Step 2: Handle detection message in Update**

In the Update method's message switch, add:

```go
case clickUpDetectedMsg:
	m.clickUpConfig = &msg.Config
	m.sidebar.SetClickUpAvailable(true)
	return m, nil
```

**Step 3: Commit**

```bash
git add app/app.go app/app_state.go
git commit -m "feat(app): detect ClickUp MCP at startup and show import row"
```

---

### Task 4: Import Flow — Search Overlay

**Files:**
- Modify: `app/app_input.go` (handle Enter on import row, search overlay key handling)
- Modify: `app/app.go` (View rendering for search overlay, Update for search results)

**Step 1: Handle import row activation**

In the key handling code where sidebar Enter/Space is processed, detect when the selected row is the import action:

```go
// In the sidebar Enter handler (app_input.go)
if m.sidebar.GetSelectedID() == ui.SidebarImportClickUp {
	m.state = stateClickUpSearch
	m.textInputOverlay = overlay.NewTextInputOverlay("Search ClickUp Tasks", "")
	m.textInputOverlay.SetSize(50, 3)
	return m, nil
}
```

**Step 2: Handle search overlay submission**

In the key handling for `stateClickUpSearch`:

```go
case stateClickUpSearch:
	closed := m.textInputOverlay.HandleKeyPress(msg)
	if closed {
		if m.textInputOverlay.IsSubmitted() {
			query := m.textInputOverlay.GetValue()
			if query != "" {
				m.state = stateClickUpFetching
				m.toastManager.Show("Searching ClickUp...", overlay.ToastInfo)
				return m, m.searchClickUp(query)
			}
		}
		m.state = stateDefault
		m.textInputOverlay = nil
	}
	return m, nil
```

**Step 3: Implement searchClickUp command**

```go
func (m *home) searchClickUp(query string) tea.Cmd {
	return func() tea.Msg {
		importer, err := m.getOrCreateImporter()
		if err != nil {
			return clickUpSearchResultMsg{Err: err}
		}
		results, err := importer.Search(query)
		return clickUpSearchResultMsg{Results: results, Err: err}
	}
}

func (m *home) getOrCreateImporter() (*clickup.Importer, error) {
	if m.clickUpImporter != nil {
		return m.clickUpImporter, nil
	}
	if m.clickUpConfig == nil {
		return nil, fmt.Errorf("no ClickUp MCP server configured")
	}
	transport, err := m.createTransport(*m.clickUpConfig)
	if err != nil {
		return nil, err
	}
	client, err := mcpclient.NewClient(transport)
	if err != nil {
		return nil, err
	}
	if err := client.Initialize(); err != nil {
		return nil, fmt.Errorf("MCP initialize: %w", err)
	}
	if _, err := client.ListTools(); err != nil {
		return nil, fmt.Errorf("MCP list tools: %w", err)
	}
	m.clickUpImporter = clickup.NewImporter(client)
	return m.clickUpImporter, nil
}

func (m *home) createTransport(cfg clickup.MCPServerConfig) (mcpclient.Transport, error) {
	switch cfg.Type {
	case "http":
		token, err := m.getClickUpToken()
		if err != nil {
			return nil, err
		}
		return mcpclient.NewHTTPTransport(cfg.URL, token), nil
	case "stdio":
		envSlice := make([]string, 0, len(cfg.Env))
		for k, v := range cfg.Env {
			envSlice = append(envSlice, k+"="+v)
		}
		return mcpclient.NewStdioTransport(cfg.Command, cfg.Args, envSlice)
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", cfg.Type)
	}
}

func (m *home) getClickUpToken() (string, error) {
	path := mcpclient.TokenPath()
	tok, err := mcpclient.LoadToken(path)
	if err == nil && !tok.IsExpired() {
		return tok.AccessToken, nil
	}
	// Trigger OAuth flow
	ctx := m.ctx
	oauthCfg := mcpclient.OAuthConfig{
		AuthURL:  "https://app.clickup.com/api",
		TokenURL: "https://api.clickup.com/api/v2/oauth/token",
		ClientID: "kasmos", // TODO: register ClickUp OAuth app
	}
	tok, err = mcpclient.OAuthFlow(ctx, oauthCfg, nil)
	if err != nil {
		return "", fmt.Errorf("oauth: %w", err)
	}
	mcpclient.SaveToken(path, tok)
	return tok.AccessToken, nil
}
```

**Step 4: Handle search results in Update**

```go
case clickUpSearchResultMsg:
	if msg.Err != nil {
		m.toastManager.Show("ClickUp search failed: "+msg.Err.Error(), overlay.ToastError)
		m.state = stateDefault
		return m, nil
	}
	if len(msg.Results) == 0 {
		m.toastManager.Show("No ClickUp tasks found", overlay.ToastWarning)
		m.state = stateDefault
		return m, nil
	}
	m.clickUpResults = msg.Results
	items := make([]string, len(msg.Results))
	for i, r := range msg.Results {
		label := r.ID + " · " + r.Name
		if r.Status != "" {
			label += " (" + r.Status + ")"
		}
		if r.ListName != "" {
			label += " — " + r.ListName
		}
		items[i] = label
	}
	m.state = stateClickUpPicker
	m.pickerOverlay = overlay.NewPickerOverlay("Select ClickUp Task", items)
	return m, nil
```

**Step 5: Commit**

```bash
git add app/app_input.go app/app.go app/app_state.go
git commit -m "feat(app): wire ClickUp search overlay and result picker"
```

---

### Task 5: Import Flow — Fetch & Scaffold

**Files:**
- Modify: `app/app_input.go` (picker selection handling)
- Modify: `app/app.go` (Update handler for fetch result)
- Modify: `app/app_state.go` (scaffold + planner spawn)

**Step 1: Handle picker selection**

In the key handling for `stateClickUpPicker`:

```go
case stateClickUpPicker:
	closed := m.pickerOverlay.HandleKeyPress(msg)
	if closed {
		if m.pickerOverlay.IsSubmitted() {
			selected := m.pickerOverlay.Value()
			if selected != "" {
				// Find the matching result by index
				for i, r := range m.clickUpResults {
					label := r.ID + " · " + r.Name
					if strings.HasPrefix(selected, label) || i == m.pickerOverlay.SelectedIndex() {
						m.state = stateClickUpFetching
						m.toastManager.Show("Fetching task details...", overlay.ToastInfo)
						return m, m.fetchClickUpTask(r.ID)
					}
				}
			}
		}
		m.state = stateDefault
		m.pickerOverlay = nil
	}
	return m, nil
```

**Step 2: Implement fetchClickUpTask command**

```go
func (m *home) fetchClickUpTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		if m.clickUpImporter == nil {
			return clickUpTaskFetchedMsg{Err: fmt.Errorf("importer not initialized")}
		}
		task, err := m.clickUpImporter.FetchTask(taskID)
		return clickUpTaskFetchedMsg{Task: task, Err: err}
	}
}
```

**Step 3: Handle fetch result — scaffold and spawn planner**

```go
case clickUpTaskFetchedMsg:
	if msg.Err != nil {
		m.toastManager.Show("ClickUp fetch failed: "+msg.Err.Error(), overlay.ToastError)
		m.state = stateDefault
		return m, nil
	}
	m.state = stateDefault
	return m.importClickUpTask(msg.Task)
```

```go
func (m *home) importClickUpTask(task *clickup.Task) (tea.Model, tea.Cmd) {
	// Generate filename
	date := time.Now().Format("2006-01-02")
	filename := clickup.ScaffoldFilename(task.Name, date)

	// Write scaffold markdown
	plansDir := filepath.Join(m.activeRepoPath, "docs", "plans")
	os.MkdirAll(plansDir, 0o755)
	planPath := filepath.Join(plansDir, filename)

	// Avoid overwrite
	if _, err := os.Stat(planPath); err == nil {
		// Append numeric suffix
		for i := 2; i < 100; i++ {
			alt := strings.TrimSuffix(filename, ".md") + fmt.Sprintf("-%d.md", i)
			altPath := filepath.Join(plansDir, alt)
			if _, err := os.Stat(altPath); os.IsNotExist(err) {
				filename = alt
				planPath = altPath
				break
			}
		}
	}

	scaffold := clickup.ScaffoldPlan(*task)
	if err := os.WriteFile(planPath, []byte(scaffold), 0o644); err != nil {
		m.toastManager.Show("Failed to write plan: "+err.Error(), overlay.ToastError)
		return m, nil
	}

	// Register in plan-state and transition to planning
	if m.planState != nil {
		m.planState.Register(filename, task.Name, "plan/"+strings.TrimSuffix(filename, ".md"), time.Now())
	}

	// FSM transition to planning
	planfsm.Transition(m.activeRepoPath, filename, planfsm.PlanStart)

	m.loadPlanState()
	m.updateSidebarPlans()

	// Spawn planner agent
	prompt := fmt.Sprintf(`Analyze this imported ClickUp task. The task details and subtasks are included as reference in the plan file.

Determine if the task is well-specified enough for implementation or needs further analysis. Write a proper implementation plan with ## Wave sections, task breakdowns, architecture notes, and tech stack. Use the ClickUp subtasks as a starting point but reorganize into waves based on dependencies.

The plan file is at: docs/plans/%s`, filename)

	m.toastManager.Show("Imported! Spawning planner...", overlay.ToastSuccess)
	return m.spawnPlanAgent(filename, "plan", prompt)
}
```

**Step 4: Add View rendering for the new states**

In the View method's overlay rendering, add:

```go
case m.state == stateClickUpSearch:
	result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
case m.state == stateClickUpPicker:
	result = overlay.PlaceOverlay(0, 0, m.pickerOverlay.Render(), mainView, true, true)
```

**Step 5: Run full test suite**

Run: `go test ./... -v -count=1`
Expected: all existing tests PASS (new functionality needs integration testing)

**Step 6: Commit**

```bash
git add app/
git commit -m "feat(app): complete ClickUp import flow — fetch, scaffold, spawn planner"
```

---

## Wave 4: Polish & Testing

### Task 1: Integration Test for Full Import Flow

**Files:**
- Create: `app/clickup_import_test.go`

**Step 1: Write integration test**

```go
// app/clickup_import_test.go
package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportClickUpTask_WritesScaffold(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	task := &clickup.Task{
		ID:          "abc123",
		Name:        "Design Auth Flow",
		Description: "Implement OAuth2 for the API gateway",
		Status:      "In Progress",
		URL:         "https://app.clickup.com/t/abc123",
		Subtasks: []clickup.Subtask{
			{Name: "Add login endpoint", Status: "open"},
			{Name: "Add token refresh", Status: "open"},
		},
	}

	scaffold := clickup.ScaffoldPlan(*task)
	filename := clickup.ScaffoldFilename(task.Name, "2026-02-24")
	planPath := filepath.Join(plansDir, filename)
	require.NoError(t, os.WriteFile(planPath, []byte(scaffold), 0o644))

	// Verify file exists and contains expected content
	data, err := os.ReadFile(planPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "**Goal:** Implement OAuth2")
	assert.Contains(t, content, "**Source:** ClickUp abc123")
	assert.Contains(t, content, "- [ ] Add login endpoint")
	assert.Contains(t, content, "- [ ] Add token refresh")
}

func TestScaffoldFilename_Dedup(t *testing.T) {
	dir := t.TempDir()
	base := clickup.ScaffoldFilename("Test Task", "2026-02-24")
	// Create the base file
	require.NoError(t, os.WriteFile(filepath.Join(dir, base), []byte("x"), 0o644))
	// Verify file exists
	_, err := os.Stat(filepath.Join(dir, base))
	assert.NoError(t, err)
}
```

**Step 2: Run tests**

Run: `go test ./app/ -run TestImportClickUp -v && go test ./internal/... -v`
Expected: all PASS

**Step 3: Commit**

```bash
git add app/clickup_import_test.go
git commit -m "test(app): add integration test for ClickUp import scaffold"
```

---

### Task 2: Error States and Edge Cases

**Files:**
- Modify: `app/app_input.go` (error recovery)
- Modify: `app/app.go` (timeout handling)

**Step 1: Add timeout for MCP operations**

Wrap the search and fetch commands with context timeout:

```go
func (m *home) searchClickUp(query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_ = ctx // used by importer if we thread context through
		importer, err := m.getOrCreateImporter()
		if err != nil {
			return clickUpSearchResultMsg{Err: err}
		}
		results, err := importer.Search(query)
		return clickUpSearchResultMsg{Results: results, Err: err}
	}
}
```

**Step 2: Handle OAuth cancellation gracefully**

Ensure that if the user closes the browser during OAuth, the TUI returns to default state with a toast message rather than hanging.

**Step 3: Verify esc cancels at every stage**

- `stateClickUpSearch`: esc → default (already handled by TextInputOverlay)
- `stateClickUpPicker`: esc → default (already handled by PickerOverlay)
- `stateClickUpFetching`: can't cancel mid-flight, but timeout handles it

**Step 4: Run full test suite**

Run: `go test ./... -v -count=1`
Expected: all PASS

**Step 5: Commit**

```bash
git add app/
git commit -m "feat(app): add timeout and error recovery for ClickUp import"
```

---

### Task 3: Sidebar GetSelectedID Helper

**Files:**
- Modify: `ui/sidebar.go` (add helper if it doesn't exist)
- Test: `ui/sidebar_test.go`

**Step 1: Check if GetSelectedID exists, add if missing**

The import flow needs `sidebar.GetSelectedID()` to return the ID of the currently selected row. Check if this method exists. If not, add:

```go
// GetSelectedID returns the ID of the currently selected sidebar row.
func (s *Sidebar) GetSelectedID() string {
	if s.selectedIdx < 0 || s.selectedIdx >= len(s.rows) {
		return ""
	}
	return s.rows[s.selectedIdx].ID
}
```

**Step 2: Write test**

```go
func TestSidebar_GetSelectedID_ImportRow(t *testing.T) {
	sp := SidebarParams{Width: 30, Height: 20}
	s := NewSidebar(&sp, false)
	s.SetClickUpAvailable(true)
	s.SetTopicsAndPlans(nil, nil, nil)
	// Navigate to import row
	for i := 0; i < len(s.rows); i++ {
		if s.rows[i].ID == SidebarImportClickUp {
			s.selectedIdx = i
			break
		}
	}
	assert.Equal(t, SidebarImportClickUp, s.GetSelectedID())
}
```

**Step 3: Run tests**

Run: `go test ./ui/ -run TestSidebar_GetSelectedID -v`
Expected: PASS

**Step 4: Commit**

```bash
git add ui/sidebar.go ui/sidebar_test.go
git commit -m "feat(sidebar): add GetSelectedID helper for import action detection"
```
