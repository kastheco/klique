package config

import (
	"github.com/kastheco/kasmos/log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain runs before all tests to set up the test environment
func TestMain(m *testing.M) {
	log.Initialize(false)
	code := m.Run()
	log.Close()
	os.Exit(code)
}

func TestGetDefaultCommand(t *testing.T) {
	t.Run("finds opencode in PATH", func(t *testing.T) {
		tempDir := t.TempDir()
		opencodePath := filepath.Join(tempDir, "opencode")

		err := os.WriteFile(opencodePath, []byte("#!/bin/bash\necho 'mock opencode'"), 0755)
		require.NoError(t, err)

		t.Setenv("PATH", tempDir)
		t.Setenv("SHELL", "/bin/sh")

		result, err := GetDefaultCommand()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(result, "opencode"))
	})

	t.Run("falls back to claude when opencode is missing", func(t *testing.T) {
		tempDir := t.TempDir()
		claudePath := filepath.Join(tempDir, "claude")

		err := os.WriteFile(claudePath, []byte("#!/bin/bash\necho 'mock claude'"), 0755)
		require.NoError(t, err)

		t.Setenv("PATH", tempDir)
		t.Setenv("SHELL", "/bin/sh")

		result, err := GetDefaultCommand()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(result, "claude"))
	})

	t.Run("handles missing opencode and claude commands", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("PATH", tempDir)
		t.Setenv("SHELL", "/bin/sh")

		result, err := GetDefaultCommand()

		assert.Error(t, err)
		assert.Equal(t, "", result)
		assert.Contains(t, err.Error(), "neither opencode nor claude command found")
	})

	t.Run("handles empty SHELL environment", func(t *testing.T) {
		tempDir := t.TempDir()
		opencodePath := filepath.Join(tempDir, "opencode")

		err := os.WriteFile(opencodePath, []byte("#!/bin/bash\necho 'mock opencode'"), 0755)
		require.NoError(t, err)

		t.Setenv("PATH", tempDir)
		t.Setenv("HOME", tempDir)
		t.Setenv("SHELL", "")

		result, err := GetDefaultCommand()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(result, "opencode"))
	})

	t.Run("prefers opencode when both commands exist", func(t *testing.T) {
		tempDir := t.TempDir()
		opencodePath := filepath.Join(tempDir, "opencode")
		claudePath := filepath.Join(tempDir, "claude")

		err := os.WriteFile(opencodePath, []byte("#!/bin/bash\necho 'mock opencode'"), 0755)
		require.NoError(t, err)
		err = os.WriteFile(claudePath, []byte("#!/bin/bash\necho 'mock claude'"), 0755)
		require.NoError(t, err)

		t.Setenv("PATH", tempDir)
		t.Setenv("SHELL", "/bin/sh")

		result, err := GetDefaultCommand()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(result, "opencode"))
	})

	t.Run("handles alias parsing", func(t *testing.T) {
		assert.Equal(t, "/usr/local/bin/opencode", parseCommandOutput("opencode: aliased to /usr/local/bin/opencode"))
		assert.Equal(t, "/usr/local/bin/opencode", parseCommandOutput("/usr/local/bin/opencode"))
		assert.Equal(t, "", parseCommandOutput("   \n"))
	})
}

func TestDefaultConfig(t *testing.T) {
	t.Run("creates config with default values", func(t *testing.T) {
		config := DefaultConfig()

		assert.NotNil(t, config)
		assert.NotEmpty(t, config.DefaultProgram)
		assert.False(t, config.AutoYes)
		assert.Equal(t, 1000, config.DaemonPollInterval)
		assert.NotEmpty(t, config.BranchPrefix)
		assert.True(t, strings.HasSuffix(config.BranchPrefix, "/"))
	})

	t.Run("falls back to opencode when command detection fails", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("PATH", tempDir)
		t.Setenv("SHELL", "/bin/sh")

		config := DefaultConfig()

		assert.Equal(t, "opencode", config.DefaultProgram)
	})
}

