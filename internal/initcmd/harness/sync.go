package harness

import (
	"fmt"
	"os"
	"path/filepath"
)

// GlobalSkillsDir returns the global skills directory for a given harness.
// Returns the absolute path where the harness expects to find skill symlinks.
func GlobalSkillsDir(home, harnessName string) string {
	switch harnessName {
	case "claude":
		return filepath.Join(home, ".claude", "skills")
	case "opencode":
		return filepath.Join(home, ".config", "opencode", "skills")
	case "codex":
		return filepath.Join(home, ".agents", "skills") // native, same as canonical
	default:
		return filepath.Join(home, "."+harnessName, "skills")
	}
}

// SyncGlobalSkills creates symlinks from a harness's global skill directory
// to ~/.agents/skills/<skill> for each personal skill.
//
// Skips entries in ~/.agents/skills/ that are themselves symlinks (managed
// externally, e.g. superpowers). Replaces existing symlinks in the destination.
// Skips non-symlink entries in the destination (user-managed directories).
//
// For codex, this is a no-op since codex reads from ~/.agents/skills/ directly.
func SyncGlobalSkills(home, harnessName string) error {
	if harnessName == "codex" {
		return nil // codex reads ~/.agents/skills/ natively
	}

	canonicalDir := filepath.Join(home, ".agents", "skills")
	entries, err := os.ReadDir(canonicalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", canonicalDir, err)
	}

	destDir := GlobalSkillsDir(home, harnessName)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create %s skills dir: %w", harnessName, err)
	}

	for _, entry := range entries {
		name := entry.Name()

		// Skip non-directories
		if !entry.IsDir() {
			// Could be a file like .skill-lock.json
			continue
		}

		// Skip entries that are themselves symlinks (e.g. superpowers/)
		// These are managed by InstallSuperpowers, not by us.
		srcPath := filepath.Join(canonicalDir, name)
		fi, err := os.Lstat(srcPath)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			continue
		}

		link := filepath.Join(destDir, name)

		// Compute relative symlink target from destDir to canonicalDir
		relTarget, err := filepath.Rel(destDir, srcPath)
		if err != nil {
			return fmt.Errorf("compute relative path for %s: %w", name, err)
		}

		// Check if link already exists
		if lfi, err := os.Lstat(link); err == nil {
			if lfi.Mode()&os.ModeSymlink != 0 {
				// Replace existing symlink
				if err := os.Remove(link); err != nil {
					return fmt.Errorf("remove existing symlink %s: %w", name, err)
				}
			} else {
				// Non-symlink entry (user-managed) — skip
				continue
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", link, err)
		}

		if err := os.Symlink(relTarget, link); err != nil {
			return fmt.Errorf("symlink %s skill %s: %w", harnessName, name, err)
		}
	}

	return nil
}
