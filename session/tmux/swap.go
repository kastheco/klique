package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kastheco/kasmos/cmd"
)

// OuterSessionName returns the name of the enclosing tmux session (the layout
// session that kas tui runs inside), or "" if not inside tmux.
// This is the exported counterpart of the package-private outerTmuxSession().
func OuterSessionName() string {
	if os.Getenv("TMUX") == "" {
		return ""
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SessionExists reports whether the named tmux session exists.
// An *exec.ExitError from `tmux has-session` is treated as (false, nil),
// matching the orphan-discovery behavior in cmd/tmux.go:61-67.
func SessionExists(ex cmd.Executor, sessionName string) (bool, error) {
	c := exec.Command("tmux", "has-session", "-t", sessionName)
	if err := ex.Run(c); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("tmux has-session %s: %w", sessionName, err)
	}
	return true, nil
}

// LayoutPaneID reads a pane ID stored in the outer session's tmux environment
// under the key envName (e.g. "KASMOS_WORKSPACE_PANE").
// The returned string is the raw pane ID (e.g. "%42") ready for use with
// tmux -t flags.
func LayoutPaneID(ex cmd.Executor, sessionName, envName string) (string, error) {
	c := exec.Command("tmux", "show-environment", "-t", sessionName, envName)
	out, err := ex.Output(c)
	if err != nil {
		return "", fmt.Errorf("show-environment %s %s: %w", sessionName, envName, err)
	}
	// tmux prints "VARNAME=value"; strip the key prefix.
	val := strings.TrimSpace(string(out))
	if idx := strings.Index(val, "="); idx >= 0 {
		val = val[idx+1:]
	}
	if val == "" {
		return "", fmt.Errorf("env %s not set in session %s", envName, sessionName)
	}
	return val, nil
}

// SwapRightPaneToSession swaps the workspace right pane to show the first pane
// of sourceSession. The workspace pane ID is read from KASMOS_WORKSPACE_PANE
// in the outer session's environment.
//
// The swap uses `tmux swap-pane -d -t <workspace-pane-id> -s <agent-pane-id>`
// so the workspace pane moves to wherever the agent pane was (the agent session)
// and the agent pane appears in the outer layout where the workspace pane was.
func SwapRightPaneToSession(ex cmd.Executor, outerSession, sourceSession string) error {
	workspacePaneID, err := LayoutPaneID(ex, outerSession, "KASMOS_WORKSPACE_PANE")
	if err != nil {
		return fmt.Errorf("get workspace pane: %w", err)
	}

	// Resolve the pane ID of the source session's first window, first pane.
	c := exec.Command("tmux", "display-message", "-p", "-t",
		sourceSession+":0.0", "#{pane_id}")
	out, err := ex.Output(c)
	if err != nil {
		return fmt.Errorf("get source pane id for %s: %w", sourceSession, err)
	}
	agentPaneID := strings.TrimSpace(string(out))
	if agentPaneID == "" {
		return fmt.Errorf("empty pane id for session %s", sourceSession)
	}

	// Perform the swap. -d prevents detaching attached clients.
	swapCmd := exec.Command("tmux", "swap-pane", "-d",
		"-t", workspacePaneID,
		"-s", agentPaneID)
	if err := ex.Run(swapCmd); err != nil {
		return fmt.Errorf("swap-pane to %s: %w", sourceSession, err)
	}
	return nil
}

// ActiveSwappedSession returns the name of the tmux session currently displayed
// in the right pane of outerSession, or "" if the workspace shell is shown.
//
// It compares the pane ID at :0.1 (right pane) against KASMOS_WORKSPACE_PANE.
// When they match the workspace is visible and "" is returned. When they differ
// the right pane belongs to an agent session whose name is resolved via
// `tmux display-message #{session_name}` and returned.
//
// Returns ("", nil) for any condition where no agent swap is active (missing
// env var, missing pane, query error). Returns a non-empty name only when an
// agent session is confirmed to be in the right pane.
func ActiveSwappedSession(ex cmd.Executor, outerSession string) (string, error) {
	workspacePaneID, err := LayoutPaneID(ex, outerSession, "KASMOS_WORKSPACE_PANE")
	if err != nil {
		// No workspace pane recorded — nothing is swapped in.
		return "", nil
	}

	// Query the pane currently in the :0.1 (right) position.
	rightQuery := exec.Command("tmux", "display-message", "-p",
		"-t", outerSession+":0.1", "#{pane_id}")
	out, err := ex.Output(rightQuery)
	if err != nil {
		// No right pane or layout error — treat as no swap.
		return "", nil
	}
	currentRightPaneID := strings.TrimSpace(string(out))
	if currentRightPaneID == "" || currentRightPaneID == workspacePaneID {
		// Workspace pane is already in the right position.
		return "", nil
	}

	// Right pane is an agent pane — resolve its session name.
	nameQuery := exec.Command("tmux", "display-message", "-p",
		"-t", currentRightPaneID, "#{session_name}")
	nameOut, err := ex.Output(nameQuery)
	if err != nil {
		// Could not determine the session; treat as no swap (pane may be gone).
		return "", nil
	}
	return strings.TrimSpace(string(nameOut)), nil
}

// SwapRightPaneToWorkspace restores the workspace shell pane to the right side
// of the outer layout by swapping the pane at position :0.1 back to the pane
// identified by KASMOS_WORKSPACE_PANE.
//
// The function is idempotent: if the workspace pane is already at :0.1, or if
// KASMOS_WORKSPACE_PANE is not set, or if there is no :0.1 pane, it returns
// nil without performing any swap.
func SwapRightPaneToWorkspace(ex cmd.Executor, outerSession string) error {
	workspacePaneID, err := LayoutPaneID(ex, outerSession, "KASMOS_WORKSPACE_PANE")
	if err != nil {
		// Env var not set — nothing to restore.
		return nil
	}

	// Query the current pane ID at position :0.1 (the right pane).
	c := exec.Command("tmux", "display-message", "-p",
		"-t", outerSession+":0.1", "#{pane_id}")
	out, err := ex.Output(c)
	if err != nil {
		// No right pane or layout error — nothing to swap.
		return nil
	}
	currentRightPaneID := strings.TrimSpace(string(out))
	if currentRightPaneID == workspacePaneID {
		// Already the workspace pane — idempotent, nothing to do.
		return nil
	}

	// Swap the workspace pane (wherever it currently is) to the :0.1 position.
	swapCmd := exec.Command("tmux", "swap-pane", "-d",
		"-t", outerSession+":0.1",
		"-s", workspacePaneID)
	if err := ex.Run(swapCmd); err != nil {
		return fmt.Errorf("restore workspace pane: %w", err)
	}
	return nil
}
