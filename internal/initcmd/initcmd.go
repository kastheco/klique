package initcmd

import (
	"fmt"
	"os"

	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/internal/initcmd/harness"
	"github.com/kastheco/klique/internal/initcmd/scaffold"
	"github.com/kastheco/klique/internal/initcmd/wizard"
)

// Options holds the CLI flags for kq init.
type Options struct {
	Force bool // overwrite existing project scaffold files
	Clean bool // ignore existing config, start with factory defaults
}

// Run executes the kq init workflow.
func Run(opts Options) error {
	registry := harness.NewRegistry()

	// Load existing config unless --clean
	var existing *config.TOMLConfigResult
	if !opts.Clean {
		var err error
		existing, err = config.LoadTOMLConfig()
		if err != nil {
			fmt.Printf("Warning: could not load existing config: %v\n", err)
		}
	}

	// Run interactive wizard
	state, err := wizard.Run(registry, existing)
	if err != nil {
		return fmt.Errorf("wizard: %w", err)
	}

	// Stage 4a: Install superpowers into selected harnesses
	fmt.Println("\nInstalling superpowers...")
	for _, name := range state.SelectedHarness {
		h := registry.Get(name)
		if h == nil {
			continue
		}
		fmt.Printf("  %-12s ", name)
		if err := h.InstallSuperpowers(); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			// Non-fatal: continue with other harnesses
		} else {
			fmt.Println("OK")
		}
	}

	// Stage 4b: Write TOML config
	fmt.Println("\nWriting config...")
	tc := state.ToTOMLConfig()
	if err := config.SaveTOMLConfig(tc); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	tomlPath, _ := config.GetTOMLConfigPath()
	fmt.Printf("  %s\n", tomlPath)

	// Stage 4c: Scaffold project files
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	agentConfigs := state.ToAgentConfigs()
	fmt.Printf("\nScaffolding project: %s\n", projectDir)
	results, err := scaffold.ScaffoldAll(projectDir, agentConfigs, opts.Force)
	if err != nil {
		return fmt.Errorf("scaffold: %w", err)
	}
	for _, r := range results {
		status := "OK"
		if !r.Created {
			status = "SKIP (exists)"
		}
		fmt.Printf("  %-40s %s\n", r.Path, status)
	}

	fmt.Println("\nDone! Run 'kq' to start.")
	return nil
}
