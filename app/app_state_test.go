package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnTaskAgent_PatchesMainBranchOpencodeConfig(t *testing.T) {
	dir := t.TempDir()

	for _, cmd := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Skipf("git setup failed (%v): %s", err, out)
		}
	}

	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	planFile := "plan-branch-patch.md"
	require.NoError(t, ps.Register(planFile, "test plan", "plan/patch", time.Now()))

	opencodeDir := filepath.Join(dir, ".opencode")
	require.NoError(t, os.MkdirAll(opencodeDir, 0o755))
	configPath := filepath.Join(opencodeDir, "opencode.jsonc")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"agent":{"planner":{"model":"anthropic/old","temperature":0.1,"reasoningEffort":"low"}}}`), 0o644))

	planTemp := 0.7
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	m := &home{
		taskState:      ps,
		activeRepoPath: dir,
		program:        "opencode",
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		toastManager:   overlay.NewToastManager(&sp),
		appConfig: &config.Config{
			PhaseRoles: map[string]string{
				"planning": "planner",
			},
			Profiles: map[string]config.AgentProfile{
				"planner": {
					Program:     "opencode",
					Model:       "claude-opus-4-6",
					Temperature: &planTemp,
					Effort:      "high",
					Enabled:     true,
				},
			},
		},
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	_, cmd := m.spawnTaskAgent(planFile, "plan", "plan prompt")
	if cmd != nil {
		_ = cmd()
	}

	var cfg map[string]any
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &cfg))
	agentCfg, ok := cfg["agent"].(map[string]any)
	require.True(t, ok)
	plannerCfg, ok := agentCfg["planner"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "anthropic/claude-opus-4-6", plannerCfg["model"])
	assert.InDelta(t, planTemp, plannerCfg["temperature"].(float64), 0.0001)
	assert.Equal(t, "high", plannerCfg["reasoningEffort"])
}

func TestSpawnElaborator_PatchesMainBranchOpencodeConfig(t *testing.T) {
	dir := t.TempDir()

	for _, cmd := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Skipf("git setup failed (%v): %s", err, out)
		}
	}

	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	ps, err := newTestPlanState(t, plansDir)
	require.NoError(t, err)
	planFile := "elaborator-branch-patch.md"
	require.NoError(t, ps.Register(planFile, "elaborator test plan", "plan/elaborator", time.Now()))

	opencodeDir := filepath.Join(dir, ".opencode")
	require.NoError(t, os.MkdirAll(opencodeDir, 0o755))
	configPath := filepath.Join(opencodeDir, "opencode.jsonc")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"agent":{"elaborator":{"model":"anthropic/old","temperature":0.1,"reasoningEffort":"low"}}}`), 0o644))

	planTemp := 0.65
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	m := &home{
		taskState:      ps,
		activeRepoPath: dir,
		program:        "opencode",
		nav:            ui.NewNavigationPanel(&sp),
		menu:           ui.NewMenu(),
		toastManager:   overlay.NewToastManager(&sp),
		appConfig: &config.Config{
			PhaseRoles: map[string]string{
				"elaborating": "elaborator",
			},
			Profiles: map[string]config.AgentProfile{
				"elaborator": {
					Program:     "opencode",
					Model:       "claude-opus-4-6",
					Temperature: &planTemp,
					Effort:      "low",
					Enabled:     true,
				},
			},
		},
		instanceFinalizers: make(map[*session.Instance]func()),
	}

	_, cmd := m.spawnElaborator(planFile)
	if cmd != nil {
		_ = cmd()
	}

	var cfg map[string]any
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &cfg))
	agentCfg, ok := cfg["agent"].(map[string]any)
	require.True(t, ok)
	elabCfg, ok := agentCfg["elaborator"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "anthropic/claude-opus-4-6", elabCfg["model"])
	assert.InDelta(t, planTemp, elabCfg["temperature"].(float64), 0.0001)
	assert.Equal(t, "low", elabCfg["reasoningEffort"])
}
