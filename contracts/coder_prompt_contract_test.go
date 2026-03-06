package contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoderPromptParallelSection(t *testing.T) {
	coderFiles := []string{
		filepath.Join("..", ".opencode", "agents", "coder.md"),
		filepath.Join("..", ".claude", "agents", "coder.md"),
		filepath.Join("..", "internal", "initcmd", "scaffold", "templates", "opencode", "agents", "coder.md"),
		filepath.Join("..", "internal", "initcmd", "scaffold", "templates", "claude", "agents", "coder.md"),
	}

	required := []string{
		"## Parallel Execution",
		"KASMOS_TASK",
		"shared worktree",
		"dirty git state",
		"kasmos-coder-lite",
	}

	for _, f := range coderFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read coder prompt %s: %v", f, err)
		}
		text := string(data)

		for _, needle := range required {
			if !strings.Contains(text, needle) {
				t.Errorf("%s missing required text: %q", f, needle)
			}
		}
	}
}
