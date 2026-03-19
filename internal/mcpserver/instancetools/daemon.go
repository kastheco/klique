package instancetools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/kastheco/kasmos/daemon/api"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func init() {
	addRegistrar(registerDaemonStatus)
}

// makeDaemonStatusHandler returns a ToolHandlerFunc that queries the kasmos
// daemon over its Unix domain socket and returns the StatusResponse as JSON.
// socketPath is captured at registration time and used for every call.
func makeDaemonStatusHandler(socketPath string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
				},
			},
			Timeout: 3 * time.Second,
		}

		resp, err := client.Get("http://kas/v1/status")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("daemon not running: %v", err)), nil
		}
		defer resp.Body.Close()

		var status api.StatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decode daemon status: %v", err)), nil
		}

		result, err := mcp.NewToolResultJSON(status)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encode daemon status: %v", err)), nil
		}
		return result, nil
	}
}

// registerDaemonStatus registers the daemon_status tool with the MCP server.
// socketPath is provided by the registrar argument; when empty, daemonSocketPath()
// is used as a fallback.
func registerDaemonStatus(srv *server.MCPServer, _ StateLoader, _ CmdRunner, socketPath string) {
	if socketPath == "" {
		socketPath = daemonSocketPath()
	}
	tool := mcp.NewTool(
		"daemon_status",
		mcp.WithDescription("query the kasmos daemon status; returns JSON with running, repo_count, and repos fields"),
	)
	srv.AddTool(tool, makeDaemonStatusHandler(socketPath))
}
