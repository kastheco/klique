package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// PRMonitorConfig holds configuration for the PR monitoring subsystem.
type PRMonitorConfig struct {
	// Enabled controls whether the PR monitor goroutine runs.
	Enabled bool

	// PollInterval is how often the monitor polls open pull requests. Default: 60s.
	PollInterval time.Duration

	// Reactions is the list of GitHub reactions to add to unprocessed review comments.
	// Default: ["eyes"].
	Reactions []string
}

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

	// AutoReviewFix enables the automatic review→fix→re-review loop.
	AutoReviewFix bool `toml:"auto_review_fix"`
	// MaxReviewFixCycles caps the review-fix loop iterations (0 = unlimited).
	MaxReviewFixCycles int `toml:"max_review_fix_cycles"`

	// SocketPath is the Unix domain socket path for the control API.
	// Defaults to ~/.config/kasmos/daemon.sock when empty.
	SocketPath string `toml:"socket_path"`

	// PRMonitor holds configuration for the PR monitoring subsystem.
	PRMonitor PRMonitorConfig `toml:"pr_monitor"`
}

// tomlPRMonitorConfig is the raw TOML representation of PRMonitorConfig.
type tomlPRMonitorConfig struct {
	Enabled         bool     `toml:"enabled"`
	PollIntervalSec float64  `toml:"poll_interval_sec"`
	Reactions       []string `toml:"reactions"`
}

// tomlDaemonConfig is the raw TOML representation, using seconds for duration
// fields so the config file stays human-readable.
type tomlDaemonConfig struct {
	PollIntervalSec    float64             `toml:"poll_interval_sec"`
	Repos              []string            `toml:"repos"`
	AutoAdvance        bool                `toml:"auto_advance"`
	AutoAdvanceWaves   bool                `toml:"auto_advance_waves"`
	AutoReviewFix      bool                `toml:"auto_review_fix"`
	MaxReviewFixCycles int                 `toml:"max_review_fix_cycles"`
	SocketPath         string              `toml:"socket_path"`
	PRMonitor          tomlPRMonitorConfig `toml:"pr_monitor"`
}

// defaultDaemonConfig returns a DaemonConfig populated with sensible defaults.
func defaultDaemonConfig() *DaemonConfig {
	return &DaemonConfig{
		PollInterval:     2 * time.Second,
		AutoAdvance:      false,
		AutoAdvanceWaves: false,
		PRMonitor: PRMonitorConfig{
			Enabled:      false,
			PollInterval: 60 * time.Second,
			Reactions:    []string{"eyes"},
		},
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
	cfg.AutoReviewFix = tc.AutoReviewFix
	cfg.MaxReviewFixCycles = tc.MaxReviewFixCycles
	cfg.SocketPath = tc.SocketPath

	// PRMonitor section
	cfg.PRMonitor.Enabled = tc.PRMonitor.Enabled
	if tc.PRMonitor.PollIntervalSec > 0 {
		cfg.PRMonitor.PollInterval = time.Duration(tc.PRMonitor.PollIntervalSec * float64(time.Second))
	}
	if tc.PRMonitor.Reactions != nil {
		// Trim empty strings from the reactions slice.
		filtered := make([]string, 0, len(tc.PRMonitor.Reactions))
		for _, r := range tc.PRMonitor.Reactions {
			if r != "" {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			cfg.PRMonitor.Reactions = []string{"eyes"}
		} else {
			cfg.PRMonitor.Reactions = filtered
		}
	}

	return cfg, nil
}
