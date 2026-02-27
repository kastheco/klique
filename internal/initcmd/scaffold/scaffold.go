package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kastheco/kasmos/internal/initcmd/harness"
)

//go:embed templates
var templates embed.FS

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
func renderTemplate(content string, agent harness.AgentConfig) string {
	rendered := content
	rendered = strings.ReplaceAll(rendered, "{{MODEL}}", agent.Model)
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
		rendered := renderTemplate(string(content), agent)
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
	results, err := writePerRoleProject(dir, "claude", agents, selectedTools, force)
	if err != nil {
		return nil, err
	}

	// Scaffold static agent files (e.g. custodial) that are always present
	// regardless of wizard configuration.
	staticResults, err := writeStaticAgents(dir, "claude", force)
	if err != nil {
		return nil, err
	}
	return append(results, staticResults...), nil
}

// renderOpenCodeConfig reads the embedded opencode.jsonc template and substitutes
// wizard-collected values (model, temperature, effort) and dynamic paths (home dir,
// project dir). Agent blocks for roles not using the opencode harness are removed.
func renderOpenCodeConfig(dir string, agents []harness.AgentConfig) (string, error) {
	content, err := templates.ReadFile("templates/opencode/opencode.jsonc")
	if err != nil {
		return "", fmt.Errorf("read opencode.jsonc template: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	rendered := string(content)
	rendered = strings.ReplaceAll(rendered, "{{HOME_DIR}}", homeDir)
	rendered = strings.ReplaceAll(rendered, "{{PROJECT_DIR}}", dir)

	// Build lookup of agents by role. Include all harnesses so that agent
	// blocks are always written to opencode.jsonc — even when a role is
	// configured to use a different harness (e.g. claude for reviewer).
	// Kasmos controls which harness is actually used at orchestration time;
	// the opencode config just needs the block to exist.
	agentByRole := make(map[string]harness.AgentConfig)
	for _, a := range agents {
		agentByRole[a.Role] = a
	}

	// Substitute per-role placeholders for wizard-configurable agents
	for _, role := range []string{"coder", "planner", "reviewer"} {
		upper := strings.ToUpper(role)
		agent, ok := agentByRole[role]
		if !ok {
			// Remove entire agent block for this role
			rendered = removeJSONBlock(rendered, role)
			continue
		}

		rendered = strings.ReplaceAll(rendered, "{{"+upper+"_MODEL}}", normalizeOpenCodeModel(agent.Harness, agent.Model))

		// Temperature: bare number or remove line
		if agent.Temperature != nil {
			rendered = strings.ReplaceAll(rendered, "{{"+upper+"_TEMP}}", fmt.Sprintf("%g", *agent.Temperature))
		} else {
			rendered = removeLine(rendered, "{{"+upper+"_TEMP}}")
		}

		// Effort: full line or remove
		if agent.Effort != "" {
			rendered = strings.ReplaceAll(rendered, "{{"+upper+"_EFFORT_LINE}}", fmt.Sprintf(`"reasoningEffort": "%s",`, agent.Effort))
		} else {
			rendered = removeLine(rendered, "{{"+upper+"_EFFORT_LINE}}")
		}
	}

	rendered = stripTrailingCommas(rendered)

	return rendered, nil
}

func normalizeOpenCodeModel(harnessName, model string) string {
	model = strings.TrimSpace(model)
	if model == "" || strings.Contains(model, "/") {
		return model
	}

	// Claude model names are typically bare (e.g. "claude-opus-4-6") while
	// OpenCode expects provider/model format.
	if harnessName == "claude" {
		return "anthropic/" + model
	}

	return model
}

// stripTrailingCommas removes JSON trailing commas that arise when removeJSONBlock
// removes a block and the preceding entry's closing "  }," becomes the last entry
// in the object. Scans every line: if it ends with a comma and the next non-blank
// line opens with "}" or "]", the comma is stripped.
func stripTrailingCommas(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if !strings.HasSuffix(trimmed, ",") {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" {
				continue
			}
			if strings.HasPrefix(next, "}") || strings.HasPrefix(next, "]") {
				// Strip the trailing comma
				lines[i] = trimmed[:len(trimmed)-1]
			}
			break
		}
	}
	return strings.Join(lines, "\n")
}

