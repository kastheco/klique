package clickup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetect_ProjectMCPJSON(t *testing.T) {
	dir := t.TempDir()
	mcpJSON := `{"mcpServers":{"clickup":{"type":"http","url":"https://mcp.clickup.com/mcp"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpJSON), 0o644))

	cfg, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
	assert.Equal(t, "http", cfg.Type)
	assert.Equal(t, "https://mcp.clickup.com/mcp", cfg.URL)
}

func TestDetect_StdioServer(t *testing.T) {
	dir := t.TempDir()
	mcpJSON := `{"mcpServers":{"clickup-tasks":{"command":"npx","args":["-y","@taazkareem/clickup-mcp-server@latest"],"env":{"CLICKUP_API_KEY":"test"}}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpJSON), 0o644))

	cfg, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
	assert.Equal(t, "stdio", cfg.Type)
	assert.Equal(t, "npx", cfg.Command)
	assert.Contains(t, cfg.Args, "-y")
}

func TestDetect_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, found := clickup.DetectMCP(dir, "")
	assert.False(t, found)
}

func TestDetect_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	mcpJSON := `{"mcpServers":{"ClickUp-Production":{"type":"http","url":"https://mcp.clickup.com/mcp"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpJSON), 0o644))

	_, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
}

func TestDetect_FallbackToClaudeSettings(t *testing.T) {
	repoDir := t.TempDir()
	claudeDir := t.TempDir()
	settingsJSON := `{"mcpServers":{"clickup":{"type":"http","url":"https://mcp.clickup.com/mcp"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settingsJSON), 0o644))

	cfg, found := clickup.DetectMCP(repoDir, claudeDir)
	assert.True(t, found)
	assert.Equal(t, "https://mcp.clickup.com/mcp", cfg.URL)
}
