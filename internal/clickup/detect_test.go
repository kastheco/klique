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

func TestDetect_ProjectOpencodeJSON(t *testing.T) {
	dir := t.TempDir()
	ocDir := filepath.Join(dir, ".opencode")
	require.NoError(t, os.MkdirAll(ocDir, 0o755))
	ocJSON := `{"mcp":{"clickup":{"type":"remote","url":"https://mcp.clickup.com/mcp","oauth":{},"enabled":true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(ocDir, "opencode.json"), []byte(ocJSON), 0o644))

	cfg, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
	assert.Equal(t, "http", cfg.Type)
	assert.Equal(t, "https://mcp.clickup.com/mcp", cfg.URL)
}

func TestDetect_OpencodeGlobalConfig(t *testing.T) {
	// Use XDG_CONFIG_HOME to redirect os.UserConfigDir() to a temp dir.
	globalDir := t.TempDir()
	ocDir := filepath.Join(globalDir, "opencode")
	require.NoError(t, os.MkdirAll(ocDir, 0o755))
	ocJSON := `{"mcp":{"clickup":{"type":"remote","url":"https://mcp.clickup.com/mcp","oauth":{},"enabled":true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(ocDir, "opencode.json"), []byte(ocJSON), 0o644))

	t.Setenv("XDG_CONFIG_HOME", globalDir)

	repoDir := t.TempDir()
	cfg, found := clickup.DetectMCP(repoDir, "")
	assert.True(t, found)
	assert.Equal(t, "http", cfg.Type)
	assert.Equal(t, "https://mcp.clickup.com/mcp", cfg.URL)
}

func TestDetect_OpencodeLocalStdio(t *testing.T) {
	dir := t.TempDir()
	ocDir := filepath.Join(dir, ".opencode")
	require.NoError(t, os.MkdirAll(ocDir, 0o755))
	ocJSON := `{"mcp":{"clickup-tasks":{"type":"local","command":["npx","-y","@taazkareem/clickup-mcp-server@latest"],"enabled":true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(ocDir, "opencode.json"), []byte(ocJSON), 0o644))

	cfg, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
	assert.Equal(t, "stdio", cfg.Type)
	assert.Equal(t, "npx", cfg.Command)
	assert.Equal(t, []string{"-y", "@taazkareem/clickup-mcp-server@latest"}, cfg.Args)
}

func TestDetect_OpencodeDisabledServer(t *testing.T) {
	dir := t.TempDir()
	ocDir := filepath.Join(dir, ".opencode")
	require.NoError(t, os.MkdirAll(ocDir, 0o755))
	ocJSON := `{"mcp":{"clickup":{"type":"remote","url":"https://mcp.clickup.com/mcp","enabled":false}}}`
	require.NoError(t, os.WriteFile(filepath.Join(ocDir, "opencode.json"), []byte(ocJSON), 0o644))

	_, found := clickup.DetectMCP(dir, "")
	assert.False(t, found)
}

func TestDetect_OpencodeCommandString(t *testing.T) {
	dir := t.TempDir()
	ocDir := filepath.Join(dir, ".opencode")
	require.NoError(t, os.MkdirAll(ocDir, 0o755))
	ocJSON := `{"mcp":{"clickup":{"type":"local","command":"clickup-mcp","enabled":true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(ocDir, "opencode.json"), []byte(ocJSON), 0o644))

	cfg, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
	assert.Equal(t, "stdio", cfg.Type)
	assert.Equal(t, "clickup-mcp", cfg.Command)
	assert.Empty(t, cfg.Args)
}

func TestDetect_ProjectMCPJSON_PriorityOverOpencode(t *testing.T) {
	dir := t.TempDir()

	// Both .mcp.json and .opencode/opencode.json exist — .mcp.json wins.
	mcpJSON := `{"mcpServers":{"clickup":{"type":"http","url":"https://from-mcp-json.com"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpJSON), 0o644))

	ocDir := filepath.Join(dir, ".opencode")
	require.NoError(t, os.MkdirAll(ocDir, 0o755))
	ocJSON := `{"mcp":{"clickup":{"type":"remote","url":"https://from-opencode.com","enabled":true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(ocDir, "opencode.json"), []byte(ocJSON), 0o644))

	cfg, found := clickup.DetectMCP(dir, "")
	assert.True(t, found)
	assert.Equal(t, "https://from-mcp-json.com", cfg.URL)
}