// removeLine removes any line containing the given substring.
func removeLine(s, substr string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if !strings.Contains(line, substr) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// removeJSONBlock removes a top-level agent block like `"role": { ... }` from the
// JSONC content. Uses brace counting to find the matching closing brace.
func removeJSONBlock(s, role string) string {
	lines := strings.Split(s, "\n")
	marker := fmt.Sprintf(`"%s":`, role)

	startIdx := -1
	for i, line := range lines {
		if strings.Contains(strings.TrimSpace(line), marker) {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		return s
	}

	// Find matching closing brace via depth counting
	depth := 0
	endIdx := startIdx
	for i := startIdx; i < len(lines); i++ {
		for _, c := range lines[i] {
			if c == '{' {
				depth++
			} else if c == '}' {
				depth--
			}
		}
		if depth == 0 {
			endIdx = i
			break
		}
	}

	// Remove lines [startIdx..endIdx] inclusive
	result := make([]string, 0, len(lines)-(endIdx-startIdx+1))
	result = append(result, lines[:startIdx]...)
	result = append(result, lines[endIdx+1:]...)

	return strings.Join(result, "\n")
}

// WriteOpenCodeProject scaffolds .opencode/ project files: agent prompts and opencode.jsonc.
func WriteOpenCodeProject(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	// Scaffold agent .md files (existing behavior)
	results, err := writePerRoleProject(dir, "opencode", agents, selectedTools, force)
	if err != nil {
		return nil, err
	}

	// Scaffold static agent files (e.g. custodial) that are always present
	// regardless of wizard configuration.
	staticResults, err := writeStaticAgents(dir, "opencode", force)
	if err != nil {
		return nil, err
	}
	results = append(results, staticResults...)

	// Generate opencode.jsonc from template
	configContent, err := renderOpenCodeConfig(dir, agents)
	if err != nil {
		return nil, fmt.Errorf("render opencode.jsonc: %w", err)
	}

	configDest := filepath.Join(dir, ".opencode", "opencode.jsonc")
	written, err := writeFile(configDest, []byte(configContent), force)
	if err != nil {
		return nil, fmt.Errorf("write opencode.jsonc: %w", err)
	}
	rel, relErr := filepath.Rel(dir, configDest)
	if relErr != nil {
		rel = configDest
	}
	results = append(results, WriteResult{Path: rel, Created: written})

	return results, nil
}

// staticAgentRoles lists agent roles that are always scaffolded regardless of
// wizard configuration. These are operational/infrastructure agents that every
// project gets by default.
var staticAgentRoles = []string{"custodial"}

// writeStaticAgents writes the static agent .md files (custodial, etc.) that
// are always present in a project regardless of wizard-configured agents.
func writeStaticAgents(dir, harnessName string, force bool) ([]WriteResult, error) {
	agentDir := filepath.Join(dir, "."+harnessName, "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .%s/agents: %w", harnessName, err)
	}

	var results []WriteResult
	for _, role := range staticAgentRoles {
		content, err := templates.ReadFile(fmt.Sprintf("templates/%s/agents/%s.md", harnessName, role))
		if err != nil {
			// No template for this static role - skip silently
			continue
		}
		dest := filepath.Join(agentDir, role+".md")
		written, err := writeFile(dest, content, force)
		if err != nil {
			return nil, fmt.Errorf("write static agent %s: %w", role, err)
		}
		rel, relErr := filepath.Rel(dir, dest)
		if relErr != nil {
			rel = dest
		}
		results = append(results, WriteResult{Path: rel, Created: written})
	}
	return results, nil
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

	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .codex: %w", err)
	}

	content, err := templates.ReadFile("templates/codex/AGENTS.md")
	if err != nil {
		return nil, fmt.Errorf("read codex template: %w", err)
	}

	rendered := string(content)
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

// runtimeDirs lists directories the app expects to exist at runtime.
// Each path is relative to the project root.
var runtimeDirs = []string{
	"docs/plans",
	filepath.Join("docs", "plans", ".signals"),
	".worktrees",
}

// EnsureRuntimeDirs creates all directories the app needs at runtime.
// Idempotent — safe to call on every init.
func EnsureRuntimeDirs(dir string) ([]WriteResult, error) {
	var results []WriteResult
	for _, rel := range runtimeDirs {
		abs := filepath.Join(dir, rel)
		info, err := os.Stat(abs)
		if err == nil && info.IsDir() {
			continue // already exists
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return results, fmt.Errorf("create %s: %w", rel, err)
		}
		results = append(results, WriteResult{Path: rel + "/", Created: true})
	}
	return results, nil
}

// ScaffoldAll writes project files for all harnesses that have at least one enabled agent.
func ScaffoldAll(dir string, agents []harness.AgentConfig, selectedTools []string, force bool) ([]WriteResult, error) {
	var results []WriteResult

	// Ensure all runtime directories exist before writing any files.
	dirResults, err := EnsureRuntimeDirs(dir)
	if err != nil {
		return results, fmt.Errorf("ensure runtime dirs: %w", err)
	}
	results = append(results, dirResults...)

	// Write project skills to .agents/skills/.
	skillResults, err := WriteProjectSkills(dir, force)
	if err != nil {
		return results, fmt.Errorf("scaffold skills: %w", err)
	}
	results = append(results, skillResults...)

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

		if err := SymlinkHarnessSkills(dir, harnessName); err != nil {
			return results, fmt.Errorf("symlink %s skills: %w", harnessName, err)
		}
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
	result = strings.ReplaceAll(result, "{{PLAN_FILENAME}}", filepath.Base(planFile))
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
