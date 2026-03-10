package check

import (
	"os"
	"path/filepath"

	"github.com/kastheco/kasmos/internal/initcmd/harness"
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

	// Build set of source skill names (dirs and symlinks to dirs; plain files skipped).
	type sourceInfo struct {
		isSymlink  bool
		hasSkillMD bool
	}
	buildSourceSkills := func() map[string]sourceInfo {
		skills := make(map[string]sourceInfo)
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
			// Check if SKILL.md exists in the canonical (real) skill dir.
			_, mdErr := os.Stat(filepath.Join(canonicalDir, name, "SKILL.md"))
			skills[name] = sourceInfo{isSymlink: isSymlink, hasSkillMD: mdErr == nil}
		}
		return skills
	}

	// Codex reads ~/.agents/skills/ natively — all real dirs are synced.
	if harnessName == "codex" {
		sourceSkills := buildSourceSkills()
		for name, info := range sourceSkills {
			if info.isSymlink {
				result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusSkipped, HasSkillMD: info.hasSkillMD})
			} else {
				result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusSynced, HasSkillMD: info.hasSkillMD})
			}
		}
		return result
	}

	destDir := harness.GlobalSkillsDir(home, harnessName)

	sourceSkills := buildSourceSkills()

	// Check each source skill against the harness dir.
	for name, info := range sourceSkills {
		if info.isSymlink {
			result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusSkipped, HasSkillMD: info.hasSkillMD})
			continue
		}

		link := filepath.Join(destDir, name)
		lfi, err := os.Lstat(link)
		if err != nil {
			if os.IsNotExist(err) {
				result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusMissing, HasSkillMD: info.hasSkillMD})
			} else {
				result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusBroken, Detail: err.Error(), HasSkillMD: info.hasSkillMD})
			}
			continue
		}

		if lfi.Mode()&os.ModeSymlink == 0 {
			// Non-symlink entry (user-managed copy) — functional but may drift
			result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusCopy, HasSkillMD: info.hasSkillMD})
			continue
		}

		// Symlink exists — check if target resolves
		target, err := os.Readlink(link)
		if err != nil {
			result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusBroken, Detail: err.Error(), HasSkillMD: info.hasSkillMD})
			continue
		}

		// Resolve target relative to destDir if it's relative
		resolvedTarget := target
		if !filepath.IsAbs(target) {
			resolvedTarget = filepath.Join(destDir, target)
		}

		if _, err := os.Stat(resolvedTarget); err != nil {
			result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusBroken, Detail: target, HasSkillMD: info.hasSkillMD})
			continue
		}

		result.Skills = append(result.Skills, SkillEntry{Name: name, Status: StatusSynced, Detail: target, HasSkillMD: info.hasSkillMD})
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
