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

func (i *Instance) Preview() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}
	return i.tmuxSession.CapturePaneContent()
}

func (i *Instance) HasUpdated() (updated bool, hasPrompt bool) {
	if !i.started {
		return false, false
	}
	return i.tmuxSession.HasUpdated()
}

// NewEmbeddedTerminalForInstance creates an embedded terminal emulator connected
// to this instance's tmux PTY for zero-latency interactive focus mode.
func (i *Instance) NewEmbeddedTerminalForInstance(cols, rows int) (*EmbeddedTerminal, error) {
	if !i.started || i.tmuxSession == nil {
		return nil, fmt.Errorf("instance not started")
	}
	sessionName := i.tmuxSession.GetSanitizedName()
	return NewEmbeddedTerminal(sessionName, cols, rows)
}

// TapEnter sends an enter key press to the tmux session if AutoYes is enabled.
func (i *Instance) TapEnter() {
	if !i.started || !i.AutoYes {
		return
	}
	if err := i.tmuxSession.TapEnter(); err != nil {
		log.ErrorLog.Printf("error tapping enter: %v", err)
	}
}

func (i *Instance) Attach() (chan struct{}, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot attach instance that has not been started")
	}
	return i.tmuxSession.Attach()
}

func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}
	return i.tmuxSession.SetDetachedSize(width, height)
}

// GetGitWorktree returns the git worktree for the instance
func (i *Instance) GetGitWorktree() (*git.GitWorktree, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot get git worktree for instance that has not been started")
	}
	return i.gitWorktree, nil
}

// SendPrompt sends a prompt to the tmux session
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	if i.tmuxSession == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	if err := i.tmuxSession.SendKeys(prompt); err != nil {
		return fmt.Errorf("error sending keys to tmux session: %w", err)
	}

	// Brief pause to prevent carriage return from being interpreted as newline
	time.Sleep(100 * time.Millisecond)
	if err := i.tmuxSession.TapEnter(); err != nil {
		return fmt.Errorf("error tapping enter: %w", err)
	}

	return nil
}

// PreviewFullHistory captures the entire tmux pane output including full scrollback history
func (i *Instance) PreviewFullHistory() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}
	return i.tmuxSession.CapturePaneContentWithOptions("-", "-")
}

// SetTmuxSession sets the tmux session for testing purposes
func (i *Instance) SetTmuxSession(session *tmux.TmuxSession) {
	i.tmuxSession = session
}

// MarkStartedForTest sets the started flag without spawning a real tmux session.
// Use only in tests that need to simulate a running instance.
func (i *Instance) MarkStartedForTest() {
	i.started = true
}

// SendKeys sends keys to the tmux session
func (i *Instance) SendKeys(keys string) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	return i.tmuxSession.SendKeys(keys)
}

// InstanceMetadata holds the results of polling a single instance.
// Collected in a goroutine — all fields are values, no pointers into the model.
type InstanceMetadata struct {
	Content            string // raw tmux capture-pane output
	ContentCaptured    bool
	Updated            bool
	HasPrompt          bool
	DiffStats          *git.DiffStats
	CPUPercent         float64
	MemMB              float64
	ResourceUsageValid bool
	TmuxAlive          bool // tmux has-session result (for reviewer completion check)
}

// CollectMetadata gathers all per-tick data for this instance via subprocess calls.
// Safe to call from a goroutine — reads only, no model mutations.
// Combines HasUpdated + UpdateDiffStats + UpdateResourceUsage data collection
// into a single method, eliminating redundant capture-pane calls.
func (i *Instance) CollectMetadata() InstanceMetadata {
	var m InstanceMetadata

	if !i.started || i.Status == Paused {
		return m
	}

	// Single capture-pane call — reused for hash check, activity parsing, and preview.
	m.Updated, m.HasPrompt, m.Content, m.ContentCaptured = i.tmuxSession.HasUpdatedWithContent()

	// Git diff stats
	if i.gitWorktree != nil {
		stats := i.gitWorktree.Diff()
		if stats.Error != nil {
			if !strings.Contains(stats.Error.Error(), "base commit SHA not set") &&
				!strings.Contains(stats.Error.Error(), "worktree path gone") {
				log.WarningLog.Printf("diff stats error: %v", stats.Error)
			}
			// On error, return nil stats (caller keeps previous)
		} else {
			m.DiffStats = stats
		}
	}

	// Resource usage (pgrep + ps)
	m.CPUPercent, m.MemMB, m.ResourceUsageValid = i.collectResourceUsage()

	// Session liveness (tmux has-session) — used by reviewer completion check.
	m.TmuxAlive = i.TmuxAlive()

	return m
}

// SetDiffStats sets the diff stats from externally collected data.
func (i *Instance) SetDiffStats(stats *git.DiffStats) {
	i.diffStats = stats
}

// UpdateDiffStats updates the git diff statistics for this instance
func (i *Instance) UpdateDiffStats() error {
	if !i.started {
		i.diffStats = nil
		return nil
	}

	if i.Status == Paused {
		// Keep the previous diff stats if the instance is paused
		return nil
	}

	stats := i.gitWorktree.Diff()
	if stats.Error != nil {
		if strings.Contains(stats.Error.Error(), "base commit SHA not set") {
			// Worktree is not fully set up yet, not an error
			i.diffStats = nil
			return nil
		}
		if strings.Contains(stats.Error.Error(), "worktree path gone") {
			// Worktree was cleaned up (pause, merge, external deletion).
			// Clear stats silently — don't spam logs every tick.
			i.diffStats = nil
			return nil
		}
		return fmt.Errorf("failed to get diff stats: %w", stats.Error)
	}

	i.diffStats = stats
	return nil
}

// collectResourceUsage queries CPU and memory usage via subprocess calls.
// Returns (cpu%, memMB, ok). Safe to call from a goroutine.
func (i *Instance) collectResourceUsage() (float64, float64, bool) {
	if !i.started || i.tmuxSession == nil {
		return 0, 0, false
	}

	pid, err := i.tmuxSession.GetPanePID()
	if err != nil {
		return 0, 0, false
	}

	targetPid := strconv.Itoa(pid)
	childCmd := exec.Command("pgrep", "-P", strconv.Itoa(pid))
	if childOutput, err := childCmd.Output(); err == nil {
		if children := strings.Fields(strings.TrimSpace(string(childOutput))); len(children) > 0 {
			targetPid = children[0]
		}
	}

	psCmd := exec.Command("ps", "-o", "%cpu=,rss=", "-p", targetPid)
	output, err := psCmd.Output()
	if err != nil {
		return 0, 0, false
	}

	fields := strings.Fields(strings.TrimSpace(string(output)))
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

// UpdateResourceUsage queries the process tree for CPU and memory usage.
// Kept for backward compat but now delegates to collectResourceUsage.
func (i *Instance) UpdateResourceUsage() {
	if cpu, mem, ok := i.collectResourceUsage(); ok {
		i.CPUPercent, i.MemMB = cpu, mem
	}
}

// GetDiffStats returns the current git diff statistics
func (i *Instance) GetDiffStats() *git.DiffStats {
	return i.diffStats
}
