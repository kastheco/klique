package contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlannerPromptBranchPolicy(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".opencode", "agents", "planner.md"))
	if err != nil {
		t.Fatalf("read planner prompt: %v", err)
	}
	text := string(data)

	required := []string{
		"Always commit plan files to the main branch.",
		"Do NOT create feature branches for planning work.",
		"Only register implementation plans in plan-state.json",
		"never register design docs",
	}

	for _, needle := range required {
		if !strings.Contains(text, needle) {
			t.Fatalf("planner prompt missing required policy text: %q", needle)
		}
	}
}
