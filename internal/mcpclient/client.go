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
	resp, err := c.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
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
func (c *Client) CallTool(name string, args map[string]any) (*ToolResult, error) {
	resp, err := c.call("tools/call", map[string]any{
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

func (c *Client) call(method string, params any) (JSONRPCResponse, error) {
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
