package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/internal/initcmd/harness"
	"github.com/kastheco/kasmos/internal/initcmd/scaffold"
	"github.com/spf13/cobra"
)

func newScaffoldCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Manage project scaffold files",
	}
	cmd.AddCommand(newScaffoldSyncCmd())
	return cmd
}

func newScaffoldSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Re-sync embedded skills and agent prompt templates from the current binary",
		Long: `Re-syncs embedded skills, agent prompt templates, harness symlinks, and
enforcement hooks from the current binary. Uses existing TOML config for agent
settings — does not re-run the interactive wizard or modify config.`,
		SilenceUsage: true,
		RunE:         runScaffoldSync,
	}
}

// profilesToAgentConfigs converts a map of AgentProfile (keyed by role name)
// to a deterministic slice of harness.AgentConfig, including only enabled profiles.
//
// The "chat" role is special: the wizard stores it with a single harness but fans
// it out to every selected harness when building agent configs (so chat.md is
// written into every harness directory). We replicate that behaviour here by
// collecting all distinct harness names from ALL non-chat profiles (enabled or
// disabled) and emitting one chat entry per harness — mirroring the wizard which
// fans chat to every selected harness regardless of role enablement. If no other
// harnesses are present, we fall back to chat's own stored Program.
func profilesToAgentConfigs(profiles map[string]config.AgentProfile) []harness.AgentConfig {
	if len(profiles) == 0 {
		return nil
	}

	// Collect the distinct harness programs used by all non-chat profiles,
	// regardless of enabled state. wizard.State.ToAgentConfigs fans chat to
	// every *selected* harness independent of whether other roles on that harness
	// are currently enabled, so we mirror that behaviour here.
	harnessSet := map[string]struct{}{}
	for role, p := range profiles {
		if role == "chat" {
			continue
		}
		if p.Program != "" {
			harnessSet[p.Program] = struct{}{}
		}
	}

	roles := make([]string, 0, len(profiles))
	for role := range profiles {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	configs := make([]harness.AgentConfig, 0, len(roles))
	for _, role := range roles {
		p := profiles[role]
		if !p.Enabled {
			continue
		}
		if role == "chat" {
			// Fan chat out to every harness present in the project, mirroring
			// wizard.State.ToAgentConfigs so chat.md lands in every harness dir.
			chatHarnesses := make([]string, 0, len(harnessSet))
			for h := range harnessSet {
				chatHarnesses = append(chatHarnesses, h)
			}
			sort.Strings(chatHarnesses)
			if len(chatHarnesses) == 0 && p.Program != "" {
				// Fallback: no other enabled agents; use chat's own program.
				chatHarnesses = []string{p.Program}
			}
			for _, h := range chatHarnesses {
				configs = append(configs, harness.AgentConfig{
					Role:        role,
					Harness:     h,
					Model:       p.Model,
					Temperature: p.Temperature,
					Effort:      p.Effort,
					Enabled:     p.Enabled,
					ExtraFlags:  p.Flags,
				})
			}
			continue
		}
		configs = append(configs, harness.AgentConfig{
			Role:        role,
			Harness:     p.Program,
			Model:       p.Model,
			Temperature: p.Temperature,
			Effort:      p.Effort,
			Enabled:     p.Enabled,
			ExtraFlags:  p.Flags,
		})
	}
	return configs
}

func runScaffoldSync(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	tomlCfg, err := config.LoadTOMLConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if tomlCfg == nil {
		return fmt.Errorf("no config found — run 'kas setup' first to create .kasmos/config.toml")
	}

	agents := profilesToAgentConfigs(tomlCfg.Profiles)
	if len(agents) == 0 {
		return fmt.Errorf("config has no enabled agents — run 'kas setup' to configure agents")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	// Resolve to the nearest checkout root (the directory that contains the .git
	// file or directory). For a normal repo this is the repo root; for a git
	// worktree this is the worktree root — NOT the main repo root. This matches
	// kas setup (initcmd.go) which also scaffolds into os.Getwd() at the checkout
	// root, and avoids scattering scaffold files into subdirectories when the user
	// invokes the command from below the root.
	projectDir := cwd
	if root, rErr := resolveCheckoutRoot(cwd); rErr == nil {
		projectDir = root
	}

	fmt.Fprintf(out, "Syncing scaffold: %s\n", projectDir)
	results, err := scaffold.SyncScaffold(projectDir, agents)
	if err != nil {
		return fmt.Errorf("sync scaffold: %w", err)
	}

	updated := 0
	unchanged := 0
	for _, r := range results {
		if r.Created {
			fmt.Fprintf(out, "  %-40s updated\n", r.Path)
			updated++
		} else {
			unchanged++
		}
	}
	fmt.Fprintf(out, "\ndone. %d files updated, %d unchanged.\n", updated, unchanged)

	// Sync global skills to harness directories.
	registry := harness.NewRegistry()

	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		fmt.Fprintf(out, "\nWARNING: could not get home dir: %v — skipping global skill sync\n", homeErr)
	} else {
		fmt.Fprintln(out, "\nSyncing personal skills...")
		for _, name := range registry.All() {
			h := registry.Get(name)
			if _, found := h.Detect(); !found {
				fmt.Fprintf(out, "  %-12s SKIP (not installed)\n", name)
				continue
			}
			fmt.Fprintf(out, "  %-12s ", name)
			if err := harness.SyncGlobalSkills(home, name); err != nil {
				fmt.Fprintf(out, "FAILED: %v\n", err)
			} else {
				fmt.Fprintln(out, "OK")
			}
		}
	}

	// Install enforcement hooks for each detected harness.
	fmt.Fprintln(out, "\nInstalling enforcement hooks...")
	for _, name := range registry.All() {
		h := registry.Get(name)
		if _, found := h.Detect(); !found {
			fmt.Fprintf(out, "  %-12s SKIP (not installed)\n", name)
			continue
		}
		fmt.Fprintf(out, "  %-12s ", name)
		if err := h.InstallEnforcement(); err != nil {
			fmt.Fprintf(out, "FAILED: %v\n", err)
		} else {
			fmt.Fprintln(out, "OK")
		}
	}

	return nil
}

// resolveCheckoutRoot walks up from dir until it finds a directory containing a
// .git file or directory and returns that directory. It stops at the first .git
// entry found, so for a git worktree it returns the worktree root (the directory
// with the .git file), not the main repo root. Falls back to dir on failure.
func resolveCheckoutRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not a git repository (or any parent of %s)", dir)
		}
		dir = parent
	}
}

func init() {
	rootCmd.AddCommand(newScaffoldCmd())
}
