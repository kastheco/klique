package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldPromptPushAfterCoderExit(t *testing.T) {
	entry := planstate.PlanEntry{Status: planstate.StatusImplementing}
	inst := &session.Instance{PlanFile: "p.md", AgentType: session.AgentTypeCoder}

	if !shouldPromptPushAfterCoderExit(entry, inst, false) {
		t.Fatal("expected push prompt for exited coder")
	}
}

func TestShouldPromptPushAfterCoderExit_NoPromptForReviewer(t *testing.T) {
	entry := planstate.PlanEntry{Status: planstate.StatusImplementing}
	inst := &session.Instance{PlanFile: "p.md", AgentType: session.AgentTypeReviewer}

	if shouldPromptPushAfterCoderExit(entry, inst, false) {
		t.Fatal("did not expect push prompt for reviewer")
	}
}

func TestFullPlanLifecycle_StateTransitions(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "docs", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "plan-state.json"), []byte(`{}`), 0o644))

	ps, err := planstate.Load(plansDir)
	require.NoError(t, err)
	require.NoError(t, ps.Register(
		"2026-02-21-auth-refactor.md",
		"Refactor JWT auth",
		"plan/auth-refactor",
		time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC),
	))

	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusPlanning))
	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusImplementing))
	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusReviewing))
	require.NoError(t, ps.SetStatus("2026-02-21-auth-refactor.md", planstate.StatusFinished))

	entry, ok := ps.Entry("2026-02-21-auth-refactor.md")
	require.True(t, ok)
	assert.Equal(t, planstate.StatusFinished, entry.Status)
	assert.Equal(t, "plan/auth-refactor", entry.Branch)
}
