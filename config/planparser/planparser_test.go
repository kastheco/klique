package planparser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePlan_MultiWave(t *testing.T) {
	input := `# Feature Plan

> **For Claude:** ...

**Goal:** Build a thing
**Architecture:** Some approach
**Tech Stack:** Go

**Waves:** 2 (T1,T2 parallel → T3 sequential)

---

## Wave 1
### Task 1: First Thing

**Files:**
- Create: ` + "`path/to/file.go`" + `

**Step 1: Do something**

Some instructions here.

### Task 2: Second Thing

**Files:**
- Modify: ` + "`other/file.go`" + `

**Step 1: Do other thing**

More instructions.

## Wave 2
### Task 3: Final Thing

**Files:**
- Modify: ` + "`path/to/file.go`" + `

**Step 1: Wrap up**

Final instructions.
`
	plan, err := Parse(input)
	require.NoError(t, err)

	assert.Equal(t, "Build a thing", plan.Goal)
	assert.Equal(t, "Some approach", plan.Architecture)
	assert.Equal(t, "Go", plan.TechStack)

	require.Len(t, plan.Waves, 2)

	// Wave 1: two tasks
	require.Len(t, plan.Waves[0].Tasks, 2)
	assert.Equal(t, 1, plan.Waves[0].Number)
	assert.Equal(t, 1, plan.Waves[0].Tasks[0].Number)
	assert.Equal(t, "First Thing", plan.Waves[0].Tasks[0].Title)
	assert.Contains(t, plan.Waves[0].Tasks[0].Body, "Do something")
	assert.Equal(t, 2, plan.Waves[0].Tasks[1].Number)
	assert.Equal(t, "Second Thing", plan.Waves[0].Tasks[1].Title)

	// Wave 2: one task
	require.Len(t, plan.Waves[1].Tasks, 1)
	assert.Equal(t, 2, plan.Waves[1].Number)
	assert.Equal(t, 3, plan.Waves[1].Tasks[0].Number)
}

func TestParsePlan_NoWaveHeaders(t *testing.T) {
	input := `# Old Plan

**Goal:** Legacy thing

---

### Task 1: Something
Step 1: do it

### Task 2: Another
Step 1: do it too
`
	_, err := Parse(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no wave headers found")
}

func TestParsePlan_EmptyPlan(t *testing.T) {
	_, err := Parse("")
	require.Error(t, err)
}

func TestParsePlan_HeaderExtraction(t *testing.T) {
	input := `# Plan

**Goal:** My goal here
**Architecture:** My arch here
**Tech Stack:** Go, bubbletea

## Wave 1
### Task 1: Only Task

Do the thing.
`
	plan, err := Parse(input)
	require.NoError(t, err)
	assert.Equal(t, "My goal here", plan.Goal)
	assert.Equal(t, "My arch here", plan.Architecture)
	assert.Equal(t, "Go, bubbletea", plan.TechStack)
}
