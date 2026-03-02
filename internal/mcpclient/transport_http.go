package mcpclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPTransport speaks JSON-RPC over HTTP POST (Streamable HTTP MCP transport).
type HTTPTransport struct {
	url   string
	token string
	http  *http.Client
}

// NewHTTPTransport creates an HTTP transport with a bearer token.
// Pass an empty token to skip authorization headers.
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
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if t.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+t.token)
	}

	httpResp, err := t.http.Do(httpReq)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("http post: %w", err)
	}
	defer httpResp.Body.Close()

	// 202 Accepted is returned for JSON-RPC notifications (no response expected).
	if httpResp.StatusCode == http.StatusAccepted {
		return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}, nil
	}

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return JSONRPCResponse{}, fmt.Errorf("http %d: %s", httpResp.StatusCode, string(respBody))
	}

	ct := httpResp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return parseSSEResponse(httpResp.Body)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return resp, nil
}

// parseSSEResponse reads a Server-Sent Events stream and extracts the first
// JSON-RPC response from a "data:" line. MCP Streamable HTTP servers may
// respond with Content-Type: text/event-stream containing:
//
//	event: message
//	data: {"jsonrpc":"2.0","id":1,"result":{...}}
func parseSSEResponse(r io.Reader) (JSONRPCResponse, error) {
	scanner := bufio.NewScanner(r)
	// Increase buffer for large tool list responses (default 64KB is too small).
	scanner.Buffer(make([]byte, 0, 256*1024), 512*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			payload := strings.TrimPrefix(line, "data: ")
			var resp JSONRPCResponse
			if err := json.Unmarshal([]byte(payload), &resp); err != nil {
				return JSONRPCResponse{}, fmt.Errorf("parse SSE data: %w", err)
			}
			return resp, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("read SSE stream: %w", err)
	}
	return JSONRPCResponse{}, fmt.Errorf("no data event in SSE stream")
}

// Close is a no-op for HTTP transport.
func (t *HTTPTransport) Close() error { return nil }
