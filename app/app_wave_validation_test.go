package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatePlanContent_WithWaves(t *testing.T) {
	content := `# Plan

**Goal:** Test

## Wave 1
### Task 1: Something

Do it.
`
	err := validatePlanContent(content)
	assert.NoError(t, err)
}

func TestValidatePlanContent_NoWaves(t *testing.T) {
	content := `# Plan

**Goal:** Test

### Task 1: Something

Do it.
`
	err := validatePlanContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no wave headers")
}
