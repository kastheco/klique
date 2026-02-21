package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kastheco/klique/internal/initcmd/harness"
)

//go:embed templates
var templates embed.FS

// loadFilteredToolsReference reads the shared tools-reference template and filters
// it to include only the selected tools. Returns empty string when no tools are
// selected or on error (non-fatal -- agents work without it, but warns).
func loadFilteredToolsReference(selectedTools []string) string {
	content, err := templates.ReadFile("templates/shared/tools-reference.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: tools-reference template missing from binary: %v\n", err)
		return ""
	}
	if len(selectedTools) == 0 {
		return ""
	}
	return FilterToolsReference(string(content), selectedTools)
}

// validateRole ensures a role name is safe for use in filesystem paths.
// Rejects empty strings and any character outside [a-zA-Z0-9_-].
func validateRole(role string) error {
	if role == "" {
		return fmt.Errorf("agent role must not be empty")
	}
	for _, c := range role {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-') {
			return fmt.Errorf("invalid agent role %q: must contain only letters, digits, hyphens, or underscores", role)
		}
	}
	return nil
}

// renderTemplate applies all placeholder substitutions to a template.
func renderTemplate(content string, agent harness.AgentConfig, toolsRef string) string {
	rendered := content
	rendered = strings.ReplaceAll(rendered, "{{MODEL}}", agent.Model)
	rendered = strings.ReplaceAll(rendered, "{{TOOLS_REFERENCE}}", toolsRef)
	return rendered
}

// WriteResult tracks scaffold output for summary display.
type WriteResult struct {
	Path    string
	Created bool // true=written, false=skipped (file already existed)
}

// writePerRoleProject is the shared implementation for per-role harnesses (claude, opencode).
// It scaffolds one .md file per agent role using templates at templates/<harnessName>/agents/<role>.md.
func writePerRoleProject(dir, harnessName string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	toolsRef := loadFilteredToolsReference(selectedTools)
	agentDir := filepath.Join(dir, "."+harnessName, "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .%s/agents: %w", harnessName, err)
	}

	var results []WriteResult
	for _, agent := range agents {
		if agent.Harness != harnessName {
			continue
		}
		if err := validateRole(agent.Role); err != nil {
			return nil, err
		}
		content, err := templates.ReadFile(fmt.Sprintf("templates/%s/agents/%s.md", harnessName, agent.Role))
		if err != nil {
			// No template for this role - skip
			continue
		}
		rendered := renderTemplate(string(content), agent, toolsRef)
		dest := filepath.Join(agentDir, agent.Role+".md")
		written, err := writeFile(dest, []byte(rendered), force)
		if err != nil {
			return nil, err
		}
		rel, relErr := filepath.Rel(dir, dest)
		if relErr != nil {
			rel = dest
		}
		results = append(results, WriteResult{Path: rel, Created: written})
	}
	return results, nil
}

// WriteClaudeProject scaffolds .claude/ project files.
func WriteClaudeProject(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	return writePerRoleProject(dir, "claude", agents, selectedTools, force)
}

// WriteOpenCodeProject scaffolds .opencode/ project files.
func WriteOpenCodeProject(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	return writePerRoleProject(dir, "opencode", agents, selectedTools, force)
}

// WriteCodexProject scaffolds .codex/ project files.
func WriteCodexProject(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	for _, agent := range agents {
		if agent.Harness != "codex" {
			continue
		}
		if err := validateRole(agent.Role); err != nil {
			return nil, err
		}
	}

	toolsRef := loadFilteredToolsReference(selectedTools)
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .codex: %w", err)
	}

	content, err := templates.ReadFile("templates/codex/AGENTS.md")
	if err != nil {
		return nil, fmt.Errorf("read codex template: %w", err)
	}

	rendered := strings.ReplaceAll(string(content), "{{TOOLS_REFERENCE}}", toolsRef)
	dest := filepath.Join(codexDir, "AGENTS.md")
	written, err := writeFile(dest, []byte(rendered), force)
	if err != nil {
		return nil, err
	}
	rel, relErr := filepath.Rel(dir, dest)
	if relErr != nil {
		rel = dest
	}
	return []WriteResult{{Path: rel, Created: written}}, nil
}

