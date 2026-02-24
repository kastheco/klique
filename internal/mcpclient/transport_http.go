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
