package check

import (
	"os"
	"path/filepath"

	"github.com/kastheco/kasmos/internal/initcmd/harness"
)

// SkillStatus represents the state of a single skill entry.
type SkillStatus int

const (
	StatusSynced  SkillStatus = iota // symlink exists, valid target
	StatusSkipped                    // source is symlink, intentionally not synced
	StatusMissing                    // source exists, no link in harness
	StatusOrphan                     // link in harness, no source
	StatusBroken                     // symlink exists, target doesn't resolve
)

func (s SkillStatus) String() string {
	switch s {
	case StatusSynced:
		return "synced"
	case StatusSkipped:
		return "skipped"
	case StatusMissing:
		return "missing"
	case StatusOrphan:
		return "orphan"
	case StatusBroken:
		return "broken"
	default:
		return "unknown"
	}
}

// SkillEntry is one skill's audit result for one harness.
type SkillEntry struct {
	Name   string
	Status SkillStatus
	Detail string // e.g. symlink target, error message
}

// HarnessResult holds audit results for one harness.
type HarnessResult struct {
	Name      string
	Installed bool
	Skills    []SkillEntry
}

// ProjectSkillEntry is one embedded skill's status in the project.
type ProjectSkillEntry struct {
	Name          string
	InCanonical   bool                   // exists in .agents/skills/
	HarnessStatus map[string]SkillStatus // harness name → status
}

// AuditResult is the complete output of kas check.
type AuditResult struct {
	Global    []HarnessResult
	Project   []ProjectSkillEntry
	InProject bool // whether cwd is a kas project
}

// Audit runs all three audit layers and returns a complete result.
// home is the user's home directory; projectDir is the current working directory.
// registry provides the list of known harnesses.
func Audit(home, projectDir string, registry *harness.Registry) *AuditResult {
	result := &AuditResult{}

	harnessNames := registry.All()

	// Detect whether cwd is a kas project.
	agentsDir := filepath.Join(projectDir, ".agents")
	if _, err := os.Stat(agentsDir); err == nil {
		result.InProject = true
	}

	// Global skills audit — one HarnessResult per harness.
	for _, name := range harnessNames {
		result.Global = append(result.Global, AuditGlobal(home, name))
	}

	// Project skills audit — only when in a kas project.
	if result.InProject {
		result.Project = AuditProject(projectDir, harnessNames)
	}

	return result
}

// Summary returns (ok, total) counts across all checks.
func (r *AuditResult) Summary() (int, int) {
	ok, total := 0, 0
	for _, h := range r.Global {
		for _, s := range h.Skills {
			if s.Status == StatusSkipped {
				continue // don't count intentional skips
			}
			total++
			if s.Status == StatusSynced {
				ok++
			}
		}
	}
	for _, p := range r.Project {
		if !p.InCanonical {
			total++
			continue
		}
		for _, st := range p.HarnessStatus {
			total++
			if st == StatusSynced {
				ok++
			}
		}
	}
	return ok, total
}
