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
		"Always commit task files to the main branch.",
		"Do NOT create feature branches for planning work.",
		"Only register implementation plans",
		"never register design docs",
		"kas task update-content",
		".kasmos/signals/planner-finished-",
		"KASMOS_MANAGED",
		"Never modify task state directly",
		"plan review", // planner must reference the review step
	}

	for _, needle := range required {
		if !strings.Contains(text, needle) {
			t.Fatalf("planner prompt missing required policy text: %q", needle)
		}
	}

	if strings.Contains(text, "YYYY-MM-DD") {
		t.Fatalf("planner prompt still references date prefix convention YYYY-MM-DD")
	}

	if strings.Contains(text, "kasmos will detect this and register the plan") {
		t.Fatalf("planner prompt still claims the sentinel registers plan content")
	}
}
