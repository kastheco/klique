package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AuditSuperpowers checks superpowers installation for each harness.
// Codex is skipped (no superpowers concept). Claude is checked via `claude plugin list`.
// OpenCode is checked via filesystem: repo dir + plugin symlink.
func AuditSuperpowers(home string, harnessNames []string) []SuperpowersResult {
	var results []SuperpowersResult

	for _, name := range harnessNames {
		switch name {
		case "codex":
			// codex has no superpowers concept — skip entirely
			continue
		case "claude":
			results = append(results, auditClaudeSuperpowers())
		case "opencode":
			results = append(results, auditOpenCodeSuperpowers(home))
		}
	}

	return results
}

// auditClaudeSuperpowers checks if the superpowers plugin is installed in Claude Code.
func auditClaudeSuperpowers() SuperpowersResult {
	r := SuperpowersResult{Name: "claude"}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		r.Installed = false
		r.Detail = "claude not found in PATH"
		return r
	}
	_ = claudePath

	out, err := exec.Command("claude", "plugin", "list").Output()
	if err != nil {
		r.Installed = false
		r.Detail = "claude plugin list failed: " + err.Error()
		return r
	}

	if strings.Contains(string(out), "superpowers") {
		r.Installed = true
		r.Detail = "plugin installed"
	} else {
		r.Installed = false
		r.Detail = "superpowers plugin not found in claude plugin list"
	}

	return r
}

// auditOpenCodeSuperpowers checks if superpowers is installed for OpenCode:
// - ~/.config/opencode/superpowers/.git must exist (repo cloned)
// - ~/.config/opencode/plugins/superpowers.js must be a valid symlink
func auditOpenCodeSuperpowers(home string) SuperpowersResult {
	r := SuperpowersResult{Name: "opencode"}

	repoDir := filepath.Join(home, ".config", "opencode", "superpowers")
	gitDir := filepath.Join(repoDir, ".git")

	if _, err := os.Stat(gitDir); err != nil {
		r.Installed = false
		r.Detail = "repo not cloned (~/.config/opencode/superpowers/.git missing)"
		return r
	}

	pluginLink := filepath.Join(home, ".config", "opencode", "plugins", "superpowers.js")
	lfi, err := os.Lstat(pluginLink)
	if err != nil {
		r.Installed = false
		r.Detail = "plugin symlink missing (~/.config/opencode/plugins/superpowers.js)"
		return r
	}

	if lfi.Mode()&os.ModeSymlink == 0 {
		// Non-symlink file — treat as installed (user-managed)
		r.Installed = true
		r.Detail = "plugin file present (user-managed)"
		return r
	}

	// Verify symlink resolves
	target, err := os.Readlink(pluginLink)
	if err != nil {
		r.Installed = false
		r.Detail = "plugin symlink unreadable: " + err.Error()
		return r
	}

	resolvedTarget := target
	if !filepath.IsAbs(target) {
		resolvedTarget = filepath.Join(filepath.Dir(pluginLink), target)
	}

	if _, err := os.Stat(resolvedTarget); err != nil {
		r.Installed = false
		r.Detail = "plugin symlink broken (target missing)"
		return r
	}

	r.Installed = true
	r.Detail = "repo cloned, plugin symlinked"
	return r
}