func TestGetConfigDir(t *testing.T) {
	runGit := func(t *testing.T, repo string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, string(out))
	}

	t.Run("returns .kasmos relative to working directory", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		configDir, err := GetConfigDir()

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tempDir, ".kasmos"), configDir)
	})

	t.Run("returns .kasmos under repo root from nested git directory", func(t *testing.T) {
		repoDir := t.TempDir()
		t.Setenv("HOME", t.TempDir())

		runGit(t, repoDir, "init", "-b", "main")
		runGit(t, repoDir, "config", "user.email", "test@example.com")
		runGit(t, repoDir, "config", "user.name", "test")
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("init\n"), 0o644))
		runGit(t, repoDir, "add", ".")
		runGit(t, repoDir, "commit", "-m", "initial")

		nestedDir := filepath.Join(repoDir, "internal", "nested")
		require.NoError(t, os.MkdirAll(nestedDir, 0o755))
		t.Chdir(nestedDir)

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(repoDir, ".kasmos"), configDir)
	})

	t.Run("returns .kasmos under main repo root from worktree", func(t *testing.T) {
		repoDir := t.TempDir()
		t.Setenv("HOME", t.TempDir())

		runGit(t, repoDir, "init", "-b", "main")
		runGit(t, repoDir, "config", "user.email", "test@example.com")
		runGit(t, repoDir, "config", "user.name", "test")
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("init\n"), 0o644))
		runGit(t, repoDir, "add", ".")
		runGit(t, repoDir, "commit", "-m", "initial")

		runGit(t, repoDir, "branch", "plan/worktree-config")
		worktreeParent := t.TempDir()
		worktreeDir := filepath.Join(worktreeParent, "worktree-config")
		runGit(t, repoDir, "worktree", "add", worktreeDir, "plan/worktree-config")
		t.Chdir(worktreeDir)

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(repoDir, ".kasmos"), configDir)
	})

	t.Run("migrates config.toml from legacy XDG location", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)
		projectDir := t.TempDir()
		t.Chdir(projectDir)

		// Create legacy config at ~/.config/kasmos/
		legacyDir := filepath.Join(tempHome, ".config", "kasmos")
		require.NoError(t, os.MkdirAll(legacyDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(legacyDir, "config.toml"),
			[]byte("[ui]\nanimate_banner = true\n"), 0644))

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(projectDir, ".kasmos"), configDir)

		// Config should be copied to new location
		data, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "animate_banner")

		// Legacy file should still exist (copy, not move)
		assert.FileExists(t, filepath.Join(legacyDir, "config.toml"))
	})

	t.Run("skips migration when config already exists in .kasmos", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)
		projectDir := t.TempDir()
		t.Chdir(projectDir)

		// Create config in both locations
		kasmosDir := filepath.Join(projectDir, ".kasmos")
		require.NoError(t, os.MkdirAll(kasmosDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(kasmosDir, "config.toml"),
			[]byte("[ui]\nanimate_banner = false\n"), 0644))

		legacyDir := filepath.Join(tempHome, ".config", "kasmos")
		require.NoError(t, os.MkdirAll(legacyDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(legacyDir, "config.toml"),
			[]byte("[ui]\nanimate_banner = true\n"), 0644))

		configDir, err := GetConfigDir()
		require.NoError(t, err)

		// Should use existing .kasmos config, NOT overwrite with legacy
		data, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "animate_banner = false")
	})

	t.Run("no-ops when neither location has config", func(t *testing.T) {
		projectDir := t.TempDir()
		t.Chdir(projectDir)
		t.Setenv("HOME", t.TempDir())

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(projectDir, ".kasmos"), configDir)
	})
}

func TestConfigFromTOML(t *testing.T) {
	falseVal := false
	zeroCycles := 0
	threshold := 3
	result := &TOMLConfigResult{
		DefaultProgram:         "test-cmd",
		AutoYes:                true,
		DaemonPollInterval:     2500,
		BranchPrefix:           "test/",
		NotificationsEnabled:   &falseVal,
		Profiles:               map[string]AgentProfile{"coder": {Program: "opencode", Enabled: true}},
		PhaseRoles:             map[string]string{"implementing": "coder"},
		AnimateBanner:          true,
		AutoAdvanceWaves:       true,
		AutoReviewFix:          &falseVal,
		MaxReviewFixCycles:     &zeroCycles,
		TelemetryEnabled:       &falseVal,
		DatabaseURL:            "https://example.test/store",
		BlueprintSkipThreshold: &threshold,
	}

	cfg := configFromTOML(result)
	require.NotNil(t, cfg)
	assert.Equal(t, "test-cmd", cfg.DefaultProgram)
	assert.True(t, cfg.AutoYes)
	assert.Equal(t, 2500, cfg.DaemonPollInterval)
	assert.Equal(t, "test/", cfg.BranchPrefix)
	require.NotNil(t, cfg.NotificationsEnabled)
	assert.False(t, cfg.AreNotificationsEnabled())
	assert.True(t, cfg.AnimateBanner)
	assert.True(t, cfg.AutoAdvanceWaves)
	assert.False(t, cfg.AutoReviewFix)
	assert.Equal(t, 0, cfg.MaxReviewFixCycles)
	require.NotNil(t, cfg.TelemetryEnabled)
	assert.False(t, cfg.IsTelemetryEnabled())
	assert.Equal(t, "https://example.test/store", cfg.DatabaseURL)
	assert.Equal(t, 3, cfg.BlueprintSkipThreshold())
	assert.Equal(t, "opencode", cfg.Profiles["coder"].Program)
}

func TestConfigFromTOML_Defaults(t *testing.T) {
	result := &TOMLConfigResult{
		Profiles:   map[string]AgentProfile{},
		PhaseRoles: map[string]string{},
	}

	cfg := configFromTOML(result)
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.DefaultProgram)
	assert.Equal(t, 1000, cfg.DaemonPollInterval)
	assert.NotEmpty(t, cfg.BranchPrefix)
	assert.True(t, cfg.AreNotificationsEnabled())
}

