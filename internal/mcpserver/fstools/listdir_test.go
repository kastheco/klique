package fstools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseFdListOutput checks that parseFdListOutput correctly parses fd output
// into DirEntry values, populating Name, IsDir, and Size fields.
func TestParseFdListOutput(t *testing.T) {
	dir := t.TempDir()

	// Create files and directories.
	subDir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	fileA := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("hello"), 0o644))
	fileB := filepath.Join(subDir, "b.txt")
	require.NoError(t, os.WriteFile(fileB, []byte("world!!!"), 0o644))

	// Simulate fd output: one entry per line, absolute paths.
	fdOutput := fmt.Sprintf("%s\n%s\n%s\n", subDir, fileA, fileB)

	entries := parseFdListOutput([]byte(fdOutput), dir)
	require.Len(t, entries, 3)

	byName := make(map[string]DirEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}

	subdirEntry, ok := byName["subdir"]
	require.True(t, ok, "expected subdir entry")
	assert.True(t, subdirEntry.IsDir)
	assert.Equal(t, int64(0), subdirEntry.Size, "directories should have size 0")

	aEntry, ok := byName["a.txt"]
	require.True(t, ok, "expected a.txt entry")
	assert.False(t, aEntry.IsDir)
	assert.Equal(t, int64(5), aEntry.Size)

	bEntry, ok := byName[filepath.Join("subdir", "b.txt")]
	require.True(t, ok, "expected subdir/b.txt entry")
	assert.False(t, bEntry.IsDir)
	assert.Equal(t, int64(8), bEntry.Size)
}

// TestParseFdListOutput_MaxEntries verifies that parseFdListOutput caps results
// at MaxListEntries by creating real files in a temp dir.
func TestParseFdListOutput_MaxEntries(t *testing.T) {
	dir := t.TempDir()

	// Create MaxListEntries+10 files.
	total := MaxListEntries + 10
	lines := make([]string, 0, total)
	for i := 0; i < total; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file%04d.txt", i))
		require.NoError(t, os.WriteFile(name, []byte("x"), 0o644))
		lines = append(lines, name)
	}
	fdOutput := strings.Join(lines, "\n") + "\n"

	entries := parseFdListOutput([]byte(fdOutput), dir)
	assert.Len(t, entries, MaxListEntries)
}

// TestListDirHandler_MissingPath checks that list_dir returns an error when no
// path argument is supplied.
func TestListDirHandler_MissingPath(t *testing.T) {
	sb := NewSandbox([]string{t.TempDir()})
	runner := &mockRunner{}
	handler := makeListDirHandler(sb, runner)

	req := mockCallToolRequest(map[string]any{})
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "expected IsError=true for missing path")
}

// TestListDirHandler_PathOutsideSandbox verifies that paths outside the sandbox
// are rejected.
func TestListDirHandler_PathOutsideSandbox(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()

	sb := NewSandbox([]string{allowed})
	runner := &mockRunner{}
	handler := makeListDirHandler(sb, runner)

	req := mockCallToolRequest(map[string]any{"path": outside})
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "expected IsError=true for path outside sandbox")
}

// TestListDirHandler_NotDirectory verifies that non-directory paths are rejected.
func TestListDirHandler_NotDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0o644))

	sb := NewSandbox([]string{dir})
	runner := &mockRunner{}
	handler := makeListDirHandler(sb, runner)

	req := mockCallToolRequest(map[string]any{"path": file})
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "expected IsError=true for non-directory path")
}

// TestListDirHandler_Success verifies that a valid directory path results in fd
// being called with the expected arguments including --max-depth, and that the
// result is a JSON-encoded list of entries.
func TestListDirHandler_Success(t *testing.T) {
	dir := t.TempDir()

	// Create a file so parseFdListOutput has something to process.
	file := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(file, []byte("hi"), 0o644))

	var capturedArgs []string
	runner := &mockRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = append([]string{name}, args...)
			// Return the file path as fd output.
			return []byte(file + "\n"), nil
		},
	}

	sb := NewSandbox([]string{dir})
	handler := makeListDirHandler(sb, runner)

	req := mockCallToolRequest(map[string]any{"path": dir})
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "expected success result")

	// Verify fd was called with --max-depth.
	require.NotEmpty(t, capturedArgs)
	assert.Contains(t, capturedArgs, "--max-depth", "expected --max-depth in fd args")
	// The validated directory path should be in the args.
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolvedDir = dir
	}
	assert.Contains(t, capturedArgs, resolvedDir, "expected validated dir path in fd args")

	// Verify the result content is valid JSON array of DirEntry.
	require.NotEmpty(t, result.Content)
	contentText := ""
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			contentText = tc.Text
			break
		}
	}
	require.NotEmpty(t, contentText, "expected text content in result")

	var entries []DirEntry
	require.NoError(t, json.Unmarshal([]byte(contentText), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "hello.txt", entries[0].Name)
	assert.False(t, entries[0].IsDir)
	assert.Equal(t, int64(2), entries[0].Size)
}
