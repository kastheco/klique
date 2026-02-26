package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootCommand_UsesSetupSubcommand(t *testing.T) {
	t.Helper()

	setupCmd, _, err := rootCmd.Find([]string{"setup"})
	require.NoError(t, err)
	require.NotNil(t, setupCmd)
	require.Equal(t, "setup", setupCmd.Name())
}
