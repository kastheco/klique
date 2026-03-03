package tmux

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/kastheco/kasmos/log"
)

// Attach connects the calling terminal to the tmux session.
// It disables mouse on the enclosing outer tmux session (if kasmos is running
// inside tmux), then spawns two goroutines:
//  1. PTY output → os.Stdout (io.Copy)
//  2. os.Stdin → PTY, with Ctrl+Q (0x11) and Ctrl+Space (0x00) as detach keys
//     (only when they arrive as the sole byte in a single read — the n==1 guard
//     prevents NUL bytes in paste data or multi-byte sequences from triggering
//     spurious detach)
//
// A 50 ms nuke window discards any buffered stdin bytes that accumulated
// before the attach. Window-size monitoring is started via monitorWindowSize.
//
// Returns a channel that is closed when Detach completes.
func (t *TmuxSession) Attach() (chan struct{}, error) {
	// Detect and disable outer tmux mouse so the inner session gets raw events.
	outer := outerTmuxSession()
	t.outerMouseWasEnabled = outerMouseEnabled(outer)
	if t.outerMouseWasEnabled && outer != "" {
		_ = exec.Command("tmux", "set-option", "-t", outer, "mouse", "off").Run()
	}

	ch := make(chan struct{})
	t.attachCh = ch

	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.cancel = cancel
	wg := &sync.WaitGroup{}
	t.wg = wg

	// Capture ptmx locally so goroutines don't race with Detach setting it to nil.
	ptmx := t.ptmx

	// Goroutine 1: stream PTY output to os.Stdout.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stdout, ptmx)
	}()

	// Goroutine 2: read stdin, write to PTY; detach on Ctrl+Q or Ctrl+Space.
	wg.Add(1)
	go func() {
		defer wg.Done()

		buf := make([]byte, 4096)

		// Nuke window: discard any stdin bytes buffered before we attached.
		// We do short deadline reads for ~50ms to drain the buffer.
		nukeEnd := time.Now().Add(50 * time.Millisecond)
		for time.Now().Before(nukeEnd) {
			_ = os.Stdin.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
			_, _ = os.Stdin.Read(buf)
		}
		_ = os.Stdin.SetReadDeadline(time.Time{}) // clear deadline

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Short read deadline allows the context-check loop to run.
			_ = os.Stdin.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, err := os.Stdin.Read(buf)
			_ = os.Stdin.SetReadDeadline(time.Time{})
			if err != nil {
				// Timeout or EOF — re-check context on next iteration.
				continue
			}

			// Detach only when the read is an isolated single byte.
			// The n==1 guard prevents two regressions that arise from scanning
			// every byte in a multi-byte read:
			//   1. NUL (0x00) bytes embedded in paste data (e.g. from Ctrl+Space
			//      bindings in Emacs) triggering a spurious detach.
			//   2. Legitimate input bytes before a detach key being discarded
			//      instead of forwarded to the PTY.
			if n == 1 && (buf[0] == 0x11 || buf[0] == 0x00) { // Ctrl+Q or Ctrl+Space
				t.Detach()
				return
			}
			_, _ = ptmx.Write(buf[:n])
		}
	}()

	// Start platform-specific window-size monitoring.
	t.monitorWindowSize()

	return ch, nil
}

// Detach disconnects from the tmux session:
//  1. Restores outer tmux mouse mode if it was disabled.
//  2. Cancels context (signals goroutines to stop).
//  3. Closes the PTY (unblocks the io.Copy goroutine).
//  4. Waits for goroutines with a 500 ms timeout.
//  5. Calls Restore() to create a background monitoring PTY.
//  6. Closes the attach channel (signals callers that detach is complete).
func (t *TmuxSession) Detach() {
	t.restoreOuterMouse()

	// Cancel context to signal goroutines.
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	// Close the PTY — this causes the io.Copy(os.Stdout, ptmx) goroutine to return.
	if t.ptmx != nil {
		_ = t.ptmx.Close()
		t.ptmx = nil
	}

	// Wait for goroutines with a timeout.
	// The stdin goroutine uses short read deadlines so it will exit promptly
	// once ctx is cancelled, but we don't want to block forever.
	if t.wg != nil {
		done := make(chan struct{})
		go func() {
			t.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			log.InfoLog.Printf("Detach: goroutines did not exit within 500ms timeout")
		}
		t.wg = nil
	}

	// Recreate a background PTY for status monitoring (HasUpdated ticks).
	if err := t.Restore(); err != nil {
		log.ErrorLog.Printf("Detach: error restoring background PTY: %v", err)
	}

	// Signal any waiter that detach is complete.
	if t.attachCh != nil {
		close(t.attachCh)
		t.attachCh = nil
	}
}

// DetachSafely is like Detach but safe to call when not attached (returns nil).
func (t *TmuxSession) DetachSafely() error {
	if t.attachCh == nil {
		return nil
	}
	t.Detach()
	return nil
}

// SetDetachedSize resizes the background PTY to the given terminal dimensions.
// Used to keep the tmux pane correctly sized while no interactive client is attached.
func (t *TmuxSession) SetDetachedSize(width, height int) error {
	return t.updateWindowSize(uint16(width), uint16(height))
}

// updateWindowSize calls pty.Setsize to resize the PTY file descriptor.
func (t *TmuxSession) updateWindowSize(cols, rows uint16) error {
	if t.ptmx == nil {
		return nil
	}
	return pty.Setsize(t.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// restoreOuterMouse re-enables mouse mode on the outer tmux session if Attach
// disabled it.
func (t *TmuxSession) restoreOuterMouse() {
	if !t.outerMouseWasEnabled {
		return
	}
	outer := outerTmuxSession()
	if outer == "" {
		return
	}
	_ = exec.Command("tmux", "set-option", "-t", outer, "mouse", "on").Run()
	t.outerMouseWasEnabled = false
}
