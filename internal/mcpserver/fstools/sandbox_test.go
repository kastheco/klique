package fstools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/internal/mcpserver/fstools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resolvedDir(t *testing.T, dir string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return r
}

func TestSandbox_Validate_AllowedPath(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})
	result, err := sb.Validate(dir)
	require.NoError(t, err)
	assert.Equal(t, resolvedDir(t, dir), result)
}

func TestSandbox_Validate_Subdirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	sb := fstools.NewSandbox([]string{dir})
	result, err := sb.Validate(sub)
	require.NoError(t, err)
	assert.Equal(t, resolvedDir(t, sub), result)
}

func TestSandbox_Validate_OutsidePath(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	sb := fstools.NewSandbox([]string{dir1})
	_, err := sb.Validate(dir2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside allowed directories")
}

func TestSandbox_Validate_TraversalEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	// Build a traversal path that starts inside dir but escapes it via "../../".
	traversal := filepath.Join(dir, "sub", "..", "..", filepath.Base(outside))

	sb := fstools.NewSandbox([]string{dir})
	_, err := sb.Validate(traversal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside allowed directories")
}

func TestSandbox_Validate_RelativeResolution(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	// A path with ".." that resolves to sub (still inside dir).
	withDotDot := filepath.Join(sub, "..", "sub")

	sb := fstools.NewSandbox([]string{dir})
	result, err := sb.Validate(withDotDot)
	require.NoError(t, err)
	assert.Equal(t, resolvedDir(t, sub), result)
}

func TestSandbox_Validate_SymlinkEscape(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()

	// Create a symlink inside the allowed dir that points to the outside dir.
	link := filepath.Join(allowed, "escape")
	require.NoError(t, os.Symlink(outside, link))

	sb := fstools.NewSandbox([]string{allowed})
	_, err := sb.Validate(link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside allowed directories")
}

func TestSandbox_DefaultDir_WithDirs(t *testing.T) {
	dir := t.TempDir()
	sb := fstools.NewSandbox([]string{dir})
	assert.Equal(t, resolvedDir(t, dir), sb.DefaultDir())
}

func TestSandbox_DefaultDir_Empty(t *testing.T) {
	sb := fstools.NewSandbox(nil)
	assert.Equal(t, ".", sb.DefaultDir())
}

func TestRegisterTools_NilServerDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		fstools.RegisterTools(nil, []string{t.TempDir()})
	})
}
