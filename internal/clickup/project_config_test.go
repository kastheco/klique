package clickup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProjectConfig_NoFile(t *testing.T) {
	cfg := clickup.LoadProjectConfig(t.TempDir())
	assert.Equal(t, "", cfg.WorkspaceID)
}

func TestSaveAndLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &clickup.ProjectConfig{WorkspaceID: "9017630208"}
	require.NoError(t, clickup.SaveProjectConfig(dir, cfg))

	loaded := clickup.LoadProjectConfig(dir)
	assert.Equal(t, "9017630208", loaded.WorkspaceID)
}

func TestSaveProjectConfig_CreatesKasmosDir(t *testing.T) {
	dir := t.TempDir()
	cfg := &clickup.ProjectConfig{WorkspaceID: "123"}
	require.NoError(t, clickup.SaveProjectConfig(dir, cfg))

	_, err := os.Stat(filepath.Join(dir, ".kasmos", "clickup.json"))
	require.NoError(t, err, ".kasmos/clickup.json should exist")
}

func TestLoadProjectConfig_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	kasmosDir := filepath.Join(dir, ".kasmos")
	require.NoError(t, os.MkdirAll(kasmosDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(kasmosDir, "clickup.json"), []byte("{corrupt"), 0o644))

	cfg := clickup.LoadProjectConfig(dir)
	assert.Equal(t, "", cfg.WorkspaceID, "corrupt file should return empty config")
}
