package tmux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kastheco/kasmos/cmd"
)

const (
	// EnvWorkspacePane is the tmux session env var holding the workspace (right) pane ID.
	EnvWorkspacePane = "KASMOS_WORKSPACE_PANE"
	// EnvNavPane is the tmux session env var holding the nav (left) pane ID.
	EnvNavPane = "KASMOS_NAV_PANE"
	// workspacePaneWidthPercent is the width reserved for the visible right pane.
	workspacePaneWidthPercent = 68
)

// Layout holds the pane and session identifiers for the kasmos two-pane layout.
// NavPaneID is the left pane running `kas tui --nav-only`.
// WorkspacePaneID is the right pane showing the agent's terminal or a workspace shell.
type Layout struct {
	SessionName     string
	WindowTarget    string
	NavPaneID       string
	WorkspacePaneID string
}

// MainSessionName returns the stable outer tmux session name for a given repo root.
// The name is kas_main_<sanitized-basename> where sanitization strips whitespace
// and replaces dots with underscores (matching tmux's own name rules).
func MainSessionName(repoRoot string) string {
	base := filepath.Base(repoRoot)
	base = whiteSpaceRegex.ReplaceAllString(base, "")
	base = strings.ReplaceAll(base, ".", "_")
	return "kas_main_" + base
}

// EnsureMainLayout creates or reattaches to the kasmos two-pane tmux layout for
// the given repository root. It returns the Layout, a boolean indicating whether
// the session already existed (true = existing, false = newly created), and any error.
//
// When the session already exists the pane IDs are read from persisted session
// environment variables (KASMOS_NAV_PANE, KASMOS_WORKSPACE_PANE) so the caller
// can reuse them without recreating panes.
//
// When creating a new session:
//   - The left nav pane runs: KASMOS_LAYOUT=1 <tuiCommand>
//   - The right workspace pane runs the user's $SHELL (fallback: /bin/bash)
//   - Session env vars are set: KASMOS_LAYOUT, KASMOS_NAV_PANE, KASMOS_WORKSPACE_PANE, KASMOS_REPO_ROOT
//   - Tmux options configured: mouse on, escape-time 0, status on, status-position top
func EnsureMainLayout(ex cmd.Executor, repoRoot, tuiCommand string, cols, rows int) (Layout, bool, error) {
	sessionName := MainSessionName(repoRoot)

	// Check if session already exists.
	hasCmd := exec.Command("tmux", "has-session", fmt.Sprintf("-t=%s", sessionName))
	if err := ex.Run(hasCmd); err == nil {
		// Session exists — read env vars to reconstruct the Layout.
		layout, err := readLayoutEnv(ex, sessionName)
		if err != nil {
			// Return partial layout with known session name so caller can still attach.
			return Layout{
				SessionName:  sessionName,
				WindowTarget: sessionName + ":0",
			}, true, fmt.Errorf("ensure main layout: read existing layout env: %w", err)
		}
		_ = InstallFocusBindings(ex, sessionName)
		return layout, true, nil
	} else if errors.Is(err, exec.ErrNotFound) {
		return Layout{}, false, fmt.Errorf("ensure main layout: tmux not found in PATH: %w", err)
	}

	// Determine workspace shell (right pane).
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	// Build the nav pane command: sets KASMOS_LAYOUT=1 so the root `kas` command
	// running in the left pane recognises it is already inside the layout.
	navCmd := "KASMOS_LAYOUT=1 " + tuiCommand
	popupExec := popupExecutableFromTUICommand(tuiCommand)

	// Create a new detached session; the initial window/pane runs navCmd.
	// -P -F prints session_name|window_id|pane_id on stdout so we can capture them.
	newSessCmd := exec.Command("tmux",
		"new-session", "-d",
		"-s", sessionName,
		"-c", repoRoot,
		"-x", strconv.Itoa(cols),
		"-y", strconv.Itoa(rows),
		"-P", "-F", "#{session_name}|#{window_id}|#{pane_id}",
		navCmd,
	)
	out, err := ex.Output(newSessCmd)
	if err != nil {
		return Layout{}, false, fmt.Errorf("ensure main layout: new-session: %w", err)
	}

	// Parse "sessionName|@windowID|%paneID".
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 3)
	if len(parts) != 3 {
		return Layout{}, false, fmt.Errorf("ensure main layout: unexpected new-session output: %q", string(out))
	}
	windowID := parts[1]  // e.g. "@0"
	navPaneID := parts[2] // e.g. "%0"

	windowTarget := sessionName + ":" + windowID

	// Create the right workspace pane by splitting horizontally.
	workspaceShellCmd := workspaceShellBootstrap(shell)
	splitCmd := exec.Command("tmux",
		"split-window", "-h", "-d",
		"-t", windowTarget,
		"-l", fmt.Sprintf("%d%%", workspacePaneWidthPercent),
		"-P", "-F", "#{pane_id}",
		workspaceShellCmd[0], workspaceShellCmd[1], workspaceShellCmd[2],
	)
	splitOut, err := ex.Output(splitCmd)
	if err != nil {
		// Kill the partial session so there is no orphan.
		_ = ex.Run(exec.Command("tmux", "kill-session", "-t", sessionName))
		return Layout{}, false, fmt.Errorf("ensure main layout: split-window: %w", err)
	}
	workspacePaneID := strings.TrimSpace(string(splitOut))

	layout := Layout{
		SessionName:     sessionName,
		WindowTarget:    windowTarget,
		NavPaneID:       navPaneID,
		WorkspacePaneID: workspacePaneID,
	}

	// Persist pane identifiers and layout flag into the session environment so
	// helpers and future attaches can read them without guessing.
	envVars := []struct{ k, v string }{
		{"KASMOS_LAYOUT", "1"},
		{EnvNavPane, navPaneID},
		{EnvWorkspacePane, workspacePaneID},
		{"KASMOS_EXECUTABLE", popupExec},
		{"KASMOS_REPO_ROOT", repoRoot},
	}
	for _, kv := range envVars {
		envCmd := exec.Command("tmux", "set-environment", "-t", sessionName, kv.k, kv.v)
		if err := ex.Run(envCmd); err != nil {
			return layout, false, fmt.Errorf("ensure main layout: set-environment %s: %w", kv.k, err)
		}
	}

	// Configure tmux options for the kasmos layout session.
	tmuxOpts := []struct{ opt, val string }{
		{"mouse", "on"},
		{"escape-time", "0"},
		{"status", "on"},
		{"status-position", "top"},
	}
	for _, o := range tmuxOpts {
		optCmd := exec.Command("tmux", "set-option", "-t", sessionName, o.opt, o.val)
		if err := ex.Run(optCmd); err != nil {
			// Non-fatal: proceed even if an option fails to apply.
			_ = err
		}
	}
	_ = InstallFocusBindings(ex, sessionName)

	return layout, false, nil
}

