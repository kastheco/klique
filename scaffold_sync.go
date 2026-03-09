package main

import (
	"fmt"
	"os"
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
func profilesToAgentConfigs(profiles map[string]config.AgentProfile) []harness.AgentConfig {
	if len(profiles) == 0 {
		return nil
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

	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
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

func init() {
	rootCmd.AddCommand(newScaffoldCmd())
}
