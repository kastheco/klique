package headless

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeadlessSession_InstallAndCapture(t *testing.T) {
	s := NewSession("capture", "sh -c \"printf 'No, and tell Claude what to do differently'; sleep 0.2\"", false)

	require.NoError(t, s.Start(t.TempDir()))
	require.Eventually(t, func() bool {
		content, err := s.CapturePaneContent()
		return err == nil && content != ""
	}, time.Second, 25*time.Millisecond)

	content, err := s.CapturePaneContent()
	require.NoError(t, err)
	assert.Contains(t, content, "No, and tell Claude what to do differently")
	pid, err := s.GetPanePID()
	require.NoError(t, err)
	assert.Positive(t, pid)

	updated, hasPrompt, _, captured := s.HasUpdatedWithContent()
	assert.True(t, updated)
	assert.True(t, hasPrompt)
	assert.True(t, captured)

	require.NoError(t, s.Close())
}

func TestHeadlessSession_RestoreRejectsStoppedSession(t *testing.T) {
	s := NewSession("restore", "printf 'ready'", false)
	err := s.Restore()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "headless session not running")
}

func TestHeadlessSession_InteractiveOperationsUnsupported(t *testing.T) {
	s := NewSession("interactive", "printf 'done'", false)
	require.ErrorIs(t, s.SendKeys("x"), ErrInteractiveOnly)
	require.ErrorIs(t, s.TapEnter(), ErrInteractiveOnly)
	require.ErrorIs(t, s.DetachSafely(), ErrInteractiveOnly)
	_, err := s.Attach()
	require.ErrorIs(t, err, ErrInteractiveOnly)
	require.ErrorIs(t, s.SetDetachedSize(80, 24), ErrInteractiveOnly)
	assert.Nil(t, s.GetPTY())
}

func TestHeadlessSession_CommandUsesTaskContext(t *testing.T) {
	s := NewSession("env", "sh -c 'env; sleep 0.2'", false)
	s.SetTaskEnv(3, 4, 2)

	require.NoError(t, s.Start(t.TempDir()))
	require.Eventually(t, func() bool {
		content, err := s.CapturePaneContent()
		if err != nil {
			return false
		}
		return content != ""
	}, time.Second, 25*time.Millisecond)

	content, err := s.CapturePaneContent()
	require.NoError(t, err)
	assert.Contains(t, content, "KASMOS_TASK=3")
	assert.Contains(t, content, "KASMOS_WAVE=4")
	assert.Contains(t, content, "KASMOS_PEERS=2")

	pid, err := s.GetPanePID()
	require.NoError(t, err)
	assert.Positive(t, pid)
	require.NoError(t, s.Close())
}

func TestHeadlessSession_InitialPromptIsShellEscaped(t *testing.T) {
	prompt := "it's\ndone"
	s := NewSession("prompt", "printf", false)
	s.SetInitialPrompt(prompt)

	require.NoError(t, s.Start(t.TempDir()))
	require.Eventually(t, func() bool {
		content, err := s.CapturePaneContent()
		return err == nil && content != ""
	}, time.Second, 25*time.Millisecond)

	content, err := s.CapturePaneContent()
	require.NoError(t, err)
	assert.Contains(t, content, prompt)
	require.NoError(t, s.Close())
}
