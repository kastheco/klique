package taskfsm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestEvent() TransitionEvent {
	return TransitionEvent{
		PlanFile:   "my-plan",
		FromStatus: StatusReady,
		ToStatus:   StatusImplementing,
		Event:      ImplementStart,
		Timestamp:  time.Now(),
		Project:    "test-proj",
	}
}

func TestCommandHook_RunsCommand(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "out.txt")
	cmd := "touch " + outFile

	h := NewCommandHook(cmd)
	assert.Equal(t, "command", h.Name())

	err := h.Run(context.Background(), makeTestEvent())
	require.NoError(t, err)

	_, statErr := os.Stat(outFile)
	assert.NoError(t, statErr, "command should have created the output file")
}

func TestCommandHook_SetsEnvVars(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "env.txt")
	cmd := "echo $KASMOS_HOOK_EVENT > " + outFile

	h := NewCommandHook(cmd)
	err := h.Run(context.Background(), makeTestEvent())
	require.NoError(t, err)

	data, readErr := os.ReadFile(outFile)
	require.NoError(t, readErr)

	content := strings.TrimSpace(string(data))
	assert.Equal(t, string(ImplementStart), content,
		"KASMOS_HOOK_EVENT should equal the event name, got: %q", content)
}

func TestCommandHook_RespectsContextTimeout(t *testing.T) {
	h := NewCommandHook("sleep 10")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := h.Run(ctx, makeTestEvent())
	assert.Error(t, err, "should return an error when context is cancelled/timed out")
}

func TestCommandHook_EmptyCommandReturnsError(t *testing.T) {
	h := NewCommandHook("   ")
	err := h.Run(context.Background(), makeTestEvent())
	assert.Error(t, err, "empty command should return an error")
}
