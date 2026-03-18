package mcpserver_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/internal/mcpserver"
	"github.com/kastheco/kasmos/internal/mcpserver/tasktools"
)

// startMCPTestServer binds on a random loopback port, starts an http.Server
// backed by the mcpserver.Handler in a goroutine, and returns the MCP endpoint
// URL (http://127.0.0.1:<port>/mcp). The server is shut down automatically
// when the test ends via t.Cleanup.
func startMCPTestServer(t *testing.T, version string) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to bind loopback listener")

	srv := &http.Server{
		Handler: mcpserver.NewServer(version, nil, nil, "").Handler(),
	}

	go func() {
		// ErrServerClosed is the expected error after Shutdown; ignore it.
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			// Surface unexpected errors in the test output without calling t.Fatal
			// from a goroutine (which would panic).
			t.Logf("mcpserver goroutine: unexpected error: %v", serveErr)
		}
	}()

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	})

	return fmt.Sprintf("http://%s/mcp", ln.Addr().String())
}

// TestIntegration_ClientConnectsAndListsTools exercises the full MCP protocol
// flow over a real TCP loopback connection: initialize then list tools (empty).
func TestIntegration_ClientConnectsAndListsTools(t *testing.T) {
	url := startMCPTestServer(t, "0.1.0")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	httpTransport, err := transport.NewStreamableHTTP(url)
	require.NoError(t, err, "failed to create StreamableHTTP transport")

	c := client.NewClient(httpTransport)
	t.Cleanup(func() { _ = c.Close() })

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "integration-test",
				Version: "0.1.0",
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	}

	initResult, err := c.Initialize(ctx, initReq)
	require.NoError(t, err, "Initialize must succeed")
	require.NotNil(t, initResult, "Initialize result must not be nil")

	assert.Equal(t, "kasmos", initResult.ServerInfo.Name, "server name should be 'kasmos'")
	assert.Equal(t, "0.1.0", initResult.ServerInfo.Version, "server version should match")
	assert.NotNil(t, initResult.Capabilities.Tools, "server should advertise tools capability")

	toolsResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err, "ListTools must succeed")
	assert.Empty(t, toolsResult.Tools, "no tools should be registered yet")
}

// TestIntegration_ServerReportsVersion verifies that the version passed to
// NewServer is faithfully echoed back in the initialize response.
func TestIntegration_ServerReportsVersion(t *testing.T) {
	url := startMCPTestServer(t, "1.2.3")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	httpTransport, err := transport.NewStreamableHTTP(url)
	require.NoError(t, err, "failed to create StreamableHTTP transport")

	c := client.NewClient(httpTransport)
	t.Cleanup(func() { _ = c.Close() })

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "integration-test",
				Version: "0.1.0",
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	}

	initResult, err := c.Initialize(ctx, initReq)
	require.NoError(t, err, "Initialize must succeed")
	require.NotNil(t, initResult, "Initialize result must not be nil")

	assert.Equal(t, "1.2.3", initResult.ServerInfo.Version, "server version should reflect NewServer argument")
}

// startMCPTestServerWithTools binds on a random loopback port, starts an
// http.Server backed by an mcpserver.Server with tasktools registered, and
// returns the MCP endpoint URL. The server is shut down automatically when the
// test ends via t.Cleanup.
func startMCPTestServerWithTools(t *testing.T, version string) string {
	t.Helper()

	store := taskstore.NewTestStore(t)
	gw, err := taskstore.NewSQLiteSignalGateway(":memory:")
	require.NoError(t, err, "failed to create signal gateway")
	t.Cleanup(func() { _ = gw.Close() })

	kasmosSrv := mcpserver.NewServer(version, store, gw, "testproj")
	tasktools.Register(kasmosSrv)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to bind loopback listener")

	httpSrv := &http.Server{
		Handler: kasmosSrv.Handler(),
	}

	go func() {
		if serveErr := httpSrv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			t.Logf("mcpserver goroutine: unexpected error: %v", serveErr)
		}
	}()

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	})

	return fmt.Sprintf("http://%s/mcp", ln.Addr().String())
}

// TestIntegration_AllTaskToolsRegistered verifies that all six task tools are
// advertised by the server after tasktools.Register is called. It exercises the
// full MCP protocol flow over a real TCP loopback connection.
func TestIntegration_AllTaskToolsRegistered(t *testing.T) {
	url := startMCPTestServerWithTools(t, "0.1.0")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	httpTransport, err := transport.NewStreamableHTTP(url)
	require.NoError(t, err, "failed to create StreamableHTTP transport")

	c := client.NewClient(httpTransport)
	t.Cleanup(func() { _ = c.Close() })

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "integration-test",
				Version: "0.1.0",
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	}

	initResult, err := c.Initialize(ctx, initReq)
	require.NoError(t, err, "Initialize must succeed")
	require.NotNil(t, initResult, "Initialize result must not be nil")

	toolsResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err, "ListTools must succeed")

	toolNames := make([]string, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	expectedTools := []string{
		"task_show",
		"task_list",
		"task_update_content",
		"task_transition",
		"task_create",
		"signal_create",
	}
	for _, name := range expectedTools {
		assert.Contains(t, toolNames, name, "tool %q should be registered", name)
	}
}
