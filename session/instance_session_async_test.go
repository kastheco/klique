package session

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/stretchr/testify/require"
)

type testPtyFactory struct{}

func (f *testPtyFactory) Start(_ *exec.Cmd) (*os.File, error) {
	return os.CreateTemp("", "kas-pty-*")
}

func (f *testPtyFactory) Close() {}

func TestCollectMetadata_DoesNotMutateCachedPreviewState(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(_ *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			cmdStr := cmd.String()
			switch {
			case strings.Contains(cmdStr, "capture-pane"):
				return []byte("fresh output"), nil
			case strings.Contains(cmdStr, "display-message"):
				return nil, fmt.Errorf("pane pid unavailable")
			default:
				return []byte(""), nil
			}
		},
	}

	tmuxSession := tmux.NewTmuxSessionWithDeps("async-md", "opencode", false, &testPtyFactory{}, cmdExec)
	require.NoError(t, tmuxSession.Restore())
	t.Cleanup(func() {
		if pty := tmuxSession.GetPTY(); pty != nil {
			_ = pty.Close()
		}
	})

	inst := &Instance{
		Status:           Running,
		CachedContent:    "existing",
		CachedContentSet: true,
		started:          true,
		tmuxSession:      tmuxSession,
	}

	_ = inst.CollectMetadata()

	require.Equal(t, "existing", inst.CachedContent)
	require.True(t, inst.CachedContentSet)
}
