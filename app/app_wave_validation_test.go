package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePlanHasWaves_WithWaves(t *testing.T) {
	dir := t.TempDir()
	planFile := "test-plan.md"
	content := `# Plan

**Goal:** Test

## Wave 1
### Task 1: Something

Do it.
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, planFile), []byte(content), 0o644))

	err := validatePlanHasWaves(dir, planFile)
	assert.NoError(t, err)
}

func TestValidatePlanHasWaves_NoWaves(t *testing.T) {
	dir := t.TempDir()
	planFile := "test-plan.md"
	content := `# Plan

**Goal:** Test

### Task 1: Something

Do it.
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, planFile), []byte(content), 0o644))

	err := validatePlanHasWaves(dir, planFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no wave headers")
}
