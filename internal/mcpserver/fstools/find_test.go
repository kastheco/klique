package fstools

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFdOutput_MultipleFiles(t *testing.T) {
	input := []byte("foo.go\nbar.go\nbaz.go\n")
	result := parseFdOutput(input)
	assert.Equal(t, []string{"foo.go", "bar.go", "baz.go"}, result)
}

func TestParseFdOutput_Empty(t *testing.T) {
	result := parseFdOutput([]byte(""))
	assert.Empty(t, result)
}

func TestParseFdOutput_MaxResults(t *testing.T) {
	// Build output with MaxFindResults+10 lines.
	lines := make([]string, MaxFindResults+10)
	for i := range lines {
		lines[i] = "file.go"
	}
	input := []byte(strings.Join(lines, "\n"))
	result := parseFdOutput(input)
	assert.Len(t, result, MaxFindResults)
}

func TestFindHandler_MissingPattern(t *testing.T) {
	dir := t.TempDir()
	sb := NewSandbox([]string{dir})
	runner := &mockRunner{}

	handler := makeFindHandler(sb, runner)
	req := mockCallToolRequest(map[string]any{"path": dir})

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestFindHandler_PathOutsideSandbox(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	sb := NewSandbox([]string{dir})
	runner := &mockRunner{}

	handler := makeFindHandler(sb, runner)
	req := mockCallToolRequest(map[string]any{"pattern": "*.go", "path": outside})

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestFindHandler_Success(t *testing.T) {
	dir := t.TempDir()
	sb := NewSandbox([]string{dir})

	var capturedName string
	var capturedArgs []string

	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedName = name
			capturedArgs = args
			return []byte("main.go\nutil.go\n"), nil
		},
	}

	handler := makeFindHandler(sb, runner)
	req := mockCallToolRequest(map[string]any{"pattern": "*.go", "path": dir})

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	assert.Equal(t, "fd", capturedName)

	hasGlob := false
	hasType := false
	for _, arg := range capturedArgs {
		if arg == "--glob" {
			hasGlob = true
		}
		if arg == "--type" {
			hasType = true
		}
	}
	assert.True(t, hasGlob, "fd args should include --glob")
	assert.True(t, hasType, "fd args should include --type")
}
