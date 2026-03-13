package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session/git"
	"github.com/spf13/cobra"
)

// TUIRunFunc is the function signature for the widened app entrypoint called by
// "kas tui". It is injected from main.go to avoid an import cycle
// (app → cmd and session → cmd, so cmd cannot import them back).
type TUIRunFunc func(ctx context.Context, program string, autoYes bool, version string, navOnly bool) error

// NewTUICmd returns the "kas tui" cobra command.
// It runs the Bubble Tea navigation UI directly — intended to run inside the
// left tmux pane of the two-pane layout created by the root launcher.
//
// version, programFlag, and autoYesFlag are shared with the root command so
// that `kas tui` honours the same --program / --autoyes flags.
// runFn is injected from main.go to avoid an import cycle between cmd and app.
func NewTUICmd(version string, programFlag *string, autoYesFlag *bool, runFn TUIRunFunc) *cobra.Command {
	var navOnly bool

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "run the kasmos navigation TUI (for use inside a tmux left pane)",
		Long: `tui starts the Bubble Tea navigation interface directly.

When launched by the root kas command inside a tmux session the --nav-only
flag is set automatically so that the left pane renders only the navigation
panel, leaving the right pane free for native agent terminal sessions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			cfg := config.LoadConfig()

			log.Initialize(false, cfg.IsTelemetryEnabled())
			defer log.Close()

			// kas tui must be run from within a git repository, same as root kas.
			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: kas tui must be run from within a git repository")
			}

			// Program flag overrides config (pointer shared with root command flags).
			program := cfg.DefaultProgram
			if programFlag != nil && *programFlag != "" {
				program = *programFlag
			}

			// AutoYes flag overrides config.
			autoYes := cfg.AutoYes
			if autoYesFlag != nil && *autoYesFlag {
				autoYes = true
			}

			// When running inside tmux and --nav-only was not explicitly set via the
			// command line, default to nav-only mode so the right pane stays a native
			// tmux pane rather than the embedded VT preview.
			if !cmd.Flags().Changed("nav-only") && os.Getenv("TMUX") != "" {
				navOnly = true
			}

			return runFn(ctx, program, autoYes, version, navOnly)
		},
	}

	cmd.Flags().BoolVar(&navOnly, "nav-only", false,
		"render only the navigation panel (for use in the left tmux pane)")

	return cmd
}
