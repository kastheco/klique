package clickup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// mcpConfigFile represents the structure of .mcp.json or Claude settings.json.
type mcpConfigFile struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

// serverEntry is a union of http and stdio server config fields.
type serverEntry struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// DetectMCP scans config files for a ClickUp MCP server.
// repoDir is the project root (checks .mcp.json).
// claudeDir is the Claude config dir (checks settings.json, settings.local.json).
// Pass empty claudeDir to skip Claude config scanning.
func DetectMCP(repoDir, claudeDir string) (MCPServerConfig, bool) {
	if cfg, ok := scanFile(filepath.Join(repoDir, ".mcp.json")); ok {
		return cfg, true
	}

	if claudeDir != "" {
		if cfg, ok := scanFile(filepath.Join(claudeDir, "settings.json")); ok {
			return cfg, true
		}

		if cfg, ok := scanFile(filepath.Join(claudeDir, "settings.local.json")); ok {
			return cfg, true
		}
	}

	return MCPServerConfig{}, false
}

func scanFile(path string) (MCPServerConfig, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MCPServerConfig{}, false
	}

	var file mcpConfigFile
	if err := json.Unmarshal(data, &file); err != nil {
		return MCPServerConfig{}, false
	}

	for name, raw := range file.MCPServers {
		if !strings.Contains(strings.ToLower(name), "clickup") {
			continue
		}

		var entry serverEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}

		cfg := MCPServerConfig{Env: entry.Env}
		if entry.Type == "http" || entry.URL != "" {
			cfg.Type = "http"
			cfg.URL = entry.URL
		} else if entry.Command != "" {
			cfg.Type = "stdio"
			cfg.Command = entry.Command
			cfg.Args = entry.Args
		} else {
			continue
		}

		return cfg, true
	}

	return MCPServerConfig{}, false
}
