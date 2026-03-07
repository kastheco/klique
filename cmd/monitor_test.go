package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMonitorCmd_HasSubcommands(t *testing.T) {
	cmd := NewMonitorCmd()
	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}
	assert.True(t, subcommands["status"], "missing 'status' subcommand")
}

func TestMonitorCmd_DefaultIsTail(t *testing.T) {
	cmd := NewMonitorCmd()
	assert.NotNil(t, cmd.RunE, "default monitor command should have RunE for live tail")
}
