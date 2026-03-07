package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// DaemonConfig holds the configuration for the background daemon.
type DaemonConfig struct {
	// PollInterval is how often the daemon scans for signals. Default: 2s.
	PollInterval time.Duration `toml:"poll_interval"`

	// Repos is the list of repo root paths to manage on startup.
	Repos []string `toml:"repos"`

	// AutoAdvance instructs the daemon to automatically start implementation
	// after the planning phase completes.
	AutoAdvance bool `toml:"auto_advance"`

	// AutoAdvanceWaves instructs the daemon to automatically advance between
	// waves when all tasks in a wave complete.
	AutoAdvanceWaves bool `toml:"auto_advance_waves"`

	// SocketPath is the Unix domain socket path for the control API.
	// Defaults to ~/.config/kasmos/daemon.sock when empty.
	SocketPath string `toml:"socket_path"`
}

// tomlDaemonConfig is the raw TOML representation, using seconds for duration
// fields so the config file stays human-readable.
type tomlDaemonConfig struct {
	PollIntervalSec  float64  `toml:"poll_interval_sec"`
	Repos            []string `toml:"repos"`
	AutoAdvance      bool     `toml:"auto_advance"`
	AutoAdvanceWaves bool     `toml:"auto_advance_waves"`
	SocketPath       string   `toml:"socket_path"`
}

// defaultDaemonConfig returns a DaemonConfig populated with sensible defaults.
func defaultDaemonConfig() *DaemonConfig {
	return &DaemonConfig{
		PollInterval:     2 * time.Second,
		AutoAdvance:      false,
		AutoAdvanceWaves: false,
	}
}

// LoadDaemonConfig reads the daemon configuration from the given path.
// If path is empty it defaults to ~/.config/kasmos/daemon.toml.
// Missing files are silently ignored; defaults are returned instead.
func LoadDaemonConfig(path string) (*DaemonConfig, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("daemon config: resolve home dir: %w", err)
		}
		path = filepath.Join(home, ".config", "kasmos", "daemon.toml")
	}

	cfg := defaultDaemonConfig()

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("daemon config: read %s: %w", path, err)
	}

	var tc tomlDaemonConfig
	if _, err := toml.Decode(string(raw), &tc); err != nil {
		return nil, fmt.Errorf("daemon config: parse %s: %w", path, err)
	}

	if tc.PollIntervalSec > 0 {
		cfg.PollInterval = time.Duration(tc.PollIntervalSec * float64(time.Second))
	}
	if len(tc.Repos) > 0 {
		cfg.Repos = tc.Repos
	}
	cfg.AutoAdvance = tc.AutoAdvance
	cfg.AutoAdvanceWaves = tc.AutoAdvanceWaves
	cfg.SocketPath = tc.SocketPath

	return cfg, nil
}
