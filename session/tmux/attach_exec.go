package tmux

import (
	"fmt"
	"io"
)

// Attacher is satisfied by any type that can initiate a tmux attach and return
// a channel that is closed when the session detaches.
// *session.Instance satisfies this interface via its Attach method.
type Attacher interface {
	Attach() (chan struct{}, error)
}

// AttachExecCommand wraps an Attacher so it can be passed to tea.Exec,
// allowing bubbletea to properly release the terminal before attach and
// restore it after detach — preventing stdin contention between bubbletea's
// event loop and the tmux attach goroutines.
type AttachExecCommand struct {
	target Attacher
}

// NewAttachExecCommand returns an AttachExecCommand that delegates to target.
func NewAttachExecCommand(target Attacher) *AttachExecCommand {
	return &AttachExecCommand{target: target}
}

// Run implements tea.ExecCommand. It calls Attach on the target exactly once,
// returns any attach error, then blocks until the detach channel is closed.
func (c *AttachExecCommand) Run() error {
	if c.target == nil {
		return fmt.Errorf("attach: target is nil")
	}
	ch, err := c.target.Attach()
	if err != nil {
		return fmt.Errorf("attach: %w", err)
	}
	if ch == nil {
		return fmt.Errorf("attach: Attach() returned nil channel without error")
	}
	<-ch
	return nil
}

// SetStdin is a no-op. The attach goroutines read/write os.Stdin and os.Stdout
// directly, so bubbletea's captured stdin/stdout are not used during attach.
func (c *AttachExecCommand) SetStdin(_ io.Reader) {}

// SetStdout is a no-op. See SetStdin.
func (c *AttachExecCommand) SetStdout(_ io.Writer) {}

// SetStderr is a no-op. See SetStdin.
func (c *AttachExecCommand) SetStderr(_ io.Writer) {}
