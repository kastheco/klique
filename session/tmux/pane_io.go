package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/kastheco/kasmos/log"
)

// GetSanitizedName returns the kas_-prefixed tmux session name.
func (t *TmuxSession) GetSanitizedName() string {
	return t.sanitizedName
}

// GetPTY returns the PTY file attached to this session, or nil if not attached.
func (t *TmuxSession) GetPTY() *os.File {
	return t.ptmx
}

// SendKeys sends a literal string to the tmux pane using send-keys -l.
// The -l flag sends the string as-is without interpreting key names.
func (t *TmuxSession) SendKeys(keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-l", "-t", t.sanitizedName, keys)
	return t.cmdExec.Run(cmd)
}

// TapEnter sends a single Enter key to the tmux pane.
func (t *TmuxSession) TapEnter() error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Enter")
	return t.cmdExec.Run(cmd)
}

// TapRight sends a single Right arrow key to the tmux pane.
func (t *TmuxSession) TapRight() error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Right")
	return t.cmdExec.Run(cmd)
}

// TapDAndEnter sends "D Enter" to the tmux pane — used to dismiss the aider
// "open documentation url?" prompt by selecting the default (D) and confirming.
func (t *TmuxSession) TapDAndEnter() error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "D", "Enter")
	return t.cmdExec.Run(cmd)
}

// SendPermissionResponse sends the appropriate key sequence for the given
// permission choice, then waits 300ms for the confirmation dialog to appear
// and confirms with a second Enter.
//
// Permission menu layout (left to right):
//
//	[Allow Once] → [Allow Always] → [Reject]
//
// AllowOnce: Enter (default, no navigation needed) + confirm Enter → 2 commands
// AllowAlways: Right + Enter + confirm Enter                        → 3 commands
// Reject: Right + Right + Enter + confirm Enter                     → 4 commands
func (t *TmuxSession) SendPermissionResponse(choice PermissionChoice) error {
	switch choice {
	case PermissionAllowAlways:
		if err := t.TapRight(); err != nil {
			return fmt.Errorf("SendPermissionResponse: navigate right: %w", err)
		}
	case PermissionReject:
		if err := t.TapRight(); err != nil {
			return fmt.Errorf("SendPermissionResponse: navigate right (1): %w", err)
		}
		if err := t.TapRight(); err != nil {
			return fmt.Errorf("SendPermissionResponse: navigate right (2): %w", err)
		}
	}

	// Confirm the selected choice.
	if err := t.TapEnter(); err != nil {
		return fmt.Errorf("SendPermissionResponse: confirm selection: %w", err)
	}

	// Wait for the confirmation dialog to appear before dismissing it.
	time.Sleep(300 * time.Millisecond)

	// Dismiss the confirmation dialog.
	if err := t.TapEnter(); err != nil {
		return fmt.Errorf("SendPermissionResponse: confirm dialog: %w", err)
	}
	return nil
}

// CapturePaneContent captures the full visible content of the tmux pane,
// joining wrapped lines (-J) and preserving escape sequences (-e).
func (t *TmuxSession) CapturePaneContent() (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-p", "-e", "-J", "-t", t.sanitizedName)
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("capture-pane: %w", err)
	}
	return string(output), nil
}

// CapturePaneContentWithOptions captures pane content between line offsets
// start and end. Negative values (e.g. "-1000") capture scrollback history.
func (t *TmuxSession) CapturePaneContentWithOptions(start, end string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-p", "-e", "-J",
		"-t", t.sanitizedName, "-S", start, "-E", end)
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("capture-pane with options: %w", err)
	}
	return string(output), nil
}

// HasUpdated captures the pane and reports whether the session appears active.
// updated is true when content has changed or is still within the debounce window.
// hasPrompt is true when the adapter's DetectPrompt returns true for the captured content.
func (t *TmuxSession) HasUpdated() (updated bool, hasPrompt bool) {
	updated, hasPrompt, _, _ = t.HasUpdatedWithContent()
	return
}

// HasUpdatedWithContent is like HasUpdated but also returns the raw captured
// content and a boolean indicating whether the capture succeeded.
func (t *TmuxSession) HasUpdatedWithContent() (updated bool, hasPrompt bool, content string, captured bool) {
	c, err := t.CapturePaneContent()
	if err != nil {
		if t.monitor != nil && t.monitor.RecordFailure() {
			log.ErrorLog.Printf("error capturing pane content in status monitor: %v", err)
		}
		return false, false, "", false
	}

	if t.monitor != nil {
		t.monitor.ResetFailures()
	}

	// Detect prompt via adapter (ANSI-stripped content).
	adapter := AdapterFor(t.program)
	if adapter != nil {
		plain := ansiRe.ReplaceAllString(c, "")
		hasPrompt = adapter.DetectPrompt(plain)
	}

	// Delegate content change detection to the monitor.
	if t.monitor != nil {
		updated = t.monitor.RecordContent(c)
	} else {
		updated = true
	}

	return updated, hasPrompt, c, true
}

// GetPanePID returns the PID of the process running in the tmux pane.
func (t *TmuxSession) GetPanePID() (int, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "-t", t.sanitizedName, "#{pane_pid}")
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return 0, fmt.Errorf("display-message pane_pid: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, fmt.Errorf("parse pane PID %q: %w", strings.TrimSpace(string(output)), err)
	}
	return pid, nil
}
