package clickup

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const projectConfigPath = ".kasmos/clickup.json"

// ProjectConfig holds per-project ClickUp settings persisted to .kasmos/clickup.json.
type ProjectConfig struct {
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// LoadProjectConfig reads the ClickUp project config from <repoRoot>/.kasmos/clickup.json.
// Returns an empty config if the file doesn't exist or is corrupt.
func LoadProjectConfig(repoRoot string) *ProjectConfig {
	data, err := os.ReadFile(filepath.Join(repoRoot, projectConfigPath))
	if err != nil {
		return &ProjectConfig{}
	}
	var cfg ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &ProjectConfig{}
	}
	return &cfg
}

// SaveProjectConfig writes the ClickUp project config to <repoRoot>/.kasmos/clickup.json.
func SaveProjectConfig(repoRoot string, cfg *ProjectConfig) error {
	dir := filepath.Join(repoRoot, ".kasmos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "clickup.json"), data, 0o644)
}
