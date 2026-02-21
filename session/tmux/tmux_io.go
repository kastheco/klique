package tmux

import (
	"bytes"
	"fmt"
	"github.com/kastheco/klique/log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

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
		// Log the first failure and then only every 30th (roughly every 15s at 500ms ticks)
		// to avoid flooding the log when a pane is gone.
		if t.monitor.captureFailures == 1 || t.monitor.captureFailures%30 == 0 {
			log.ErrorLog.Printf("error capturing pane content in status monitor (failure #%d): %v",
				t.monitor.captureFailures, err)
		}
		return false, false
	}
	t.monitor.captureFailures = 0 // reset on success

	// Detect when the program is idle and waiting for user input.
	switch {
	case isClaudeProgram(t.program):
		hasPrompt = strings.Contains(content, "No, and tell Claude what to do differently")
	case isAiderProgram(t.program):
		hasPrompt = strings.Contains(content, "(Y)es/(N)o/(D)on't ask again")
	case isGeminiProgram(t.program):
		hasPrompt = strings.Contains(content, "Yes, allow once")
	case isOpenCodeProgram(t.program):
		// opencode shows "Ask anything" as placeholder text in the input area when idle.
		hasPrompt = strings.Contains(content, "Ask anything")
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
