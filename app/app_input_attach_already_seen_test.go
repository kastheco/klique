package app

import (
	"context"
	"os/exec"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"github.com/stretchr/testify/require"
	"os"
)

// mockAppState is a minimal in-test implementation of config.AppState.
type mockAppState struct {
	seen uint32
}

func (s *mockAppState) GetHelpScreensSeen() uint32        { return s.seen }
func (s *mockAppState) SetHelpScreensSeen(v uint32) error { s.seen = v; return nil }

// noopPtyFactory satisfies tmux.PtyFactory without spawning a real PTY.
type noopPtyFactory struct{}

func (f *noopPtyFactory) Start(_ *exec.Cmd) (*os.File, error) { return nil, nil }
func (f *noopPtyFactory) Close()                              {}

// newStartedInstance returns an instance that passes the Started/Paused/TmuxAlive
// guards in app_input.go, using a mock cmdExec so DoesSessionExist returns true.
func newStartedInstanceWithMockTmux(t *testing.T) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "test-attach-seen",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	inst.MarkStartedForTest()

	// Inject a mock TmuxSession whose DoesSessionExist() always returns true (nil exit).
	mockExec := cmd_test.MockCmdExec{
		RunFunc:    func(_ *exec.Cmd) error { return nil },
		OutputFunc: func(_ *exec.Cmd) ([]byte, error) { return nil, nil },
	}
	sess := tmux.NewTmuxSessionWithDeps("test-attach-seen", "claude", false, &noopPtyFactory{}, mockExec)
	inst.SetTmuxSession(sess)
	return inst
}

// TestEnterKey_AttachHelpAlreadySeen_ExecsDirectly verifies that pressing Enter on a
// running instance when the attach help overlay has already been acknowledged does
// NOT silently do nothing. Instead, the fix should detect that showHelpScreen was
// a no-op (m.state != stateHelp) and immediately return tea.Exec for the attach.
//
// Regression for: second+ attach via Enter silently failing because pendingAttachInstance
// was set but never consumed when showHelpScreen skipped the overlay.
func TestEnterKey_AttachHelpAlreadySeen_ExecsDirectly(t *testing.T) {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))

	// helpTypeInstanceAttach.mask() == 1 << 2 == 4; pre-seed it as "already seen".
	state := &mockAppState{seen: 1 << 2}

	h := &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		nav:          ui.NewNavigationPanel(&spin),
		menu:         ui.NewMenu(),
		toastManager: overlay.NewToastManager(&spin),
		overlays:     overlay.NewManager(),
		appState:     state,
	}

	inst := newStartedInstanceWithMockTmux(t)
	h.nav.AddInstance(inst)()
	h.nav.SelectInstance(inst)

	// keySent = true bypasses menu-highlight delay (see handleMenuHighlighting).
	h.keySent = true
	_, cmd := h.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Before the fix: cmd is nil (pendingAttachInstance silently abandoned).
	// After the fix: cmd is non-nil tea.Exec that runs the attach.
	require.NotNil(t, cmd, "Enter on running instance with help-overlay already seen must return tea.Exec, not nil")

	// pendingAttachInstance must be cleared — it was consumed by the direct Exec path.
	require.Nil(t, h.pendingAttachInstance, "pendingAttachInstance must be cleared after direct Exec path")
}
