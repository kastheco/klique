package config

import (
	"github.com/kastheco/kasmos/log"
	"os"
	"path/filepath"
	"regexp"
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

func TestGetClaudeCommand(t *testing.T) {
	t.Run("finds claude in PATH", func(t *testing.T) {
		originalPath := os.Getenv("PATH")
		tempDir := t.TempDir()
		claudePath := filepath.Join(tempDir, "claude")

		err := os.WriteFile(claudePath, []byte("#!/bin/bash\necho 'mock claude'"), 0755)
		require.NoError(t, err)

		t.Setenv("PATH", tempDir+":"+originalPath)
		t.Setenv("SHELL", "/bin/bash")

		result, err := GetClaudeCommand()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(result, "claude"))
	})

	t.Run("handles missing claude command", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("PATH", tempDir)
		t.Setenv("SHELL", "/bin/bash")

		result, err := GetClaudeCommand()

		assert.Error(t, err)
		assert.Equal(t, "", result)
		assert.Contains(t, err.Error(), "claude command not found")
	})

	t.Run("handles empty SHELL environment", func(t *testing.T) {
		originalPath := os.Getenv("PATH")
		originalShell := os.Getenv("SHELL")
		tempDir := t.TempDir()
		claudePath := filepath.Join(tempDir, "claude")

		err := os.WriteFile(claudePath, []byte("#!/bin/bash\necho 'mock claude'"), 0755)
		require.NoError(t, err)

		t.Setenv("PATH", tempDir+":"+originalPath)
		os.Unsetenv("SHELL")
		t.Cleanup(func() {
			if originalShell != "" {
				os.Setenv("SHELL", originalShell)
			}
		})

		result, err := GetClaudeCommand()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(result, "claude"))
	})

	t.Run("handles alias parsing", func(t *testing.T) {
		aliasRegex := regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)

		output := "claude: aliased to /usr/local/bin/claude"
		matches := aliasRegex.FindStringSubmatch(output)
		assert.Len(t, matches, 2)
		assert.Equal(t, "/usr/local/bin/claude", matches[1])

		output = "/usr/local/bin/claude"
		matches = aliasRegex.FindStringSubmatch(output)
		assert.Len(t, matches, 0)
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
}

func TestGetConfigDir(t *testing.T) {
	t.Run("returns valid config directory", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		configDir, err := GetConfigDir()

		require.NoError(t, err)
		assert.NotEmpty(t, configDir)
		assert.True(t, strings.HasSuffix(configDir, filepath.Join(".config", "kasmos")))
		assert.True(t, filepath.IsAbs(configDir))
	})

	t.Run("migrates legacy .hivemind to .config/kasmos", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		oldDir := filepath.Join(tempHome, ".hivemind")
		require.NoError(t, os.MkdirAll(oldDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(oldDir, "config.json"),
			[]byte(`{"auto_yes":true}`), 0644))

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(configDir, filepath.Join(".config", "kasmos")))

		// Old dir should be gone
		_, err = os.Stat(oldDir)
		assert.True(t, os.IsNotExist(err))

		// New dir should contain the migrated file with original contents intact
		data, err := os.ReadFile(filepath.Join(configDir, "config.json"))
		require.NoError(t, err)
		assert.Equal(t, `{"auto_yes":true}`, string(data))
	})

	t.Run("migrates legacy .klique to .config/kasmos", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		oldDir := filepath.Join(tempHome, ".klique")
		require.NoError(t, os.MkdirAll(oldDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(oldDir, "config.json"),
			[]byte(`{"auto_yes":true}`), 0644))

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(configDir, filepath.Join(".config", "kasmos")))

		// Old dir should be gone
		_, err = os.Stat(oldDir)
		assert.True(t, os.IsNotExist(err))

		// New dir should contain the migrated file with original contents intact
		data, err := os.ReadFile(filepath.Join(configDir, "config.json"))
		require.NoError(t, err)
		assert.Equal(t, `{"auto_yes":true}`, string(data))
	})

	t.Run("skips migration when .config/kasmos already exists", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		newDir := filepath.Join(tempHome, ".config", "kasmos")
		oldDir := filepath.Join(tempHome, ".hivemind")
		require.NoError(t, os.MkdirAll(newDir, 0755))
		require.NoError(t, os.MkdirAll(oldDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(oldDir, "config.json"), []byte(`{"auto_yes":false}`), 0644))

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(configDir, filepath.Join(".config", "kasmos")))

		// Old dir should still exist with original contents untouched
		_, err = os.Stat(oldDir)
		assert.NoError(t, err)
		data, err := os.ReadFile(filepath.Join(oldDir, "config.json"))
		require.NoError(t, err)
		assert.Equal(t, `{"auto_yes":false}`, string(data))
	})

	t.Run("no-ops when neither dir exists", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		configDir, err := GetConfigDir()
		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(configDir, filepath.Join(".config", "kasmos")))
	})
}

