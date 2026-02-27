package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/kastheco/kasmos/log"
)

// PermissionChoice represents the user's response to an opencode permission prompt.
type PermissionChoice int

const (
	PermissionAllowOnce PermissionChoice = iota
	PermissionAllowAlways
	PermissionReject
)

// TapRight sends a Right arrow keystroke to the tmux pane.
func (t *TmuxSession) TapRight() error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Right")
	return t.cmdExec.Run(cmd)
}

// sendKeyDelay is the pause between the first and second Enter in opencode's
// two-step permission flow: selection → confirmation. 300ms gives the TUI time
// to render the confirmation dialog before the second keystroke arrives.
const sendKeyDelay = 300 * time.Millisecond

// SendPermissionResponse sends the key sequence for the given permission choice.
// opencode's permission prompt is a two-step flow:
//  1. Select the choice (arrow keys + Enter)
//  2. Confirm the selection (Enter again after a short delay)
//
// Allow once: Enter, delay, Enter. Allow always: Right Enter, delay, Enter.
// Reject: Right Right Enter, delay, Enter.
func (t *TmuxSession) SendPermissionResponse(choice PermissionChoice) error {
	switch choice {
	case PermissionAllowOnce:
		if err := t.TapEnter(); err != nil {
			return err
		}
	case PermissionAllowAlways:
		if err := t.TapRight(); err != nil {
			return err
		}
		if err := t.TapEnter(); err != nil {
			return err
		}
	case PermissionReject:
		if err := t.TapRight(); err != nil {
			return err
		}
		if err := t.TapRight(); err != nil {
			return err
		}
		if err := t.TapEnter(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown permission choice: %d", choice)
	}

	// Second step: opencode shows a confirmation dialog after the initial
	// selection. Wait for it to render, then confirm.
	time.Sleep(sendKeyDelay)
	return t.TapEnter()
}

// TapEnter sends an Enter keystroke to the tmux pane via tmux send-keys.
// Using tmux send-keys is more reliable than raw PTY writes for TUI programs
// (e.g. bubbletea-based CLIs like opencode) that manage their own input loop.
func (t *TmuxSession) TapEnter() error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Enter")
	return t.cmdExec.Run(cmd)
}

// TapDAndEnter sends 'D' followed by Enter to the tmux pane.
// Used for Aider's "Don't open documentation" prompt.
func (t *TmuxSession) TapDAndEnter() error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "D", "Enter")
	return t.cmdExec.Run(cmd)
}

// SendKeys sends literal text to the tmux pane via tmux send-keys -l.
// The -l flag transmits each character verbatim without key-binding interpretation,
// making it equivalent to the user typing the text directly.
func (t *TmuxSession) SendKeys(keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-l", "-t", t.sanitizedName, keys)
	return t.cmdExec.Run(cmd)
}

// HasUpdated checks if the tmux pane content has changed since the last tick. It also returns true if
// the tmux pane has a prompt for aider or claude code.
func (t *TmuxSession) HasUpdated() (updated bool, hasPrompt bool) {
	content, err := t.CapturePaneContent()
	if err != nil {
		t.monitor.captureFailures++
		// Log the first failure as an error, then downgrade subsequent failures
		// to warnings (Sentry breadcrumbs only) to avoid bombarding Sentry with
		// CaptureMessage events when a pane is permanently gone.
		if t.monitor.captureFailures == 1 {
			log.ErrorLog.Printf("error capturing pane content in status monitor: %v", err)
		} else if t.monitor.captureFailures%30 == 0 {
			log.WarningLog.Printf("error capturing pane content in status monitor (failure #%d): %v",
				t.monitor.captureFailures, err)
		}
		return false, false
	}
	t.monitor.captureFailures = 0 // reset on success

	// Detect when the program is idle and waiting for user input.
	plain := ansi.Strip(content)
	switch {
	case isClaudeProgram(t.program):
		hasPrompt = strings.Contains(plain, "No, and tell Claude what to do differently")
	case isAiderProgram(t.program):
		hasPrompt = strings.Contains(plain, "(Y)es/(N)o/(D)on't ask again")
	case isGeminiProgram(t.program):
		hasPrompt = strings.Contains(plain, "Yes, allow once")
	case isOpenCodeProgram(t.program):
		// opencode shows "esc interrupt" in its bottom bar only while a task is running.
		// When idle and waiting for input, that line disappears. So idle = no interrupt shown.
		// Strip ANSI first — "esc" and "interrupt" are separately styled with codes between them.
		hasPrompt = !strings.Contains(plain, "esc interrupt")
	}

	newHash := t.monitor.hash(content)
	if !bytes.Equal(newHash, t.monitor.prevOutputHash) {
		t.monitor.prevOutputHash = newHash
		t.monitor.unchangedTicks = 0
		return true, hasPrompt
	}

	// Content unchanged — only report !updated after a debounce threshold so that
	// brief pauses (API waits, thinking between tool calls) don't cause false
	// Running→Ready transitions. ~6 ticks × 500ms = 3s of stability required.
	t.monitor.unchangedTicks++
	if t.monitor.unchangedTicks < 6 {
		// Still debouncing — report as updated to keep status as Running.
		return true, hasPrompt
	}
	return false, hasPrompt
}

