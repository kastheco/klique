package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestDefaultAgentRoles_IncludesFixer(t *testing.T) {
	roles := DefaultAgentRoles()
	assert.Contains(t, roles, "fixer")
	assert.Contains(t, roles, "chat")
	assert.Len(t, roles, 6)
}

func TestRoleDefaults_HasAllRoles(t *testing.T) {
	defaults := RoleDefaults()
	for _, role := range DefaultAgentRoles() {
		_, ok := defaults[role]
		assert.True(t, ok, "RoleDefaults should have entry for %q", role)
	}
}