func TestLoadConfig(t *testing.T) {
	t.Run("returns default config when file doesn't exist", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		t.Setenv("HOME", t.TempDir())

		config := LoadConfig()

		assert.NotNil(t, config)
		assert.NotEmpty(t, config.DefaultProgram)
		assert.False(t, config.AutoYes)
		assert.Equal(t, 1000, config.DaemonPollInterval)
		assert.NotEmpty(t, config.BranchPrefix)
		assert.FileExists(t, filepath.Join(tempDir, ".kasmos", TOMLConfigFileName))
	})

	t.Run("loads valid config file", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		t.Setenv("HOME", t.TempDir())

		configDir := filepath.Join(tempDir, ".kasmos")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		configPath := filepath.Join(configDir, TOMLConfigFileName)
		configContent := `default_program = "test-claude"
auto_yes = true
daemon_poll_interval = 2000
branch_prefix = "test/"
`
		err = os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		config := LoadConfig()

		assert.NotNil(t, config)
		assert.Equal(t, "test-claude", config.DefaultProgram)
		assert.True(t, config.AutoYes)
		assert.Equal(t, 2000, config.DaemonPollInterval)
		assert.Equal(t, "test/", config.BranchPrefix)
	})

	t.Run("returns default config on invalid TOML", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		t.Setenv("HOME", t.TempDir())

		configDir := filepath.Join(tempDir, ".kasmos")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		configPath := filepath.Join(configDir, TOMLConfigFileName)
		invalidContent := `[invalid toml content`
		err = os.WriteFile(configPath, []byte(invalidContent), 0644)
		require.NoError(t, err)

		config := LoadConfig()

		assert.NotNil(t, config)
		assert.NotEmpty(t, config.DefaultProgram)
		assert.False(t, config.AutoYes)
		assert.Equal(t, 1000, config.DaemonPollInterval)
	})

	t.Run("toml false and zero values are respected", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		t.Setenv("HOME", t.TempDir())

		configDir := filepath.Join(tempDir, ".kasmos")
		require.NoError(t, os.MkdirAll(configDir, 0755))

		tomlPath := filepath.Join(configDir, TOMLConfigFileName)
		tomlContent := `[ui]
auto_review_fix = false
max_review_fix_cycles = 0
`
		require.NoError(t, os.WriteFile(tomlPath, []byte(tomlContent), 0644))

		config := LoadConfig()
		require.NotNil(t, config)
		assert.False(t, config.AutoReviewFix)
		assert.Equal(t, 0, config.MaxReviewFixCycles)
	})
}

func TestLoadConfig_MigratesJSON(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("HOME", t.TempDir())

	configDir := filepath.Join(tempDir, ".kasmos")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	jsonContent := `{
		"default_program": "migrated-claude",
		"auto_yes": true,
		"daemon_poll_interval": 3000,
		"branch_prefix": "migrated/",
		"auto_advance_waves": true,
		"auto_review_fix": false,
		"max_review_fix_cycles": 0,
		"notifications_enabled": false
	}`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), []byte(jsonContent), 0o644))

	cfg := LoadConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, "migrated-claude", cfg.DefaultProgram)
	assert.True(t, cfg.AutoYes)
	assert.Equal(t, 3000, cfg.DaemonPollInterval)
	assert.Equal(t, "migrated/", cfg.BranchPrefix)
	assert.True(t, cfg.AutoAdvanceWaves)
	assert.False(t, cfg.AutoReviewFix)
	assert.Equal(t, 0, cfg.MaxReviewFixCycles)
	require.NotNil(t, cfg.NotificationsEnabled)
	assert.False(t, cfg.AreNotificationsEnabled())
	assert.NoFileExists(t, filepath.Join(configDir, "config.json"))
	assert.FileExists(t, filepath.Join(configDir, "config.json.migrated"))
	assert.FileExists(t, filepath.Join(configDir, TOMLConfigFileName))

	written, err := os.ReadFile(filepath.Join(configDir, TOMLConfigFileName))
	require.NoError(t, err)
	assert.Contains(t, string(written), `default_program = "migrated-claude"`)
	assert.Contains(t, string(written), `auto_advance_waves = true`)
	assert.Contains(t, string(written), `auto_review_fix = false`)
	assert.Contains(t, string(written), `max_review_fix_cycles = 0`)
	assert.Contains(t, string(written), `notifications_enabled = false`)
}

func boolPtr(b bool) *bool { return &b }

func TestIsTelemetryEnabled(t *testing.T) {
	tests := []struct {
		name     string
		field    *bool
		expected bool
	}{
		{"nil defaults to true", nil, true},
		{"explicit true", boolPtr(true), true},
		{"explicit false", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{TelemetryEnabled: tt.field}
			assert.Equal(t, tt.expected, cfg.IsTelemetryEnabled())
		})
	}
}
