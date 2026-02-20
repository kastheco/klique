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

// loadToolsReference reads the shared tools-reference template once.
// Returns empty string on error (non-fatal -- agents work without it).
func loadToolsReference() string {
	content, err := templates.ReadFile("templates/shared/tools-reference.md")
	if err != nil {
		return ""
	}
	return string(content)
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
	Created bool // true=created, false=skipped
}

// WriteClaudeProject scaffolds .claude/ project files.
func WriteClaudeProject(dir string, agents []harness.AgentConfig, force bool) error {
	toolsRef := loadToolsReference()
	agentDir := filepath.Join(dir, ".claude", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("create .claude/agents: %w", err)
	}

	for _, agent := range agents {
		if agent.Harness != "claude" {
			continue
		}
		templatePath := fmt.Sprintf("templates/claude/agents/%s.md", agent.Role)
		content, err := templates.ReadFile(templatePath)
		if err != nil {
			// No template for this role - skip
			continue
		}

		rendered := renderTemplate(string(content), agent, toolsRef)

		dest := filepath.Join(agentDir, agent.Role+".md")
		if err := writeFile(dest, []byte(rendered), force); err != nil {
			return err
		}
	}

	return nil
}

// WriteOpenCodeProject scaffolds .opencode/ project files.
func WriteOpenCodeProject(dir string, agents []harness.AgentConfig, force bool) error {
	toolsRef := loadToolsReference()
	agentDir := filepath.Join(dir, ".opencode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("create .opencode/agents: %w", err)
	}

	for _, agent := range agents {
		if agent.Harness != "opencode" {
			continue
		}
		templatePath := fmt.Sprintf("templates/opencode/agents/%s.md", agent.Role)
		content, err := templates.ReadFile(templatePath)
		if err != nil {
			continue
		}

		rendered := renderTemplate(string(content), agent, toolsRef)

		dest := filepath.Join(agentDir, agent.Role+".md")
		if err := writeFile(dest, []byte(rendered), force); err != nil {
			return err
		}
	}

	return nil
}

// WriteCodexProject scaffolds .codex/ project files.
func WriteCodexProject(dir string, agents []harness.AgentConfig, force bool) error {
	toolsRef := loadToolsReference()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return fmt.Errorf("create .codex: %w", err)
	}

	content, err := templates.ReadFile("templates/codex/AGENTS.md")
	if err != nil {
		return fmt.Errorf("read codex template: %w", err)
	}

	rendered := strings.ReplaceAll(string(content), "{{TOOLS_REFERENCE}}", toolsRef)

	dest := filepath.Join(codexDir, "AGENTS.md")
	return writeFile(dest, []byte(rendered), force)
}

// ScaffoldAll writes project files for all harnesses that have at least one enabled agent.
func ScaffoldAll(dir string, agents []harness.AgentConfig, force bool) ([]WriteResult, error) {
	var results []WriteResult

	// Group agents by harness
	byHarness := make(map[string][]harness.AgentConfig)
	for _, a := range agents {
		byHarness[a.Harness] = append(byHarness[a.Harness], a)
	}

	scaffolders := map[string]func(string, []harness.AgentConfig, bool) error{
		"claude":   WriteClaudeProject,
		"opencode": WriteOpenCodeProject,
		"codex":    WriteCodexProject,
	}

	for harnessName, harnessAgents := range byHarness {
		scaffolder, ok := scaffolders[harnessName]
		if !ok {
			continue
		}
		if err := scaffolder(dir, harnessAgents, force); err != nil {
			return results, fmt.Errorf("scaffold %s: %w", harnessName, err)
		}
	}

	// Walk the created files for summary
	prefixes := map[string]string{
		"claude":   ".claude",
		"opencode": ".opencode",
		"codex":    ".codex",
	}
	for harnessName := range byHarness {
		prefix, ok := prefixes[harnessName]
		if !ok {
			continue
		}
		target := filepath.Join(dir, prefix)
		_ = filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			results = append(results, WriteResult{Path: rel, Created: true})
			return nil
		})
	}

	return results, nil
}

// writeFile writes content to path. If force is false and the file exists, skip.
func writeFile(path string, content []byte, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil // skip existing
		}
	}
	return os.WriteFile(path, content, 0o644)
}
