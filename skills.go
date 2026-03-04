package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kastheco/kasmos/internal/initcmd/harness"
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

// listSkillEntries prints skill entries from dir to out. Each entry that is a
// real directory or a symlink-to-directory is listed with its name and, for
// symlinks, the symlink target annotated as "(external)".
func listSkillEntries(dir string, out io.Writer) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		fullPath := filepath.Join(dir, name)

		// Accept real dirs and symlinks-to-dirs; skip plain files.
		fi, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}
		isSymlink := fi.Mode()&os.ModeSymlink != 0
		if !entry.IsDir() && !isSymlink {
			continue // plain file
		}
		// If symlink, verify target is a directory.
		if isSymlink {
			targetInfo, err := os.Stat(fullPath) // follows symlink
			if err != nil || !targetInfo.IsDir() {
				continue
			}
		}

		managed := ""
		if isSymlink {
			target, _ := os.Readlink(fullPath)
			managed = fmt.Sprintf(" -> %s (external)", target)
		}

		fmt.Fprintf(out, "  %-30s%s\n", name, managed)
	}
}

func runSkillsList(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	skillsDir := filepath.Join(home, ".agents", "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		fmt.Fprintln(out, "No personal skills found. Install skills to ~/.agents/skills/")
	} else {
		fmt.Fprintf(out, "Personal skills in %s:\n\n", skillsDir)
		listSkillEntries(skillsDir, out)
	}

	// Show project-level skills if cwd contains .agents/skills/.
	cwd, err := os.Getwd()
	if err == nil {
		projectSkillsDir := filepath.Join(cwd, ".agents", "skills")
		if info, err := os.Stat(projectSkillsDir); err == nil && info.IsDir() {
			fmt.Fprintf(out, "\nProject skills in %s:\n\n", projectSkillsDir)
			listSkillEntries(projectSkillsDir, out)
		}
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
skills (externally managed).`,
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
