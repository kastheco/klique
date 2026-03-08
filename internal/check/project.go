package check

import (
	"os"
	"path/filepath"
	"sort"
)

// AuditProject checks <dir>/.agents/skills/ dynamically and verifies harness
// project skill dirs have valid symlinks. Results are derived from the skills
// found in .agents/skills/ rather than a hardcoded list.
func AuditProject(dir string, harnessNames []string) []ProjectSkillEntry {
	canonicalDir := filepath.Join(dir, ".agents", "skills")

	entries, err := os.ReadDir(canonicalDir)
	if err != nil {
		// Surface the missing/unreadable canonical dir as an unhealthy entry.
		// AuditProject is only called when InProject=true (.agents/ exists), so a
		// missing or unreadable .agents/skills/ directory is itself a health issue.
		return []ProjectSkillEntry{{
			Name:          ".agents/skills",
			InCanonical:   false,
			HarnessStatus: map[string]SkillStatus{},
		}}
	}

	results := []ProjectSkillEntry{}
	for _, e := range entries {
		if !e.IsDir() {
			continue // skip plain files and symlinks
		}

		skillName := e.Name()
		skillPath := filepath.Join(canonicalDir, skillName)

		entry := ProjectSkillEntry{
			Name:          skillName,
			InCanonical:   true,
			HarnessStatus: make(map[string]SkillStatus),
		}

		// Check SKILL.md existence.
		_, statErr := os.Stat(filepath.Join(skillPath, "SKILL.md"))
		entry.HasSkillMD = statErr == nil

		// Check each harness's project skill dir for a symlink.
		for _, harnessName := range harnessNames {
			if harnessName == "codex" {
				// Codex reads .agents/skills/ natively — always synced if in canonical.
				entry.HarnessStatus[harnessName] = StatusSynced
				continue
			}

			harnessSkillsDir := filepath.Join(dir, "."+harnessName, "skills")
			link := filepath.Join(harnessSkillsDir, skillName)

			lfi, err := os.Lstat(link)
			if err != nil {
				if os.IsNotExist(err) {
					entry.HarnessStatus[harnessName] = StatusMissing
				} else {
					entry.HarnessStatus[harnessName] = StatusBroken
				}
				continue
			}

			if lfi.Mode()&os.ModeSymlink == 0 {
				// Non-symlink directory — functional but may drift from source.
				entry.HarnessStatus[harnessName] = StatusCopy
				continue
			}

			// Symlink exists — verify target resolves.
			target, err := os.Readlink(link)
			if err != nil {
				entry.HarnessStatus[harnessName] = StatusBroken
				continue
			}

			resolvedTarget := target
			if !filepath.IsAbs(target) {
				resolvedTarget = filepath.Join(harnessSkillsDir, target)
			}

			if _, err := os.Stat(resolvedTarget); err != nil {
				entry.HarnessStatus[harnessName] = StatusBroken
			} else {
				entry.HarnessStatus[harnessName] = StatusSynced
			}
		}

		results = append(results, entry)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return results
}
