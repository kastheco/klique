package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePermissionPrompt_OpenCodeDetectsPrompt(t *testing.T) {
	content := `
→ Read ../../../../opt

■  Chat · claude-opus-4-6

△ Permission required
  ← Access external directory /opt

Patterns

- /opt/*

 Allow once   Allow always   Reject                          ctrl+f fullscreen ⇥ select enter confirm
`
	result := ParsePermissionPrompt(content, "opencode")
	assert.NotNil(t, result)
	assert.Equal(t, "Access external directory /opt", result.Description)
	assert.Equal(t, "/opt/*", result.Pattern)
}

func TestParsePermissionPrompt_OpenCodeNoPrompt(t *testing.T) {
	content := `some normal opencode output without permission prompt`
	result := ParsePermissionPrompt(content, "opencode")
	assert.Nil(t, result)
}

func TestParsePermissionPrompt_IgnoresNonOpenCode(t *testing.T) {
	content := `△ Permission required
  ← Access external directory /opt
Patterns
- /opt/*`
	result := ParsePermissionPrompt(content, "claude")
	assert.Nil(t, result)
}

func TestParsePermissionPrompt_HandlesAnsiCodes(t *testing.T) {
	content := "\x1b[33m△\x1b[0m \x1b[1mPermission required\x1b[0m\n  ← Access external directory /tmp\n\nPatterns\n\n- /tmp/*\n"
	result := ParsePermissionPrompt(content, "opencode")
	assert.NotNil(t, result)
	assert.Equal(t, "Access external directory /tmp", result.Description)
	assert.Equal(t, "/tmp/*", result.Pattern)
}

func TestParsePermissionPrompt_MissingPattern(t *testing.T) {
	content := "△ Permission required\n  ← Access external directory /opt\n"
	result := ParsePermissionPrompt(content, "opencode")
	assert.NotNil(t, result)
	assert.Equal(t, "Access external directory /opt", result.Description)
	assert.Empty(t, result.Pattern)
}