func popupExecutableFromTUICommand(tuiCommand string) string {
	fields := strings.Fields(strings.TrimSpace(tuiCommand))
	if len(fields) == 0 {
		return "kas"
	}
	return fields[0]
}

func workspaceShellBootstrap(userShell string) [3]string {
	banner := strings.Join([]string{
		"██╗  ██╗ █████╗ ███████╗███╗   ███╗ ██████╗ ███████╗",
		"██║ ██╔╝██╔══██╗██╔════╝████╗ ████║██╔═══██╗██╔════╝",
		"█████╔╝ ███████║███████╗██╔████╔██║██║   ██║███████╗",
		"██╔═██╗ ██╔══██║╚════██║██║╚██╔╝██║██║   ██║╚════██║",
		"██║  ██╗██║  ██║███████║██║ ╚═╝ ██║╚██████╔╝███████║",
		"╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝ ╚═════╝ ╚══════╝",
		"",
		"multi-agent orchestration IDE",
		"",
	}, "\n")
	script := fmt.Sprintf("clear; printf '%%b' %s; exec %q -i", strconv.Quote(banner), userShell)
	return [3]string{"/bin/sh", "-lc", script}
}

func visibleNavPaneTarget(sessionName string) string {
	return sessionName + ":0.0"
}

func visibleRightPaneTarget(sessionName string) string {
	return sessionName + ":0.1"
}

// AttachMainLayout attaches the calling terminal to the kasmos layout session.
// If the caller is already inside a tmux session, switch-client is used to avoid
// nesting; otherwise attach-session is used to start a new tmux client.
func AttachMainLayout(ex cmd.Executor, sessionName string) error {
	if os.Getenv("TMUX") != "" {
		// Inside an existing tmux session: switch the current client rather than
		// opening a nested attach, regardless of whether it is the kasmos session
		// or an unrelated one.
		switchCmd := exec.Command("tmux", "switch-client", "-t", sessionName)
		if err := ex.Run(switchCmd); err != nil {
			return fmt.Errorf("attach main layout: switch-client: %w", err)
		}
		return nil
	}

	// Outside tmux: attach as a new client.
	attachCmd := exec.Command("tmux", "attach-session", "-t", sessionName)
	attachCmd.Stdin = os.Stdin
	attachCmd.Stdout = os.Stdout
	attachCmd.Stderr = os.Stderr
	if err := ex.Run(attachCmd); err != nil {
		return fmt.Errorf("attach main layout: attach-session: %w", err)
	}
	return nil
}

// FocusPane moves the tmux focus to the given pane ID.
func FocusPane(ex cmd.Executor, paneID string) error {
	selectCmd := exec.Command("tmux", "select-pane", "-t", paneID)
	if err := ex.Run(selectCmd); err != nil {
		return fmt.Errorf("focus pane %s: %w", paneID, err)
	}
	return nil
}

// FocusWorkspacePane selects the visible right pane in the given tmux layout
// session, regardless of whether it currently shows the workspace shell or a
// swapped-in agent session.
func FocusWorkspacePane(ex cmd.Executor, sessionName string) error {
	return ex.Run(exec.Command("tmux", "select-pane", "-t", visibleRightPaneTarget(sessionName)))
}

