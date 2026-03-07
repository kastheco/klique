package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoleDescription(t *testing.T) {
	desc := RoleDescription("coder")
	assert.Contains(t, desc, "implementation")

	desc = RoleDescription("unknown")
	assert.Equal(t, "", desc)

	desc = RoleDescription("fixer")
	assert.Contains(t, desc, "debug")

	desc = RoleDescription("chat")
	assert.Contains(t, desc, "assistant")
}

func TestRolePhaseText(t *testing.T) {
	text := RolePhaseText("coder")
	assert.Contains(t, text, "implementing")

	text = RolePhaseText("fixer")
	assert.Contains(t, text, "fixer")
}

func TestDefaultAgentRoles_IncludesMaster(t *testing.T) {
	roles := DefaultAgentRoles()
	assert.Contains(t, roles, "master")
}

func TestRoleDefaults_HasMaster(t *testing.T) {
	defaults := RoleDefaults()
	master, ok := defaults["master"]
	require.True(t, ok)
	assert.True(t, master.Enabled)
	assert.NotEmpty(t, master.Model)
}

func TestRoleDescription_Master(t *testing.T) {
	desc := RoleDescription("master")
	assert.NotEmpty(t, desc)
	assert.Contains(t, desc, "final")
}

func TestRolePhaseText_Master(t *testing.T) {
	phase := RolePhaseText("master")
	assert.NotEmpty(t, phase)
	assert.Contains(t, phase, "master_review")
}

func TestDefaultAgentRoles_IncludesFixer(t *testing.T) {
	roles := DefaultAgentRoles()
	assert.Contains(t, roles, "fixer")
	assert.Contains(t, roles, "chat")
	assert.Len(t, roles, 7)
}

func TestRoleDefaults_HasAllRoles(t *testing.T) {
	defaults := RoleDefaults()
	for _, role := range DefaultAgentRoles() {
		_, ok := defaults[role]
		assert.True(t, ok, "RoleDefaults should have entry for %q", role)
	}
}
