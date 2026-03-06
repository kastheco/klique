// Package headless provides an execution backend that runs agent programs
// directly as child processes without a tmux session. All interactive
// operations (Attach, SendKeys, SetDetachedSize, permission responses) return
// ErrInteractiveOnly.
package headless

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kastheco/kasmos/session/tmux"
)

// ErrInteractiveOnly is returned when the caller requests an operation that
// requires a live terminal (tmux). Exported so session.ErrInteractiveOnly can
// alias it without a circular import.
var ErrInteractiveOnly = errors.New("interactive operation requires tmux execution")

var whiteSpaceRegex = regexp.MustCompile(`\s+`)

// Session is a headless execution backend that runs the agent program directly
// via exec.Cmd without tmux. Output is captured in an in-memory buffer and
// appended to a log file under <workDir>/.kasmos/logs/.
type Session struct {
	name            string
	sanitizedName   string
	program         string
	skipPermissions bool

	agentType     string
	initialPrompt string
	taskNumber    int
	waveNumber    int
	peerCount     int
	sessionTitle  string
	titleFunc     func(workDir string, beforeStart time.Time, title string)

	// mu protects cmd, buf, done, lastContent.
	mu          sync.Mutex
	cmd         *exec.Cmd
	buf         bytes.Buffer
	done        chan struct{} // closed when the child process exits
	lastContent string        // previous content snapshot for HasUpdated tracking
}