// HasUpdatedWithContent is like HasUpdated but also returns the raw captured
// pane content and whether capture succeeded, eliminating duplicate capture-pane calls.
func (t *TmuxSession) HasUpdatedWithContent() (updated bool, hasPrompt bool, content string, captured bool) {
	raw, err := t.CapturePaneContent()
	if err != nil {
		t.monitor.captureFailures++
		if t.monitor.captureFailures == 1 {
			log.ErrorLog.Printf("error capturing pane content in status monitor: %v", err)
		} else if t.monitor.captureFailures%30 == 0 {
			log.WarningLog.Printf("error capturing pane content in status monitor (failure #%d): %v",
				t.monitor.captureFailures, err)
		}
		return false, false, "", false
	}
	t.monitor.captureFailures = 0

	content = raw
	captured = true

	plain := ansi.Strip(content)
	switch {
	case isClaudeProgram(t.program):
		hasPrompt = strings.Contains(plain, "No, and tell Claude what to do differently")
	case isAiderProgram(t.program):
		hasPrompt = strings.Contains(plain, "(Y)es/(N)o/(D)on't ask again")
	case isGeminiProgram(t.program):
		hasPrompt = strings.Contains(plain, "Yes, allow once")
	case isOpenCodeProgram(t.program):
		hasPrompt = !strings.Contains(plain, "esc interrupt")
	}

	newHash := t.monitor.hash(content)
	if !bytes.Equal(newHash, t.monitor.prevOutputHash) {
		t.monitor.prevOutputHash = newHash
		t.monitor.unchangedTicks = 0
		return true, hasPrompt, content, true
	}

	t.monitor.unchangedTicks++
	if t.monitor.unchangedTicks < 6 {
		return true, hasPrompt, content, true
	}
	return false, hasPrompt, content, true
}

// CapturePaneContent captures the content of the tmux pane
func (t *TmuxSession) CapturePaneContent() (string, error) {
	// Add -e flag to preserve escape sequences (ANSI color codes)
	cmd := exec.Command("tmux", "capture-pane", "-p", "-e", "-J", "-t", t.sanitizedName)
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("error capturing pane content: %v", err)
	}
	return string(output), nil
}

// CapturePaneContentWithOptions captures the pane content with additional options
// start and end specify the starting and ending line numbers (use "-" for the start/end of history)
func (t *TmuxSession) CapturePaneContentWithOptions(start, end string) (string, error) {
	// Add -e flag to preserve escape sequences (ANSI color codes)
	cmd := exec.Command("tmux", "capture-pane", "-p", "-e", "-J", "-S", start, "-E", end, "-t", t.sanitizedName)
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to capture tmux pane content with options: %v", err)
	}
	return string(output), nil
}

// GetPTY returns the master PTY file descriptor for direct I/O.
func (t *TmuxSession) GetPTY() *os.File {
	return t.ptmx
}

// GetSanitizedName returns the tmux session name used for tmux commands.
func (t *TmuxSession) GetSanitizedName() string {
	return t.sanitizedName
}

func (t *TmuxSession) GetPanePID() (int, error) {
	pidCmd := exec.Command("tmux", "display-message", "-p", "-t", t.sanitizedName, "#{pane_pid}")
	output, err := t.cmdExec.Output(pidCmd)
	if err != nil {
		return 0, fmt.Errorf("failed to get pane PID: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, fmt.Errorf("failed to parse pane PID: %w", err)
	}
	return pid, nil
}