// FocusNavPane selects the visible left nav pane in the given tmux layout session.
func FocusNavPane(ex cmd.Executor, sessionName string) error {
	return ex.Run(exec.Command("tmux", "select-pane", "-t", visibleNavPaneTarget(sessionName)))
}

// InstallFocusBindings installs the global tmux bindings for the outer kasmos layout.
//
// C-Space toggles focus between panes; C-f focuses the right pane; C-n opens the
// new-plan popup; C-g opens the spawn-agent popup.
func InstallFocusBindings(ex cmd.Executor, sessionName string) error {
	toggleScript := fmt.Sprintf(
		`cur=$(tmux display-message -p '#{pane_id}'); `+
			`rp=$(tmux display-message -p -t '%[1]s:0.1' '#{pane_id}' 2>/dev/null); `+
			`if [ "$cur" = "$rp" ]; then tmux select-pane -t '%[1]s:0.0'; else tmux select-pane -t '%[1]s:0.1'; fi`,
		sessionName,
	)
	popupScript := func(title, subcmd string) string {
		return fmt.Sprintf(
			`repo=$(tmux show-environment -t '%[1]s' KASMOS_REPO_ROOT 2>/dev/null | cut -d= -f2); `+
				`exe=$(tmux show-environment -t '%[1]s' KASMOS_EXECUTABLE 2>/dev/null | cut -d= -f2); `+
				`if [ -z "$repo" ]; then repo="$PWD"; fi; `+
				`if [ -z "$exe" ]; then exe='kas'; fi; `+
				`tmux display-popup -E -w 80%% -h 80%% -T '%[2]s' -d "$repo" "$exe popup %[3]s"`,
			sessionName, title, subcmd,
		)
	}
	bindings := [][]string{
		{"bind-key", "-n", "C-Space", "run-shell", toggleScript},
		{"bind-key", "-n", "C-f", "select-pane", "-t", visibleRightPaneTarget(sessionName)},
		{"bind-key", "-n", "C-n", "run-shell", popupScript("new plan", "new-plan")},
		{"bind-key", "-n", "C-g", "run-shell", popupScript("spawn agent", "spawn-agent")},
	}
	for _, binding := range bindings {
		if err := ex.Run(exec.Command("tmux", binding...)); err != nil {
			return err
		}
	}
	return nil
}

// readLayoutEnv reads KASMOS_NAV_PANE and KASMOS_WORKSPACE_PANE from the given
// tmux session's environment to reconstruct a Layout for an already-running session.
func readLayoutEnv(ex cmd.Executor, sessionName string) (Layout, error) {
	navPane, err := showSessionEnvVar(ex, sessionName, EnvNavPane)
	if err != nil {
		return Layout{}, err
	}
	workspacePane, err := showSessionEnvVar(ex, sessionName, EnvWorkspacePane)
	if err != nil {
		return Layout{}, err
	}
	return Layout{
		SessionName:     sessionName,
		WindowTarget:    sessionName + ":0",
		NavPaneID:       navPane,
		WorkspacePaneID: workspacePane,
	}, nil
}

// ApplyStatusBar updates the tmux status bar for the named session by issuing
// set-option calls for status-left, status-right, and the one-time style options
// (status-style, status-left-length, status-right-length).
//
// It is safe to call repeatedly — tmux silently accepts duplicate set-option calls
// so unchanged values are effectively no-ops. The caller is responsible for
// skipping unchanged renders to avoid unnecessary subprocess spawns.
//
// Errors from individual set-option calls do not abort the loop; the first error
// encountered is returned so callers can toast it if desired.
func ApplyStatusBar(ex cmd.Executor, sessionName string, render StatusBarRender) error {
	opts := []struct{ opt, val string }{
		{"status-style", "bg=default,fg=default"},
		{"status-left-length", "120"},
		{"status-right-length", "80"},
		{"status-left", render.Left},
		{"status-right", render.Right},
	}
	var firstErr error
	for _, o := range opts {
		c := exec.Command("tmux", "set-option", "-t", sessionName, o.opt, o.val)
		if err := ex.Run(c); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("apply status bar: set-option %s: %w", o.opt, err)
		}
	}
	return firstErr
}

// showSessionEnvVar runs `tmux show-environment -t <session> <var>` and parses
// the output (format: "VAR=value\n") returning the value portion.
func showSessionEnvVar(ex cmd.Executor, sessionName, envVar string) (string, error) {
	showCmd := exec.Command("tmux", "show-environment", "-t", sessionName, envVar)
	out, err := ex.Output(showCmd)
	if err != nil {
		return "", fmt.Errorf("show-environment %s: %w", envVar, err)
	}
	line := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(line, '='); idx >= 0 {
		return line[idx+1:], nil
	}
	return "", fmt.Errorf("show-environment %s: unexpected output: %q", envVar, line)
}
