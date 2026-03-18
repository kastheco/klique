package fstools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

type mockRunner struct {
	outputFn func(context.Context, string, ...string) ([]byte, error)
}

func (m *mockRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if m.outputFn != nil {
		return m.outputFn(ctx, name, args...)
	}
	return nil, nil
}

func mockCallToolRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}
