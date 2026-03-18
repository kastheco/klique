package mcpserver_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/internal/mcpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodePayload reads a JSON-RPC response from rec. If the response
// content type starts with text/event-stream it pulls the first "data: "
// line; otherwise it decodes the body as JSON directly.
func decodePayload(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	ct := rec.Header().Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		scanner := bufio.NewScanner(rec.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				payload := strings.TrimPrefix(line, "data: ")
				require.NoError(t, json.Unmarshal([]byte(payload), out))
				return
			}
		}
		require.NoError(t, scanner.Err())
		t.Fatal("no data event in SSE stream")
	} else {
		require.NoError(t, json.NewDecoder(rec.Body).Decode(out))
	}
}

func TestNewServer_ReturnsNonNil(t *testing.T) {
	s := mcpserver.NewServer("0.1.0", nil, nil, "")
	require.NotNil(t, s)
	assert.Nil(t, s.Store())
	assert.Nil(t, s.Gateway())
}

func TestServer_Project_ReturnsValue(t *testing.T) {
	s := mcpserver.NewServer("0.1.0", nil, nil, "my-project")
	assert.Equal(t, "my-project", s.Project())
}

func TestServer_Project_EmptyString(t *testing.T) {
	s := mcpserver.NewServer("0.1.0", nil, nil, "")
	assert.Equal(t, "", s.Project())
}

func TestServer_Handler_ReturnsHTTPHandler(t *testing.T) {
	s := mcpserver.NewServer("0.1.0", nil, nil, "")
	require.NotNil(t, s.Handler())
	require.NotNil(t, s.MCPServer())
}

func TestServer_Handler_RespondsToInitialize(t *testing.T) {
	s := mcpserver.NewServer("0.1.0", nil, nil, "")
	h := s.Handler()

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test",
				"version": "0.0.1",
			},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			ServerInfo struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
			Capabilities struct {
				Tools *json.RawMessage `json:"tools"`
			} `json:"capabilities"`
		} `json:"result"`
	}
	decodePayload(t, rec, &resp)

	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Equal(t, 1, resp.ID)
	assert.Equal(t, "kasmos", resp.Result.ServerInfo.Name)
	assert.Equal(t, "0.1.0", resp.Result.ServerInfo.Version)
	assert.NotNil(t, resp.Result.Capabilities.Tools)
}

func TestServer_Handler_ToolsListReturnsEmpty(t *testing.T) {
	s := mcpserver.NewServer("0.1.0", nil, nil, "")
	h := s.Handler()

	// First: initialize to obtain the Mcp-Session-Id the server assigns.
	initBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test",
				"version": "0.0.1",
			},
		},
	})
	require.NoError(t, err)

	initReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(initBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	h.ServeHTTP(initRec, initReq)
	require.Equal(t, http.StatusOK, initRec.Code)

	// The Streamable HTTP transport issues an Mcp-Session-Id on initialize.
	// Subsequent requests must carry it so the session ID is validated.
	sessionID := initRec.Header().Get("Mcp-Session-Id")

	// Then: tools/list (carries session ID if one was issued)
	listBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})
	require.NoError(t, err)

	listReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(listBody))
	listReq.Header.Set("Content-Type", "application/json")
	listReq.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		listReq.Header.Set("Mcp-Session-Id", sessionID)
	}
	listRec := httptest.NewRecorder()
	h.ServeHTTP(listRec, listReq)

	assert.Equal(t, http.StatusOK, listRec.Code)

	var resp struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	decodePayload(t, listRec, &resp)
	assert.Len(t, resp.Result.Tools, 0)
}
