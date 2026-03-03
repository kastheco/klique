//go:build windows

package tmux

import (
	"os"
	"time"

	"github.com/creack/pty"
)

// monitorWindowSize polls the terminal size every 250 ms on Windows and
// propagates changes to the PTY when the dimensions differ from the last
// known size.
//
// Windows does not deliver SIGWINCH; polling is the standard alternative.
// The goroutine respects t.ctx.Done() and exits when the context is cancelled.
func (t *TmuxSession) monitorWindowSize() {
	if t.ctx == nil || t.ptmx == nil || t.wg == nil {
		return
	}

	ctx := t.ctx

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()

		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		var lastCols, lastRows uint16

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sz, err := pty.GetsizeFull(os.Stdout)
				if err != nil || sz.Cols == 0 || sz.Rows == 0 {
					continue
				}
				if sz.Cols != lastCols || sz.Rows != lastRows {
					_ = t.updateWindowSize(sz.Cols, sz.Rows)
					lastCols = sz.Cols
					lastRows = sz.Rows
				}
			}
		}
	}()
}
