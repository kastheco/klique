package session

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/session/tmux"
)

// Preview returns the current pane content as a string.
// Returns an empty string if the instance has not been started or is paused.
func (i *Instance) Preview() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}
	return i.executionSession.CapturePaneContent()
}

// HasUpdated reports whether the pane content has changed since the last check.
// Returns (false, false) if the instance has not been started.
func (i *Instance) HasUpdated() (updated bool, hasPrompt bool) {
	if !i.started {
		return false, false
	}
	return i.executionSession.HasUpdated()
}

// NewEmbeddedTerminalForInstance creates an embedded terminal emulator connected
// to this instance's tmux PTY for zero-latency interactive focus mode.
func (i *Instance) NewEmbeddedTerminalForInstance(cols, rows int) (*EmbeddedTerminal, error) {
	if !i.started || i.executionSession == nil {
		return nil, fmt.Errorf("instance not started")
	}
	sessionName := i.executionSession.GetSanitizedName()
	return NewEmbeddedTerminal(sessionName, cols, rows)
}

// TapEnter sends an enter keypress to the pane when AutoYes is enabled.
// No-op if the instance is not started or AutoYes is false.
func (i *Instance) TapEnter() {
	if !i.started || !i.AutoYes {
		return
	}
	if err := i.executionSession.TapEnter(); err != nil {
		log.ErrorLog.Printf("error tapping enter: %v", err)
	}
}

// Attach connects the caller to the instance's tmux session.
// Returns an error if the instance has not been started.
func (i *Instance) Attach() (chan struct{}, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot attach instance that has not been started")
	}
	return i.executionSession.Attach()
}

// SetPreviewSize resizes the detached pane to the given dimensions.
// Returns an error if the instance is not started or is paused.
func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}
	return i.executionSession.SetDetachedSize(width, height)
}

// GetGitWorktree returns the git worktree associated with this instance.
// Returns an error if the instance has not been started.
func (i *Instance) GetGitWorktree() (*git.GitWorktree, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot get git worktree for instance that has not been started")
	}
	return i.gitWorktree, nil
}

// SendPrompt sends a text prompt followed by an enter keypress to the agent pane.
// Returns an error if the instance is not started or the tmux session is nil.
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	if i.executionSession == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	if err := i.executionSession.SendKeys(prompt); err != nil {
		return fmt.Errorf("error sending keys to tmux session: %w", err)
	}
	// Brief pause to prevent the carriage return from being misinterpreted.
	time.Sleep(100 * time.Millisecond)
	if err := i.executionSession.TapEnter(); err != nil {
		return fmt.Errorf("error tapping enter: %w", err)
	}
	return nil
}

// PreviewFullHistory captures the complete tmux pane output including the full scrollback buffer.
// Returns an empty string if the instance is not started or is paused.
func (i *Instance) PreviewFullHistory() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}
	return i.executionSession.CapturePaneContentWithOptions("-", "-")
}

// SetTmuxSession replaces the tmux session handle. Intended for use in tests only.
func (i *Instance) SetTmuxSession(session *tmux.TmuxSession) {
	i.executionSession = session
}

// MarkStartedForTest sets the started flag without spawning a real tmux session.
// Use only in tests that need to simulate a running instance.
func (i *Instance) MarkStartedForTest() {
	i.started = true
}

// SendKeys sends raw key sequences to the pane.
// Returns an error if the instance is not started or is paused.
func (i *Instance) SendKeys(keys string) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	return i.executionSession.SendKeys(keys)
}

// InstanceMetadata holds the results of a single per-tick poll for one instance.
// All fields are value types — safe to pass between goroutines without synchronization.
type InstanceMetadata struct {
	// Content is the raw tmux capture-pane output.
	Content         string
	ContentCaptured bool
	Updated         bool
	HasPrompt       bool
	CPUPercent      float64
	MemMB           float64
	// ResourceUsageValid is true when CPU/memory data was successfully collected.
	ResourceUsageValid bool
	// TmuxAlive reflects the result of tmux has-session (used by the reviewer completion check).
	TmuxAlive        bool
	PermissionPrompt *PermissionPrompt
}

// CollectMetadata gathers all per-tick data for this instance via subprocess calls.
// Safe to call from a goroutine — does not mutate the instance's cached preview fields.
func (i *Instance) CollectMetadata() InstanceMetadata {
	var m InstanceMetadata

	if !i.started || i.Status == Paused {
		return m
	}

	// Single capture-pane call shared by hash check, activity parsing, and preview.
	m.Updated, m.HasPrompt, m.Content, m.ContentCaptured = i.executionSession.HasUpdatedWithContent()

	// Permission prompt detection — only meaningful when content was actually captured.
	if m.ContentCaptured && m.Content != "" {
		m.PermissionPrompt = ParsePermissionPrompt(m.Content, i.Program)
	}

	// Resource usage via pgrep + ps.
	m.CPUPercent, m.MemMB, m.ResourceUsageValid = i.collectResourceUsage()

	// Session liveness check for the reviewer completion logic.
	m.TmuxAlive = i.TmuxAlive()

	return m
}

// collectResourceUsage samples CPU and RSS memory for the agent process via pgrep and ps.
// Returns (cpu%, memMB, ok). Safe to call from a goroutine.
func (i *Instance) collectResourceUsage() (float64, float64, bool) {
	if !i.started || i.executionSession == nil {
		return 0, 0, false
	}

	pid, err := i.executionSession.GetPanePID()
	if err != nil {
		return 0, 0, false
	}

	// Prefer the first child process of the pane's shell so we measure the agent binary, not the shell.
	targetPid := strconv.Itoa(pid)
	childOut, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
	if err == nil {
		if children := strings.Fields(strings.TrimSpace(string(childOut))); len(children) > 0 {
			targetPid = children[0]
		}
	}

	psOut, err := exec.Command("ps", "-o", "%cpu=,rss=", "-p", targetPid).Output()
	if err != nil {
		return 0, 0, false
	}

	fields := strings.Fields(strings.TrimSpace(string(psOut)))
	if len(fields) < 2 {
		return 0, 0, false
	}

	cpu, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, false
	}
	rssKB, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, false
	}
	return cpu, rssKB / 1024, true
}

// UpdateResourceUsage refreshes the instance's CPU and memory fields.
func (i *Instance) UpdateResourceUsage() {
	if cpu, mem, ok := i.collectResourceUsage(); ok {
		i.CPUPercent, i.MemMB = cpu, mem
	}
}

// SendPermissionResponse forwards a permission dialog choice to the agent pane.
// No-op if the instance is not started or the tmux session is nil.
func (i *Instance) SendPermissionResponse(choice tmux.PermissionChoice) {
	if !i.started || i.executionSession == nil {
		return
	}
	if err := i.executionSession.SendPermissionResponse(choice); err != nil {
		log.ErrorLog.Printf("error sending permission response: %v", err)
	}
}
