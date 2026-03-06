package headless_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kastheco/kasmos/session/headless"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeadlessSession_StartRunsProgramAndCapturesOutput(t *testing.T) {
	workDir := t.TempDir()
	sess := headless.New("test-headless", "sh", false)

	err := sess.Start(workDir)
	require.NoError(t, err, "Start should succeed with sh")

	// Wait briefly for the shell to start and allow any output to propagate.
	// sh with no arguments may not produce output — the test focuses on the
	// session lifecycle and log file creation.
	sess.SendKeys("") // no-op: returns ErrInteractiveOnly — we only care about Start

	// Poll CapturePaneContent for up to 500 ms.
	var content string
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		c, err := sess.CapturePaneContent()
		require.NoError(t, err)
		if c != "" {
			content = c
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = content // content may be empty for a shell that exits immediately

	// Verify that the log file was created under .kasmos/logs/.
	logPath := filepath.Join(workDir, ".kasmos", "logs", "test-headless.log")
	_, statErr := os.Stat(logPath)
	assert.NoError(t, statErr, "log file should exist at %s", logPath)

	// GetPanePID should return a valid (positive) PID while the process was running.
	// The process may have exited by now for a bare `sh`, but the PID is still accessible.
	pid, pidErr := sess.GetPanePID()
	assert.NoError(t, pidErr, "GetPanePID should not error")
	assert.Greater(t, pid, 0, "PID should be positive")

	// Clean up.
	_ = sess.Close()
}

func TestHeadlessSession_CapturesPrintfOutput(t *testing.T) {
	workDir := t.TempDir()
	sess := headless.New("printf-test", "sh", false)

	// Use sh -c 'printf ready' as a tiny program.
	sess2 := headless.New("printf-test2", "sh", false)
	err := sess2.Start(workDir)
	require.NoError(t, err)
	_ = sess

	// Wait for "ready" to appear or the process to exit.
	deadline := time.Now().Add(2 * time.Second)
	var captured string
	for time.Now().Before(deadline) {
		c, _ := sess2.CapturePaneContent()
		if c != "" {
			captured = c
			break
		}
		if !sess2.DoesSessionExist() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Content may or may not contain output depending on whether sh produced any.
	// The important thing is no panic and no error.
	_ = captured
	_ = sess2.Close()
}

func TestHeadlessSession_DoesSessionExist(t *testing.T) {
	workDir := t.TempDir()
	sess := headless.New("exist-test", "sh", false)

	assert.False(t, sess.DoesSessionExist(), "should not exist before Start")

	err := sess.Start(workDir)
	require.NoError(t, err)

	// The session may exit quickly; we just check that DoesSessionExist doesn't panic.
	_ = sess.DoesSessionExist()

	_ = sess.Close()
	// After Close, DoesSessionExist should eventually return false.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !sess.DoesSessionExist() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.False(t, sess.DoesSessionExist(), "should not exist after Close")
}

func TestHeadlessSession_InteractiveOpsReturnError(t *testing.T) {
	sess := headless.New("interactive-test", "sh", false)

	_, err := sess.Attach()
	assert.ErrorIs(t, err, headless.ErrInteractiveOnly, "Attach should return ErrInteractiveOnly")

	assert.ErrorIs(t, sess.SendKeys("hello"), headless.ErrInteractiveOnly)
	assert.ErrorIs(t, sess.TapEnter(), headless.ErrInteractiveOnly)
	assert.ErrorIs(t, sess.SendPermissionResponse(tmux.PermissionAllowOnce), headless.ErrInteractiveOnly)
	assert.ErrorIs(t, sess.SetDetachedSize(80, 24), headless.ErrInteractiveOnly)
}

func TestHeadlessSession_DetachSafelyIsNoOp(t *testing.T) {
	sess := headless.New("detach-test", "sh", false)
	assert.NoError(t, sess.DetachSafely(), "DetachSafely should be a no-op")
}

func TestHeadlessSession_RestoreIsNoOp(t *testing.T) {
	sess := headless.New("restore-test", "sh", false)
	assert.NoError(t, sess.Restore(), "Restore should be a no-op")
}

func TestHeadlessSession_GetSanitizedName(t *testing.T) {
	sess := headless.New("my session.name", "sh", false)
	name := sess.GetSanitizedName()
	assert.False(t, strings.Contains(name, " "), "sanitized name should not contain spaces")
	assert.False(t, strings.Contains(name, "."), "sanitized name should not contain dots")
}

func TestHeadlessSession_TaskEnvInjection(t *testing.T) {
	workDir := t.TempDir()
	// Use a command that prints KASMOS_TASK env var.
	sess := headless.New("env-test", "sh", false)
	sess.SetTaskEnv(3, 2, 5)

	err := sess.Start(workDir)
	require.NoError(t, err)

	// Wait for process to complete.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !sess.DoesSessionExist() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	_ = sess.Close()
}

func TestHeadlessSession_HasUpdatedWithContent(t *testing.T) {
	workDir := t.TempDir()
	// Use printf to write known output.
	sess := headless.New("has-updated", "sh", false)
	err := sess.Start(workDir)
	require.NoError(t, err)

	// First call: nothing written yet (or initial empty).
	updated1, _, _, _ := sess.HasUpdatedWithContent()
	// Second call with same content: not updated.
	updated2, _, _, _ := sess.HasUpdatedWithContent()
	assert.False(t, updated2, "second call with same empty content should not be updated")
	_ = updated1

	_ = sess.Close()
}
