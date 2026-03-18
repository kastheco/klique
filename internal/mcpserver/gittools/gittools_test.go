package gittools

import (
	"context"
	"testing"

	"github.com/kastheco/kasmos/internal/mcpserver/fstools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRunner struct {
	outputFn func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (m *mockRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if m.outputFn != nil {
		return m.outputFn(ctx, name, args...)
	}
	return nil, nil
}

func mockReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

func textResult(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	return tc.Text
}

// TestRegisterTools_NilServer verifies RegisterTools does not panic when
// passed a nil server.
func TestRegisterTools_NilServer(t *testing.T) {
	assert.NotPanics(t, func() { RegisterTools(nil, []string{t.TempDir()}) })
}

// TestGitStatus_DefaultsToSandboxDir verifies git status defaults to the
// sandbox default directory when no path argument is provided and that the
// exact git command prefix is used.
func TestGitStatus_DefaultsToSandboxDir(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedName string
	var capturedArgs []string

	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedName = name
			capturedArgs = args
			return []byte("## main\n"), nil
		},
	}

	handler := makeGitStatusHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	assert.Equal(t, "git", capturedName)
	// Verify prefix: git -C <sb.DefaultDir()> status --short --branch
	require.GreaterOrEqual(t, len(capturedArgs), 4)
	assert.Equal(t, "-C", capturedArgs[0])
	assert.Equal(t, sb.DefaultDir(), capturedArgs[1])
	assert.Equal(t, "status", capturedArgs[2])
	assert.Equal(t, "--short", capturedArgs[3])
	assert.Equal(t, "--branch", capturedArgs[4])
}

// TestGitStatus_OutsideSandbox verifies that a path outside the sandbox
// returns a tool error with appropriate message.
func TestGitStatus_OutsideSandbox(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	sb := fstools.NewSandbox([]string{allowed})

	runner := &mockRunner{}
	handler := makeGitStatusHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"path": outside}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	require.NotEmpty(t, result.Content)
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "outside allowed directories")
}

// TestGitDiff_NoStaged verifies that --cached is absent when staged is false.
func TestGitDiff_NoStaged(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("diff output"), nil
		},
	}

	handler := makeGitDiffHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// --cached should not be present
	for _, arg := range capturedArgs {
		assert.NotEqual(t, "--cached", arg, "--cached should not be in args when staged=false")
	}
}

// TestGitDiff_Staged verifies that --cached is present when staged is true.
func TestGitDiff_Staged(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("staged diff"), nil
		},
	}

	handler := makeGitDiffHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"staged": true}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	assert.Contains(t, capturedArgs, "--cached")
}

// TestGitDiff_WithFile verifies that when file is set, the args end with
// -- <file> so filenames cannot be parsed as flags.
func TestGitDiff_WithFile(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("file diff"), nil
		},
	}

	handler := makeGitDiffHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"file": "foo.go"}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// Last two args should be -- foo.go
	require.GreaterOrEqual(t, len(capturedArgs), 2)
	assert.Equal(t, "--", capturedArgs[len(capturedArgs)-2])
	assert.Equal(t, "foo.go", capturedArgs[len(capturedArgs)-1])
}

// TestGitDiff_NoChanges verifies that empty diff output returns "no changes".
func TestGitDiff_NoChanges(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			return []byte("   \n  "), nil
		},
	}

	handler := makeGitDiffHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := textResult(t, result)
	assert.Equal(t, "no changes", text)
}

// TestGitDiff_OutsideSandbox verifies that a path outside the sandbox returns
// a tool error with appropriate message.
func TestGitDiff_OutsideSandbox(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	sb := fstools.NewSandbox([]string{allowed})

	runner := &mockRunner{}
	handler := makeGitDiffHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"path": outside}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	require.NotEmpty(t, result.Content)
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "outside allowed directories")
}

// TestGitLog_DefaultCount verifies that the default count of 20 is used when
// no count is provided.
func TestGitLog_DefaultCount(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("abc1234 first commit"), nil
		},
	}

	handler := makeGitLogHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	assert.Contains(t, capturedArgs, "-20")
}

// TestGitLog_ClampCountAbove100 verifies that count > 100 is clamped to 100.
func TestGitLog_ClampCountAbove100(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("commits"), nil
		},
	}

	handler := makeGitLogHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"count": float64(999)}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	assert.Contains(t, capturedArgs, "-100")
}

// TestGitLog_ClampCountBelow1 verifies that count < 1 is clamped to 1.
func TestGitLog_ClampCountBelow1(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("commit"), nil
		},
	}

	handler := makeGitLogHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"count": float64(-5)}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	assert.Contains(t, capturedArgs, "-1")
}

// TestGitLog_OnelineFalse verifies that oneline: false omits --oneline.
func TestGitLog_OnelineFalse(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("commit"), nil
		},
	}

	handler := makeGitLogHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"oneline": false}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	for _, arg := range capturedArgs {
		assert.NotEqual(t, "--oneline", arg, "--oneline should not be present when oneline=false")
	}
}

// TestGitLog_OnelineDefault verifies that oneline defaults to true.
func TestGitLog_OnelineDefault(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("commit"), nil
		},
	}

	handler := makeGitLogHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	assert.Contains(t, capturedArgs, "--oneline")
}

// TestGitLog_WithFile verifies that when file is set, args end with -- <file>.
func TestGitLog_WithFile(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("commit"), nil
		},
	}

	handler := makeGitLogHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"file": "main.go"}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	require.GreaterOrEqual(t, len(capturedArgs), 2)
	assert.Equal(t, "--", capturedArgs[len(capturedArgs)-2])
	assert.Equal(t, "main.go", capturedArgs[len(capturedArgs)-1])
}

// TestGitLog_OutsideSandbox verifies that a path outside the sandbox returns
// a tool error with appropriate message.
func TestGitLog_OutsideSandbox(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	sb := fstools.NewSandbox([]string{allowed})

	runner := &mockRunner{}
	handler := makeGitLogHandler(sb, runner)
	result, err := handler(context.Background(), mockReq(map[string]any{"path": outside}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	require.NotEmpty(t, result.Content)
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "outside allowed directories")
}
