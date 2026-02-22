package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kastheco/klique/internal/initcmd/harness"
	"github.com/spf13/cobra"
)

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage agent skills",
	}
	cmd.AddCommand(newSkillsSyncCmd())
	cmd.AddCommand(newSkillsListCmd())
	return cmd
}

func newSkillsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List personal skills in ~/.agents/skills/",
		RunE:  runSkillsList,
	}
}

func runSkillsList(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	skillsDir := filepath.Join(home, ".agents", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No personal skills found. Install skills to ~/.agents/skills/")
			return nil
		}
		return err
	}

	fmt.Printf("Personal skills in %s:\n\n", skillsDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Check if it's a symlink (externally managed)
		fi, err := os.Lstat(filepath.Join(skillsDir, name))
		if err != nil {
			continue
		}
		managed := ""
		if fi.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(filepath.Join(skillsDir, name))
			managed = fmt.Sprintf(" -> %s (external)", target)
		}

		fmt.Printf("  %-30s%s\n", name, managed)
	}

	return nil
}

func newSkillsSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync personal skills from ~/.agents/skills/ to all harness skill directories",
		Long: `Reads personal skills from ~/.agents/skills/ and creates symlinks in each
detected harness's global skill directory:

  Claude Code:  ~/.claude/skills/
  OpenCode:     ~/.config/opencode/skills/
  Codex:        (native, no sync needed)

Replaces stale symlinks. Skips user-managed directories and symlink-based
skills (e.g. superpowers/) which are managed by 'kq init'.`,
		RunE: runSkillsSync,
	}
}

func runSkillsSync(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	registry := harness.NewRegistry()
	synced := 0

	for _, name := range registry.All() {
		h := registry.Get(name)
		if _, found := h.Detect(); !found {
			fmt.Printf("  %-12s SKIP (not installed)\n", name)
			continue
		}

		fmt.Printf("  %-12s ", name)
		if err := harness.SyncGlobalSkills(home, name); err != nil {
			fmt.Printf("FAILED: %v\n", err)
		} else {
			fmt.Println("OK")
			synced++
		}
	}

	if synced == 0 {
		fmt.Println("\nNo harnesses detected. Install claude, opencode, or codex first.")
	}

	return nil
}

func init() {
	rootCmd.AddCommand(newSkillsCmd())
}
