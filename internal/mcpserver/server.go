// Package mcpserver wraps github.com/mark3labs/mcp-go to expose a Streamable
// HTTP MCP server that shares the kasmos task store and signal gateway. Future
// task/codebase/instance tool batches attach to the Server to register their
// tools at startup.
package mcpserver

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/kastheco/kasmos/config/taskstore"
)

// Server holds the MCP server instance, its HTTP handler, and references to the
// shared task store and signal gateway. Store and Gateway may be nil when no
// tools have been registered yet; tool handlers should check them at call time.
type Server struct {
	mcp     *server.MCPServer
	handler http.Handler
	store   taskstore.Store
	gateway taskstore.SignalGateway
}

// NewServer constructs a Server with Streamable HTTP transport mounted at /mcp.
// version is advertised in the initialize response (e.g. "0.1.0"). store and
// gateway may be nil; they are stored as-is for future tool handlers to use.
func NewServer(version string, store taskstore.Store, gateway taskstore.SignalGateway) *Server {
	mcpSrv := server.NewMCPServer(
		"kasmos",
		version,
		server.WithToolCapabilities(true),
		server.WithRecovery(),
		server.WithInstructions("kasmos MCP server — task orchestration and codebase tools"),
	)

	httpTransport := server.NewStreamableHTTPServer(mcpSrv,
		server.WithEndpointPath("/mcp"),
	)

	return &Server{
		mcp:     mcpSrv,
		handler: httpTransport,
		store:   store,
		gateway: gateway,
	}
}

// Handler returns the http.Handler that serves the MCP Streamable HTTP
// transport. Mount it on any HTTP mux to serve MCP requests.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// MCPServer returns the underlying *server.MCPServer so that tool-registration
// code in other packages can call AddTool without depending on this wrapper.
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcp
}

// Store returns the shared taskstore.Store, which may be nil if no store was
// provided at construction time.
func (s *Server) Store() taskstore.Store {
	return s.store
}

// Gateway returns the shared taskstore.SignalGateway, which may be nil if no
// gateway was provided at construction time.
func (s *Server) Gateway() taskstore.SignalGateway {
	return s.gateway
}
