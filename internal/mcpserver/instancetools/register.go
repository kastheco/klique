package instancetools

import "github.com/mark3labs/mcp-go/server"

// registrarFn is a function that registers a single tool group with the MCP
// server. Each tool file calls addRegistrar in its init function so that
// RegisterTools can wire everything without importing each tool file individually.
type registrarFn func(srv *server.MCPServer, loadState StateLoader, runner CmdRunner, socketPath string)

// registrars holds all tool registrar functions added via addRegistrar.
var registrars []registrarFn

// addRegistrar appends fn to the list of registrar functions that
// RegisterTools calls. Tool files should call this from their init function.
func addRegistrar(fn registrarFn) {
	registrars = append(registrars, fn)
}

// RegisterTools wires all registered instance tools into srv. When runner is
// nil it defaults to &ExecRunner{}. It is safe to call with a nil srv; in that
// case it returns without panicking or registering anything.
func RegisterTools(srv *server.MCPServer, loadState StateLoader, runner CmdRunner, socketPath string) {
	if srv == nil {
		return
	}
	if runner == nil {
		runner = &ExecRunner{}
	}
	for _, fn := range registrars {
		fn(srv, loadState, runner, socketPath)
	}
}
