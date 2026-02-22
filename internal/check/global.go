package check

import (
	"os"
	"path/filepath"

	"github.com/kastheco/klique/internal/initcmd/harness"
)

// AuditGlobal checks ~/.agents/skills/ against one harness's global skill dir.
// For codex, all real skills in ~/.agents/skills/ are considered natively synced.
func AuditGlobal(home, harnessName string) HarnessResult {
	result := HarnessResult{Name: harnessName, Installed: true}

	canonicalDir := filepath.Join(home, ".agents", "skills")
	entries, readErr := os.ReadDir(canonicalDir)
	if readErr != nil && !os.IsNotExist(readErr) {
		result.Skills = append(result.Skills, SkillEntry{
			Name:   "~/.agents/skills",
			Status: StatusBroken,
			Detail: readErr.Error(),
		})
		return result
	}
	// entries may be nil/empty if canonicalDir doesn't exist — that's fine.

	// Codex reads ~/.agents/skills/ natively — all real dirs are synced.
	if harnessName == "codex" {
		for _, entry := range entries {
			name := entry.Name()
			srcPath := filepath.Join(canonicalDir, name)
			fi, err := os.Lstat(srcPath)
			if err != nil {
				continue
			}
			isSymlink := fi.Mode()&os.ModeSymlink != 0
			if !entry.IsDir() && !isSymlink {
				continue // plain file
			}
			if isSymlink {
				result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusSkipped})
			} else {
				result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusSynced})
			}
		}
		return result
	}

	destDir := harness.GlobalSkillsDir(home, harnessName)

	// Build set of source skill names (dirs and symlinks to dirs; plain files skipped).
	type sourceInfo struct {
		isSymlink bool
	}
	sourceSkills := make(map[string]sourceInfo)
	for _, entry := range entries {
		name := entry.Name()
		srcPath := filepath.Join(canonicalDir, name)
		fi, err := os.Lstat(srcPath)
		if err != nil {
			continue
		}
		isSymlink := fi.Mode()&os.ModeSymlink != 0
		// Include real dirs and symlinks (symlinks may point to dirs)
		if !entry.IsDir() && !isSymlink {
			continue // plain file, skip
		}
		sourceSkills[name] = sourceInfo{isSymlink: isSymlink}
	}

	// Check each source skill against the harness dir.
	for name, info := range sourceSkills {
		if info.isSymlink {
			result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusSkipped})
			continue
		}

		link := filepath.Join(destDir, name)
		lfi, err := os.Lstat(link)
		if err != nil {
			if os.IsNotExist(err) {
				result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusMissing})
			} else {
				result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusBroken, Detail: err.Error()})
			}
			continue
		}

		if lfi.Mode()&os.ModeSymlink == 0 {
			// Non-symlink entry (user-managed dir) — treat as synced
			result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusSynced})
			continue
		}

		// Symlink exists — check if target resolves
		target, err := os.Readlink(link)
		if err != nil {
			result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusBroken, Detail: err.Error()})
			continue
		}

		// Resolve target relative to destDir if it's relative
		resolvedTarget := target
		if !filepath.IsAbs(target) {
			resolvedTarget = filepath.Join(destDir, target)
		}

		if _, err := os.Stat(resolvedTarget); err != nil {
			result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusBroken, Detail: target})
			continue
		}

		result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusSynced, Detail: target})
	}

	// Check for orphans: entries in harness dir with no corresponding source.
	destEntries, err := os.ReadDir(destDir)
	if err != nil && !os.IsNotExist(err) {
		return result
	}
	for _, entry := range destEntries {
		name := entry.Name()
		if _, exists := sourceSkills[name]; !exists {
			link := filepath.Join(destDir, name)
			target, _ := os.Readlink(link)
			result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusOrphan, Detail: target})
		}
	}

	return result
}
