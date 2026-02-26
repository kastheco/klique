package keys

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGlobalKeyStringsMap_ViewPlanHasPAlias(t *testing.T) {
	if got, ok := GlobalKeyStringsMap["p"]; !ok || got != KeyViewPlan {
		t.Fatalf("GlobalKeyStringsMap[\"p\"] = (%v, %v), want (%v, true)", got, ok, KeyViewPlan)
	}
}

func TestSpawnAgentKeyInGlobalMap(t *testing.T) {
	name, ok := GlobalKeyStringsMap["s"]
	assert.True(t, ok, "'s' must be in GlobalKeyStringsMap")
	assert.Equal(t, KeySpawnAgent, name)
}

func TestFocusSidebarRemoved(t *testing.T) {
	_, ok := GlobalKeyStringsMap["s"]
	// Should map to KeySpawnAgent, not any legacy sidebar-focus key.
	assert.True(t, ok)
	assert.Equal(t, KeySpawnAgent, GlobalKeyStringsMap["s"])
}

func TestGlobalKeyBindings_UpdatedStatusLineLabels(t *testing.T) {
	if got := GlobalkeyBindings[KeyEnter].Help().Desc; got != "select" {
		t.Fatalf("KeyEnter help desc = %q, want %q", got, "select")
	}
	if got := GlobalkeyBindings[KeySpaceExpand].Help().Desc; got != "toggle" {
		t.Fatalf("KeySpaceExpand help desc = %q, want %q", got, "toggle")
	}
	if got := GlobalkeyBindings[KeyViewPlan].Help().Desc; got != "preview" {
		t.Fatalf("KeyViewPlan help desc = %q, want %q", got, "preview")
	}
}
