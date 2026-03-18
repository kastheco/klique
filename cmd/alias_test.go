package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestSubcommandAliases(t *testing.T) {
	tests := []struct {
		name      string
		buildCmd  func() *cobra.Command
		wantAlias string
	}{
		{"task", NewTaskCmd, "t"},
		{"instance", NewInstanceCmd, "i"},
		{"tmux", NewTmuxCmd, "tx"},
		{"daemon", NewDaemonCmd, "d"},
		{"monitor", NewMonitorCmd, "mon"},
		{"signal", NewSignalCmd, "sig"},
		{"audit", NewAuditCmd, "au"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.buildCmd()
			assert.Contains(t, cmd.Aliases, tc.wantAlias,
				"%s command should have alias %q", tc.name, tc.wantAlias)
		})
	}
}