func TestLoadConfig(t *testing.T) {
	t.Run("returns default config when file doesn't exist", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		config := LoadConfig()

		assert.NotNil(t, config)
		assert.NotEmpty(t, config.DefaultProgram)
		assert.False(t, config.AutoYes)
		assert.Equal(t, 1000, config.DaemonPollInterval)
		assert.NotEmpty(t, config.BranchPrefix)
	})

	t.Run("loads valid config file", func(t *testing.T) {
		tempHome := t.TempDir()
		configDir := filepath.Join(tempHome, ".config", "kasmos")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		configPath := filepath.Join(configDir, ConfigFileName)
		configContent := `{
			"default_program": "test-claude",
			"auto_yes": true,
			"daemon_poll_interval": 2000,
			"branch_prefix": "test/"
		}`
		err = os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		t.Setenv("HOME", tempHome)

		config := LoadConfig()

		assert.NotNil(t, config)
		assert.Equal(t, "test-claude", config.DefaultProgram)
		assert.True(t, config.AutoYes)
		assert.Equal(t, 2000, config.DaemonPollInterval)
		assert.Equal(t, "test/", config.BranchPrefix)
	})

	t.Run("returns default config on invalid JSON", func(t *testing.T) {
		tempHome := t.TempDir()
		configDir := filepath.Join(tempHome, ".config", "kasmos")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		configPath := filepath.Join(configDir, ConfigFileName)
		invalidContent := `{"invalid": json content}`
		err = os.WriteFile(configPath, []byte(invalidContent), 0644)
		require.NoError(t, err)

		t.Setenv("HOME", tempHome)

		config := LoadConfig()

		assert.NotNil(t, config)
		assert.NotEmpty(t, config.DefaultProgram)
		assert.False(t, config.AutoYes)
		assert.Equal(t, 1000, config.DaemonPollInterval)
	})
}

func TestSaveConfig(t *testing.T) {
	t.Run("saves config to file", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		testConfig := &Config{
			DefaultProgram:     "test-program",
			AutoYes:            true,
			DaemonPollInterval: 3000,
			BranchPrefix:       "test-branch/",
		}

		err := SaveConfig(testConfig)
		assert.NoError(t, err)

		configDir := filepath.Join(tempHome, ".config", "kasmos")
		configPath := filepath.Join(configDir, ConfigFileName)

		assert.FileExists(t, configPath)

		loadedConfig := LoadConfig()
		assert.Equal(t, testConfig.DefaultProgram, loadedConfig.DefaultProgram)
		assert.Equal(t, testConfig.AutoYes, loadedConfig.AutoYes)
		assert.Equal(t, testConfig.DaemonPollInterval, loadedConfig.DaemonPollInterval)
		assert.Equal(t, testConfig.BranchPrefix, loadedConfig.BranchPrefix)
	})
}
