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

func TestCoderPromptMinimal(t *testing.T) {
	coderFiles := []string{
		filepath.Join("..", ".opencode", "agents", "coder.md"),
		filepath.Join("..", ".claude", "agents", "coder.md"),
		filepath.Join("..", "internal", "initcmd", "scaffold", "templates", "opencode", "agents", "coder.md"),
		filepath.Join("..", "internal", "initcmd", "scaffold", "templates", "claude", "agents", "coder.md"),
	}

	forbidden := []string{
		"kasmos-coder",
		"cli-tools",
		"Load the",
	}

	required := []string{
		"KASMOS_TASK",
		"commit",
		"## Parallel Execution",
	}

	for _, f := range coderFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read coder prompt %s: %v", f, err)
		}
		text := string(data)

		for _, needle := range forbidden {
			if strings.Contains(text, needle) {
				t.Errorf("%s must not contain: %q", f, needle)
			}
		}

		for _, needle := range required {
			if !strings.Contains(text, needle) {
				t.Errorf("%s missing required text: %q", f, needle)
			}
		}
	}
}
