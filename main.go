package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kastheco/kasmos/app"
	cmd2 "github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/daemon"
	initcmd "github.com/kastheco/kasmos/internal/initcmd"
	sentrypkg "github.com/kastheco/kasmos/internal/sentry"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/spf13/cobra"
)

var (
	version     = "0.2.0"
	programFlag string
	autoYesFlag bool
	daemonFlag  bool
	rootCmd     = &cobra.Command{
		Use:   "kas",
		Short: "kas - Manage multiple AI agents like Claude Code, Aider, Codex, and Amp.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			cfg := config.LoadConfig()
			if err := sentrypkg.Init(version, cfg.IsTelemetryEnabled()); err != nil {
				// Non-fatal: sentry failure should not prevent startup
				_ = err
			}
			defer sentrypkg.Flush()
			defer sentrypkg.RecoverPanic()

			log.Initialize(daemonFlag, cfg.IsTelemetryEnabled())
			defer log.Close()

			if daemonFlag {
				session.NotificationsEnabled = cfg.AreNotificationsEnabled()
				if err := daemon.RunDaemon(cfg); err != nil {
					log.ErrorLog.Printf("failed to start daemon: %v", err)
					return err
				}
				return nil
			}

			// Check if we're in a git repository
			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: kas must be run from within a git repository")
			}

			session.NotificationsEnabled = cfg.AreNotificationsEnabled()

			// Program flag overrides config
			program := cfg.DefaultProgram
			if programFlag != "" {
				program = programFlag
			}
			// AutoYes flag overrides config
			autoYes := cfg.AutoYes
			if autoYesFlag {
				autoYes = true
			}

			sentrypkg.SetContext(program, autoYes, filepath.Base(currentDir))

			if autoYes {
				defer func() {
					if err := daemon.LaunchDaemon(); err != nil {
						log.ErrorLog.Printf("failed to launch daemon: %v", err)
					}
				}()
			}
			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				log.ErrorLog.Printf("failed to stop daemon: %v", err)
			}

			return app.Run(ctx, program, autoYes)
		},
	}

	resetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Reset all stored instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			state := config.LoadState()
			storage, err := session.NewStorage(state)
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}
			if err := storage.DeleteAllInstances(); err != nil {
				return fmt.Errorf("failed to reset storage: %w", err)
			}
			fmt.Println("Storage has been reset successfully")

			if err := tmux.CleanupSessions(cmd2.MakeExecutor()); err != nil {
				return fmt.Errorf("failed to cleanup tmux sessions: %w", err)
			}
			fmt.Println("Tmux sessions have been cleaned up")

			cwd, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
			if err := git.CleanupWorktrees(cwd); err != nil {
				return fmt.Errorf("failed to cleanup worktrees: %w", err)
			}
			fmt.Println("Worktrees have been cleaned up")

			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				return err
			}
			fmt.Println("daemon has been stopped")

			return nil
		},
	}

	debugCmd = &cobra.Command{
		Use:   "debug",
		Short: "Print debug information like config paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			cfg := config.LoadConfig()

			configDir, err := config.GetConfigDir()
			if err != nil {
				return fmt.Errorf("failed to get config directory: %w", err)
			}
			configJson, _ := json.MarshalIndent(cfg, "", "  ")

			fmt.Printf("Config: %s\n%s\n", filepath.Join(configDir, config.ConfigFileName), configJson)

			return nil
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of kas",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kas version %s\n", version)
			fmt.Printf("https://github.com/kastheco/kasmos/releases/tag/v%s\n", version)
		},
	}
)

func init() {
	rootCmd.Flags().StringVarP(&programFlag, "program", "p", "",
		"Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')")
	rootCmd.Flags().BoolVarP(&autoYesFlag, "autoyes", "y", false,
		"[experimental] If enabled, all instances will automatically accept prompts")
	rootCmd.Flags().BoolVar(&daemonFlag, "daemon", false, "Run a program that loads all sessions"+
		" and runs autoyes mode on them.")

	// Hide the daemonFlag as it's only for internal use
	err := rootCmd.Flags().MarkHidden("daemon")
	if err != nil {
		panic(err)
	}

	var forceFlag bool
	var cleanFlag bool

	kasSetupCmd := &cobra.Command{
		Use:     "setup",
		Aliases: []string{"init"},
		Short:   "Configure agent harnesses, install superpowers, and scaffold project files",
		Long: `Run an interactive wizard to:
  1. Detect and select agent CLIs (claude, opencode, codex)
  2. Configure agent roles (coder, reviewer, planner) with model and tuning
  3. Install superpowers skills into each harness
  4. Write ~/.config/kasmos/config.toml and scaffold project-level agent files`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return initcmd.Run(initcmd.Options{
				Force: forceFlag,
				Clean: cleanFlag,
			})
		},
	}

	kasSetupCmd.Flags().BoolVar(&forceFlag, "force", false, "Overwrite existing project scaffold files")
	kasSetupCmd.Flags().BoolVar(&cleanFlag, "clean", false, "Ignore existing config, start with factory defaults")

	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(kasSetupCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, errUnhealthy) {
			os.Exit(1)
		}
		fmt.Println(err)
	}
}
