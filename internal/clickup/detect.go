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

// opencodeConfigFile represents the structure of opencode.json.
type opencodeConfigFile struct {
	MCP map[string]json.RawMessage `json:"mcp"`
}

// serverEntry is a union of http and stdio server config fields.
type serverEntry struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// opencodeServerEntry is the opencode MCP server config format.
type opencodeServerEntry struct {
	Type    string            `json:"type"`    // "remote" or "local"
	URL     string            `json:"url"`     // for remote type
	Command json.RawMessage   `json:"command"` // string or []string for local type
	Env     map[string]string `json:"env"`
	Enabled *bool             `json:"enabled"`
}

// DetectMCP scans config files for a ClickUp MCP server.
// repoDir is the project root (checks .mcp.json, .opencode/opencode.json).
// claudeDir is the Claude config dir (checks settings.json, settings.local.json).
// Pass empty claudeDir to skip Claude config scanning.
func DetectMCP(repoDir, claudeDir string) (MCPServerConfig, bool) {
	// Project-level: .mcp.json (Claude Code / generic)
	if cfg, ok := scanFile(filepath.Join(repoDir, ".mcp.json")); ok {
		return cfg, true
	}

	// Project-level: .opencode/opencode.json
	if cfg, ok := scanOpencode(filepath.Join(repoDir, ".opencode", "opencode.json")); ok {
		return cfg, true
	}

	// User-level: Claude Desktop settings
	if claudeDir != "" {
		if cfg, ok := scanFile(filepath.Join(claudeDir, "settings.json")); ok {
			return cfg, true
		}

		if cfg, ok := scanFile(filepath.Join(claudeDir, "settings.local.json")); ok {
			return cfg, true
		}
	}

	// User-level: opencode global config
	if configDir, err := os.UserConfigDir(); err == nil {
		if cfg, ok := scanOpencode(filepath.Join(configDir, "opencode", "opencode.json")); ok {
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

	return matchServers(file.MCPServers)
}

func scanOpencode(path string) (MCPServerConfig, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MCPServerConfig{}, false
	}

	var file opencodeConfigFile
	if err := json.Unmarshal(data, &file); err != nil {
		return MCPServerConfig{}, false
	}

	for name, raw := range file.MCP {
		if !strings.Contains(strings.ToLower(name), "clickup") {
			continue
		}

		var entry opencodeServerEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}

		// Skip explicitly disabled servers.
		if entry.Enabled != nil && !*entry.Enabled {
			continue
		}

		cfg := MCPServerConfig{Env: entry.Env}
		switch entry.Type {
		case "remote":
			cfg.Type = "http"
			cfg.URL = entry.URL
		case "local":
			cmd, args := parseOpencodeCommand(entry.Command)
			if cmd == "" {
				continue
			}
			cfg.Type = "stdio"
			cfg.Command = cmd
			cfg.Args = args
		default:
			// Fall back to URL presence check.
			if entry.URL != "" {
				cfg.Type = "http"
				cfg.URL = entry.URL
			} else {
				continue
			}
		}

		return cfg, true
	}

	return MCPServerConfig{}, false
}

// parseOpencodeCommand handles the opencode command field which can be
// a string ("npx") or an array (["npx", "-y", "pkg"]).
func parseOpencodeCommand(raw json.RawMessage) (string, []string) {
	if len(raw) == 0 {
		return "", nil
	}

	// Try array first (most common in opencode configs).
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		return arr[0], arr[1:]
	}

	// Fall back to plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return s, nil
	}

	return "", nil
}

func matchServers(servers map[string]json.RawMessage) (MCPServerConfig, bool) {
	for name, raw := range servers {
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
