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

// TestProfilesToAgentConfigs_ChatFanOut verifies that the "chat" role is emitted
// once per harness program present among enabled non-chat agents, mirroring the
// behaviour of wizard.State.ToAgentConfigs.
func TestProfilesToAgentConfigs_ChatFanOut(t *testing.T) {
	profiles := map[string]config.AgentProfile{
		"coder":    {Program: "opencode", Model: "m1", Enabled: true},
		"reviewer": {Program: "claude", Model: "m2", Enabled: true},
		"chat":     {Program: "opencode", Model: "chat-model", Enabled: true},
	}
	configs := profilesToAgentConfigs(profiles)

	// Collect chat entries.
	var chatEntries []harness.AgentConfig
	for _, c := range configs {
		if c.Role == "chat" {
			chatEntries = append(chatEntries, c)
		}
	}
	// chat should be fanned out to both "claude" and "opencode".
	require.Len(t, chatEntries, 2, "chat should be emitted for every harness")
	harnessNames := []string{chatEntries[0].Harness, chatEntries[1].Harness}
	assert.ElementsMatch(t, []string{"claude", "opencode"}, harnessNames)
	for _, c := range chatEntries {
		assert.Equal(t, "chat-model", c.Model)
		assert.True(t, c.Enabled)
	}
}

// TestProfilesToAgentConfigs_ChatFallback verifies that when no other enabled
// agents exist, chat falls back to its own Program instead of being dropped.
func TestProfilesToAgentConfigs_ChatFallback(t *testing.T) {
	profiles := map[string]config.AgentProfile{
		"chat": {Program: "opencode", Model: "chat-model", Enabled: true},
	}
	configs := profilesToAgentConfigs(profiles)
	require.Len(t, configs, 1)
	assert.Equal(t, "chat", configs[0].Role)
	assert.Equal(t, "opencode", configs[0].Harness)
}

// TestProfilesToAgentConfigs_ChatFanOut_IncludesDisabledHarnesses verifies that the
// chat fan-out includes harnesses from disabled non-chat profiles. wizard.State.ToAgentConfigs
// fans chat to every *selected* harness regardless of role enablement; we must match that.
func TestProfilesToAgentConfigs_ChatFanOut_IncludesDisabledHarnesses(t *testing.T) {
	profiles := map[string]config.AgentProfile{
		"coder":    {Program: "opencode", Model: "m1", Enabled: true},
		"reviewer": {Program: "claude", Model: "m2", Enabled: false}, // disabled
		"chat":     {Program: "opencode", Model: "chat-model", Enabled: true},
	}
	configs := profilesToAgentConfigs(profiles)

	var chatHarnesses []string
	for _, c := range configs {
		if c.Role == "chat" {
			chatHarnesses = append(chatHarnesses, c.Harness)
		}
	}
	// claude must be in the fan-out even though its reviewer role is disabled.
	assert.ElementsMatch(t, []string{"claude", "opencode"}, chatHarnesses,
		"disabled harnesses must still receive a chat entry")
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
