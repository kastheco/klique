package check

import (
	"os"
	"path/filepath"
)

// EmbeddedSkillNames is the list of skill names that kq init writes to .agents/skills/.
var EmbeddedSkillNames = []string{"cli-tools", "golang-pro", "tmux-orchestration", "tui-design"}

// AuditProject checks <dir>/.agents/skills/ against expected embedded skills
// and verifies harness project skill dirs have valid symlinks.
func AuditProject(dir string, harnessNames []string) []ProjectSkillEntry {
	canonicalDir := filepath.Join(dir, ".agents", "skills")

	// Determine which embedded skills exist in canonical dir.
	canonicalSet := make(map[string]bool)
	entries, err := os.ReadDir(canonicalDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				canonicalSet[e.Name()] = true
			}
		}
	}

	var results []ProjectSkillEntry
	for _, skillName := range EmbeddedSkillNames {
		entry := ProjectSkillEntry{
			Name:          skillName,
			InCanonical:   canonicalSet[skillName],
			HarnessStatus: make(map[string]SkillStatus),
		}

		if !entry.InCanonical {
			results = append(results, entry)
			continue
		}

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
				// Non-symlink (user-managed) — treat as synced.
				entry.HarnessStatus[harnessName] = StatusSynced
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

	return results
}
