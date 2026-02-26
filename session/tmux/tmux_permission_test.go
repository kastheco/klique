package tmux

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendPermissionResponse_AllowOnce verifies that PermissionAllowOnce sends a
// single Enter keystroke — "Allow once" is the first (default-selected) option.
func TestSendPermissionResponse_AllowOnce(t *testing.T) {
	var ranCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ranCmds = append(ranCmds, strings.Join(cmd.Args, " "))
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	session := newTmuxSession("test", "opencode", false, &MockPtyFactory{t: t}, cmdExec)

	err := session.SendPermissionResponse(PermissionAllowOnce)
	require.NoError(t, err)

	// Should send exactly one Enter.
	require.Len(t, ranCmds, 1)
	assert.Equal(t, "tmux send-keys -t kas_test Enter", ranCmds[0])
}

// TestSendPermissionResponse_AllowAlways verifies that PermissionAllowAlways sends
// Right → Enter → Enter — navigating to "Allow always" (second option) and confirming.
func TestSendPermissionResponse_AllowAlways(t *testing.T) {
	var ranCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ranCmds = append(ranCmds, strings.Join(cmd.Args, " "))
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	session := newTmuxSession("test", "opencode", false, &MockPtyFactory{t: t}, cmdExec)

	err := session.SendPermissionResponse(PermissionAllowAlways)
	require.NoError(t, err)

	// Should send Right, Enter, Enter — three separate send-keys calls.
	require.Len(t, ranCmds, 3, "AllowAlways should send 3 key events: Right, Enter, Enter")
	assert.Equal(t, "tmux send-keys -t kas_test Right", ranCmds[0])
	assert.Equal(t, "tmux send-keys -t kas_test Enter", ranCmds[1])
	assert.Equal(t, "tmux send-keys -t kas_test Enter", ranCmds[2])
}

// TestSendPermissionResponse_Reject verifies that PermissionReject sends
// Right → Right → Enter — navigating to "Reject" (third option) and confirming.
func TestSendPermissionResponse_Reject(t *testing.T) {
	var ranCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			ranCmds = append(ranCmds, strings.Join(cmd.Args, " "))
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	session := newTmuxSession("test", "opencode", false, &MockPtyFactory{t: t}, cmdExec)

	err := session.SendPermissionResponse(PermissionReject)
	require.NoError(t, err)

	// Should send Right, Right, Enter — three separate send-keys calls.
	require.Len(t, ranCmds, 3, "Reject should send 3 key events: Right, Right, Enter")
	assert.Equal(t, "tmux send-keys -t kas_test Right", ranCmds[0])
	assert.Equal(t, "tmux send-keys -t kas_test Right", ranCmds[1])
	assert.Equal(t, "tmux send-keys -t kas_test Enter", ranCmds[2])
}