// WriteProjectSkills writes embedded skill trees to <dir>/.agents/skills/.
// Each skill is a directory containing SKILL.md and reference/script files.
func WriteProjectSkills(dir string, force bool) ([]WriteResult, error) {
	const prefix = "templates/skills"
	var results []WriteResult

	err := fs.WalkDir(templates, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel := strings.TrimPrefix(path, prefix+"/")
		dest := filepath.Join(dir, ".agents", "skills", rel)

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("create skill dir: %w", err)
		}

		content, err := templates.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded skill %s: %w", path, err)
		}

		written, err := writeFile(dest, content, force)
		if err != nil {
			return fmt.Errorf("write skill %s: %w", rel, err)
		}

		relResult, relErr := filepath.Rel(dir, dest)
		if relErr != nil {
			relResult = dest
		}
		results = append(results, WriteResult{Path: relResult, Created: written})
		return nil
	})

	return results, err
}

// SymlinkHarnessSkills creates symlinks from .<harnessName>/skills/<skill>
// to ../../.agents/skills/<skill> for each skill in .agents/skills/.
// Replaces existing symlinks. Skips non-symlink entries (user-managed dirs).
func SymlinkHarnessSkills(dir, harnessName string) error {
	srcDir := filepath.Join(dir, ".agents", "skills")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read skills dir: %w", err)
	}

	destDir := filepath.Join(dir, "."+harnessName, "skills")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create %s skills dir: %w", harnessName, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		link := filepath.Join(destDir, name)
		target := filepath.Join("..", "..", ".agents", "skills", name)

		if fi, err := os.Lstat(link); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				if err := os.Remove(link); err != nil {
					return fmt.Errorf("remove existing symlink %s: %w", name, err)
				}
			} else {
				continue
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s link: %w", name, err)
		}

		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("symlink %s skill %s: %w", harnessName, name, err)
		}
	}

	return nil
}

// ScaffoldAll writes project files for all harnesses that have at least one enabled agent.
func ScaffoldAll(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	var results []WriteResult

	// Group agents by harness
	byHarness := make(map[string][]harness.AgentConfig)
	for _, a := range agents {
		byHarness[a.Harness] = append(byHarness[a.Harness], a)
	}

	type scaffoldFn func(string, []harness.AgentConfig, []string, bool) ([]WriteResult, error)
	scaffolders := map[string]scaffoldFn{
		"claude":   WriteClaudeProject,
		"opencode": WriteOpenCodeProject,
		"codex":    WriteCodexProject,
	}

	// Iterate in stable order so results are deterministic across runs.
	for _, harnessName := range []string{"claude", "opencode", "codex"} {
		harnessAgents, ok := byHarness[harnessName]
		if !ok {
			continue
		}
		harnessResults, err := scaffolders[harnessName](dir, harnessAgents, selectedTools, force)
		if err != nil {
			return results, fmt.Errorf("scaffold %s: %w", harnessName, err)
		}
		results = append(results, harnessResults...)
	}

	return results, nil
}

// LoadReviewPrompt reads the embedded review prompt template and fills in the plan placeholders.
// Falls back to a minimal inline prompt if the template is missing from the binary.
func LoadReviewPrompt(planFile, planName string) string {
	content, err := templates.ReadFile("templates/shared/review-prompt.md")
	if err != nil {
		return fmt.Sprintf("Review the implementation of plan: %s\nPlan file: %s", planName, planFile)
	}
	result := strings.ReplaceAll(string(content), "{{PLAN_FILE}}", planFile)
	result = strings.ReplaceAll(result, "{{PLAN_NAME}}", planName)
	return result
}

// writeFile writes content to path. If force is false and the file exists, skip.
// Returns true if the file was actually written, false if skipped.
func writeFile(path string, content []byte, force bool) (bool, error) {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil // skip existing
		}
	}
	return true, os.WriteFile(path, content, 0o644)
}