// sanitizeName converts a human-readable session name into a safe identifier.
// Whitespace is stripped and dots are replaced with underscores.
func sanitizeName(name string) string {
	s := whiteSpaceRegex.ReplaceAllString(name, "")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

// New constructs a new unstarted headless Session.
func New(name, program string, skipPermissions bool) *Session {
	return &Session{
		name:            name,
		sanitizedName:   sanitizeName(name),
		program:         program,
		skipPermissions: skipPermissions,
	}
}

// Configuration (builder-style, called before Start)

// SetAgentType stores the agent type identifier (informational; not forwarded to CLI).
func (s *Session) SetAgentType(agentType string) { s.agentType = strings.TrimSpace(agentType) }

// SetInitialPrompt stores the initial prompt (not supported for headless programs).
func (s *Session) SetInitialPrompt(prompt string) { s.initialPrompt = prompt }

// SetTaskEnv sets the task/wave/peer identity injected as env vars at Start().
func (s *Session) SetTaskEnv(task, wave, peers int) {
	s.taskNumber = task
	s.waveNumber = wave
	s.peerCount = peers
}

// SetSessionTitle stores the session title (no-op for headless).
func (s *Session) SetSessionTitle(title string) { s.sessionTitle = title }

// SetTitleFunc stores the title callback (no-op for headless).
func (s *Session) SetTitleFunc(fn func(workDir string, beforeStart time.Time, title string)) {
	s.titleFunc = fn
}

// GetSanitizedName returns the sanitized session name used for log file naming.
func (s *Session) GetSanitizedName() string { return s.sanitizedName }

// Write implements io.Writer so the Session can be used as a combined stdout/stderr
// destination for the child process. Protected by mu.
func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

// Start launches the program as a child process in workDir.
// Output is captured to both the in-memory buffer and a log file under
// <workDir>/.kasmos/logs/<sanitizedName>.log.
// KASMOS_MANAGED=1 and (when TaskNumber > 0) KASMOS_TASK/KASMOS_WAVE/KASMOS_PEERS
// are prepended to the process environment.
func (s *Session) Start(workDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil {
		return fmt.Errorf("headless session already started")
	}

	// Ensure the log directory exists.
	logDir := filepath.Join(workDir, ".kasmos", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	logPath := filepath.Join(logDir, s.sanitizedName+".log")

	// Parse the program string into argv.
	parts := strings.Fields(s.program)
	if len(parts) == 0 {
		return fmt.Errorf("empty program")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = workDir

	// Build the child's environment.
	env := os.Environ()
	env = append(env, "KASMOS_MANAGED=1")
	if s.taskNumber > 0 {
		env = append(env,
			fmt.Sprintf("KASMOS_TASK=%d", s.taskNumber),
			fmt.Sprintf("KASMOS_WAVE=%d", s.waveNumber),
			fmt.Sprintf("KASMOS_PEERS=%d", s.peerCount),
		)
	}
	cmd.Env = env

	// Open the log file for appending.
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Capture combined stdout+stderr to both the in-memory buffer and the log file.
	combined := io.MultiWriter(s, lf)
	cmd.Stdout = combined
	cmd.Stderr = combined

	if err := cmd.Start(); err != nil {
		lf.Close()
		return fmt.Errorf("failed to start headless session: %w", err)
	}

	s.cmd = cmd
	s.done = make(chan struct{})

	// Reap the child process in the background and signal done on exit.
	go func() {
		defer close(s.done)
		_ = cmd.Wait()
		lf.Close()
	}()

	return nil
}

// Restore is a no-op for headless sessions (no reconnection to an existing session).
func (s *Session) Restore() error { return nil }

// Close stops the child process idempotently. Safe to call multiple times.
func (s *Session) Close() error {
	s.mu.Lock()
	cmd := s.cmd
	done := s.done
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// Already exited.
	if done != nil {
		select {
		case <-done:
			return nil
		default:
		}
	}
	return cmd.Process.Kill()
}

// DoesSessionExist reports whether the child process is currently running.
func (s *Session) DoesSessionExist() bool {
	s.mu.Lock()
	cmd := s.cmd
	done := s.done
	s.mu.Unlock()

	if cmd == nil || done == nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}

// CapturePaneContent returns the current in-memory output buffer as a string.
func (s *Session) CapturePaneContent() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String(), nil
}

// CapturePaneContentWithOptions returns the same content as CapturePaneContent.
// The start/end arguments are ignored (not meaningful for headless).
func (s *Session) CapturePaneContentWithOptions(_, _ string) (string, error) {
	return s.CapturePaneContent()
}

// HasUpdated reports whether new output has been produced since the last call.
// hasPrompt is always false for headless sessions.
func (s *Session) HasUpdated() (updated bool, hasPrompt bool) {
	updated, _, _, _ = s.HasUpdatedWithContent()
	return updated, false
}

// HasUpdatedWithContent is like HasUpdated but also returns the current content
// and a captured flag (always true for headless).
func (s *Session) HasUpdatedWithContent() (updated bool, hasPrompt bool, content string, captured bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content = s.buf.String()
	updated = content != s.lastContent
	s.lastContent = content
	return updated, false, content, true
}

// GetPanePID returns the PID of the child process.
func (s *Session) GetPanePID() (int, error) {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return 0, fmt.Errorf("process not started")
	}
	return cmd.Process.Pid, nil
}

// --- Interactive operations (not supported by headless) -----------------

// Attach returns ErrInteractiveOnly — headless sessions do not support
// interactive terminal attachment.
func (s *Session) Attach() (chan struct{}, error) {
	return nil, ErrInteractiveOnly
}

// SendKeys returns ErrInteractiveOnly.
func (s *Session) SendKeys(_ string) error { return ErrInteractiveOnly }

// TapEnter returns ErrInteractiveOnly.
func (s *Session) TapEnter() error { return ErrInteractiveOnly }

// SendPermissionResponse returns ErrInteractiveOnly.
func (s *Session) SendPermissionResponse(_ tmux.PermissionChoice) error {
	return ErrInteractiveOnly
}

// DetachSafely is a no-op for headless sessions (nothing to detach from).
func (s *Session) DetachSafely() error { return nil }

// SetDetachedSize returns ErrInteractiveOnly.
func (s *Session) SetDetachedSize(_, _ int) error { return ErrInteractiveOnly }
