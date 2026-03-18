package fstools

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeRgMatchLine creates a single rg --json match line for use in tests.
// file and lineText are JSON-quoted via %q so special characters are safe.
func makeRgMatchLine(file, lineText string, lineNum, start, end int) string {
	return fmt.Sprintf(
		`{"type":"match","data":{"path":{"text":%q},"lines":{"text":%q},"line_number":%d,"absolute_offset":0,"submatches":[{"match":{"text":"x"},"start":%d,"end":%d}]}}`,
		file, lineText, lineNum, start, end,
	)
}

func TestParseRgJSON_SingleMatch(t *testing.T) {
	line := makeRgMatchLine("foo.go", "Hello world\n", 10, 0, 5)
	matches, err := parseRgJSON([]byte(line))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "foo.go", matches[0].File)
	assert.Equal(t, 10, matches[0].Line)
	assert.Equal(t, 1, matches[0].Column) // start=0 → column=0+1=1
	assert.Equal(t, "Hello world", matches[0].Text)
}

func TestParseRgJSON_MultipleMatches(t *testing.T) {
	var lines []string
	lines = append(lines, makeRgMatchLine("a.go", "line one\n", 1, 0, 4))
	lines = append(lines, makeRgMatchLine("b.go", "line two\n", 2, 5, 8))
	lines = append(lines, `{"type":"summary","data":{}}`)
	data := []byte(strings.Join(lines, "\n"))

	matches, err := parseRgJSON(data)
	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.Equal(t, "a.go", matches[0].File)
	assert.Equal(t, 1, matches[0].Column) // start=0 → 1
	assert.Equal(t, "b.go", matches[1].File)
	assert.Equal(t, 6, matches[1].Column) // start=5 → 6
}

func TestParseRgJSON_MaxResults(t *testing.T) {
	var lines []string
	for i := 0; i < MaxGrepMatches+10; i++ {
		lines = append(lines, makeRgMatchLine("f.go", fmt.Sprintf("line %d\n", i), i+1, 0, 1))
	}
	data := []byte(strings.Join(lines, "\n"))

	matches, err := parseRgJSON(data)
	require.NoError(t, err)
	assert.Len(t, matches, MaxGrepMatches)
}

func TestParseRgJSON_InvalidJSON(t *testing.T) {
	data := []byte(`not valid json`)
	_, err := parseRgJSON(data)
	require.Error(t, err)
}

func TestGrepHandler_MissingPattern(t *testing.T) {
	dir := t.TempDir()
	sb := NewSandbox([]string{dir})
	runner := &mockRunner{}
	handler := makeGrepHandler(sb, runner)

	req := mockCallToolRequest(map[string]any{}) // no pattern
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestGrepHandler_PathOutsideSandbox(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	sb := NewSandbox([]string{dir})
	runner := &mockRunner{}
	handler := makeGrepHandler(sb, runner)

	req := mockCallToolRequest(map[string]any{
		"pattern": "foo",
		"path":    outside,
	})
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	require.NotEmpty(t, result.Content)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "outside allowed directories")
}

func TestGrepHandler_NoMatchesReturnsEmptyJSON(t *testing.T) {
	dir := t.TempDir()
	sb := NewSandbox([]string{dir})

	// Produce a real *exec.ExitError with ExitCode() == 1 (rg "no matches").
	exitErr := func() error {
		cmd := exec.Command("sh", "-c", "exit 1")
		return cmd.Run()
	}()
	require.IsType(t, (*exec.ExitError)(nil), exitErr, "expected *exec.ExitError from sh exit 1")

	runner := &mockRunner{
		outputFn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, exitErr
		},
	}
	handler := makeGrepHandler(sb, runner)

	req := mockCallToolRequest(map[string]any{
		"pattern": "notfound",
		"path":    dir,
	})
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "[]", result.Content[0].(mcp.TextContent).Text)
}

func TestGrepHandler_Success(t *testing.T) {
	dir := t.TempDir()
	sb := NewSandbox([]string{dir})

	rgOutput := makeRgMatchLine(filepath.Join(dir, "main.go"), "func Hello() {}\n", 1, 5, 10)

	var capturedName string
	var capturedArgs []string

	runner := &mockRunner{
		outputFn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			capturedName = name
			capturedArgs = append(capturedArgs, args...)
			return []byte(rgOutput), nil
		},
	}
	handler := makeGrepHandler(sb, runner)

	req := mockCallToolRequest(map[string]any{
		"pattern":       "Hello",
		"path":          dir,
		"glob":          "*.go",
		"context_lines": 2,
	})
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// Verify the runner was invoked with rg and --json.
	assert.Equal(t, "rg", capturedName)
	assert.Contains(t, capturedArgs, "--json")
}
