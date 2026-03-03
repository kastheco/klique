//go:build !windows

package tmux

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// monitorWindowSize starts two goroutines that propagate terminal resize
// events (SIGWINCH) to the PTY with a 50 ms debounce.
//
// Goroutine 1 — signal bridge: listens for SIGWINCH and forwards events to
// an internal channel without blocking, dropping duplicates.
//
// Goroutine 2 — debounce + apply: reads from that channel, resets a 50 ms
// AfterFunc timer on each event, and calls updateWindowSize when the timer fires.
//
// Both goroutines respect t.ctx.Done() and exit when the context is cancelled.
func (t *TmuxSession) monitorWindowSize() {
	if t.ctx == nil || t.ptmx == nil || t.wg == nil {
		return
	}

	ctx := t.ctx
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)

	// eventCh carries resize events from goroutine 1 to goroutine 2.
	// Buffered to 1 so goroutine 1 never blocks; excess signals are merged.
	eventCh := make(chan struct{}, 1)

	// Goroutine 1: bridge SIGWINCH → eventCh.
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer signal.Stop(sigCh)
		defer close(eventCh)

		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-sigCh:
				if !ok {
					return
				}
				// Non-blocking send: if eventCh is already full, the pending
				// event will cover this one (debounce merging).
				select {
				case eventCh <- struct{}{}:
				default:
				}
			}
		}
	}()

	// Goroutine 2: debounce events and apply window size.
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()

		var debounce *time.Timer

		for {
			select {
			case <-ctx.Done():
				if debounce != nil {
					debounce.Stop()
				}
				return
			case _, ok := <-eventCh:
				if !ok {
					if debounce != nil {
						debounce.Stop()
					}
					return
				}
				// Reset the debounce timer on each event.
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(50*time.Millisecond, func() {
					sz, err := pty.GetsizeFull(os.Stdout)
					if err != nil || sz.Cols == 0 || sz.Rows == 0 {
						return
					}
					_ = t.updateWindowSize(sz.Cols, sz.Rows)
				})
			}
		}
	}()
}
