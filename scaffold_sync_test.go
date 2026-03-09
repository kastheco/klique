package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/internal/initcmd/harness"
)

func TestNewScaffoldCmd_HasSyncSubcommand(t *testing.T) {
	cmd := newScaffoldCmd()
	assert.Equal(t, "scaffold", cmd.Use)
	sub, _, err := cmd.Find([]string{"sync"})
	require.NoError(t, err)
	assert.Equal(t, "sync", sub.Use)
}

func TestProfilesToAgentConfigs(t *testing.T) {
	temp := 0.3
	profiles := map[string]config.AgentProfile{
		"coder":    {Program: "opencode", Model: "anthropic/claude-sonnet-4-6", Temperature: &temp, Effort: "medium", Enabled: true, Flags: []string{"--verbose"}},
		"reviewer": {Program: "claude", Model: "claude-opus-4-6", Effort: "high", Enabled: true},
		"planner":  {Program: "opencode", Model: "anthropic/claude-opus-4-6", Enabled: false},
	}
	configs := profilesToAgentConfigs(profiles)
	require.Len(t, configs, 2)
	byRole := map[string]harness.AgentConfig{}
	for _, cfg := range configs {
		byRole[cfg.Role] = cfg
	}
	assert.Equal(t, "opencode", byRole["coder"].Harness)
	assert.Equal(t, []string{"--verbose"}, byRole["coder"].ExtraFlags)
	assert.Equal(t, &temp, byRole["coder"].Temperature)
	assert.Equal(t, "high", byRole["reviewer"].Effort)
	_, ok := byRole["planner"]
	assert.False(t, ok)
}

func TestProfilesToAgentConfigs_Empty(t *testing.T) {
	assert.Empty(t, profilesToAgentConfigs(nil))
	assert.Empty(t, profilesToAgentConfigs(map[string]config.AgentProfile{}))
}

func TestScaffoldSync_RequiresTomlConfig(t *testing.T) {
	// Isolate HOME so GetConfigDir() migration cannot copy legacy config.toml
	// from ~/.config/kasmos (or similar) into the temp project dir.
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	var buf bytes.Buffer
	cmd := newScaffoldSyncCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config")
}
