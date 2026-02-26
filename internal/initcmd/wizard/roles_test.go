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
}

func TestRolePhaseText(t *testing.T) {
	text := RolePhaseText("coder")
	assert.Contains(t, text, "implementing")
}
