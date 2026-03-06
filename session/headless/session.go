package headless

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/kastheco/kasmos/session/tmux"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
var sanitizeRegex = regexp.MustCompile(`\s+`)

type outputWriter struct {
	mu     sync.Mutex
	buffer *bytes.Buffer
}

func (w *outputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(p)
}

var ErrInteractiveOnly = errors.New("interactive operation requires tmux execution")

// Session is a lightweight non-interactive execution backend.
type Session struct {
	name     string
	program  string
	skipPerm bool

	agentType     string
	initialPrompt string
	taskNumber    int
	waveNumber    int
	peerCount     int

	progressFn func(int, string)

	cmd      *exec.Cmd
	ctx      context.Context
	cancel   context.CancelFunc
	waitDone bool
	waitErr  error
	waitMu   sync.Mutex
	closedMu sync.Mutex
	closed   bool

	logFile     *os.File
	logFilePath string

	outputMu sync.RWMutex
	output   outputWriter

	preview   string
	previewMu sync.Mutex

	started time.Time
}

// NewSession creates a headless execution session.
func NewSession(name string, program string, skipPermissions bool) *Session {
	return &Session{
		name:     sanitizeSessionName(name),
		program:  strings.TrimSpace(program),
		skipPerm: skipPermissions,
		output: outputWriter{
			buffer: &bytes.Buffer{},
		},
	}
}

func sanitizeSessionName(name string) string {
	name = sanitizeRegex.ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, ".", "_")
	return "kas_" + name
}

// SetProgressFunc stores a startup progress callback.
func (s *Session) SetProgressFunc(fn func(int, string)) {
	s.progressFn = fn
}

// Start runs the command as a background process under the given working directory.
func (s *Session) Start(workDir string) error {
	if s.DoesSessionExist() {
		return s.Restore()
	}

	if s.progressFn != nil {
		s.progressFn(1, "preparing")
	}

	command := s.program
	if s.skipPerm && strings.HasSuffix(s.program, "claude") {
		command += " --dangerously-skip-permissions"
	}
	if s.taskNumber > 0 {
		command = fmt.Sprintf("KASMOS_TASK=%d KASMOS_WAVE=%d KASMOS_PEERS=%d %s", s.taskNumber, s.waveNumber, s.peerCount, command)
	}
	if s.initialPrompt != "" {
		command = command + " " + shellQuote(s.initialPrompt)
	}

	logFilePath := filepath.Join(workDir, ".kasmos", "logs", s.name+".log")
	if err := os.MkdirAll(filepath.Dir(logFilePath), 0o755); err != nil {
		return fmt.Errorf("error creating log directory: %w", err)
	}

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("error creating session log: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "KASMOS_MANAGED=1")
	cmd.Stdout = io.MultiWriter(logFile, &s.output)
	cmd.Stderr = io.MultiWriter(logFile, &s.output)

	if err := cmd.Start(); err != nil {
		s.logFile = logFile
		return fmt.Errorf("error starting headless session: %w", err)
	}

	s.cmd = cmd
	s.resetWaitState()
	s.logFile = logFile
	s.logFilePath = logFilePath
	s.started = time.Now()

	if s.progressFn != nil {
		s.progressFn(2, "running")
	}

	go func() {
		_ = s.waitForExit()
		if s.progressFn != nil {
			s.progressFn(3, "stopped")
		}
	}()

	return nil
}

// Restore returns nil for a running process and an error otherwise.
func (s *Session) Restore() error {
	if !s.DoesSessionExist() {
		return fmt.Errorf("headless session not running")
	}
	return nil
}

// Close stops the process and cleans up resources.
func (s *Session) Close() error {
	s.closedMu.Lock()
	if s.closed {
		s.closedMu.Unlock()
		return nil
	}
	s.closed = true
	s.closedMu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	var errs []error
	if s.cmd != nil && s.cmd.Process != nil {
		if err := s.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			errs = append(errs, fmt.Errorf("error killing headless process: %w", err))
		}
		if err := s.waitForExit(); err != nil && !isBenignWaitError(err) {
			errs = append(errs, err)
		}
	}

	if s.logFile != nil {
		if err := s.logFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing log file: %w", err))
		}
		s.logFile = nil
	}

	return errors.Join(errs...)
}

func (s *Session) waitForExit() error {
	s.waitMu.Lock()
	if s.waitDone {
		waitErr := s.waitErr
		s.waitMu.Unlock()
		return waitErr
	}
	s.waitDone = true
	s.waitMu.Unlock()

	waitErr := s.cmd.Wait()

	s.waitMu.Lock()
	s.waitErr = waitErr
	s.waitMu.Unlock()

	return waitErr
}

func (s *Session) resetWaitState() {
	s.waitMu.Lock()
	s.waitDone = false
	s.waitErr = nil
	s.waitMu.Unlock()
}

func isBenignWaitError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "wait was already called") || strings.Contains(msg, "no child processes")
}

// DoesSessionExist reports whether the underlying process is still alive.
func (s *Session) DoesSessionExist() bool {
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	if s.cmd.ProcessState != nil {
		return false
	}
	return true
}

// SendKeys is interactive-only for headless execution.
func (s *Session) SendKeys(_ string) error { return ErrInteractiveOnly }

// TapEnter is interactive-only for headless execution.
func (s *Session) TapEnter() error { return ErrInteractiveOnly }

// SendPermissionResponse is interactive-only for headless execution.
func (s *Session) SendPermissionResponse(_ tmux.PermissionChoice) error { return ErrInteractiveOnly }

// CapturePaneContent returns the in-memory output captured so far.
func (s *Session) CapturePaneContent() (string, error) {
	s.output.mu.Lock()
	defer s.output.mu.Unlock()
	return s.output.buffer.String(), nil
}

// CapturePaneContentWithOptions returns all available output.
func (s *Session) CapturePaneContentWithOptions(_ string, _ string) (string, error) {
	return s.CapturePaneContent()
}

// HasUpdated reports output-change status and prompt detection.
func (s *Session) HasUpdated() (updated bool, hasPrompt bool) {
	updated, hasPrompt, _, _ = s.HasUpdatedWithContent()
	return
}

// HasUpdatedWithContent compares against the last captured snapshot and reports prompt detection.
func (s *Session) HasUpdatedWithContent() (updated bool, hasPrompt bool, content string, captured bool) {
	content, err := s.CapturePaneContent()
	if err != nil {
		return false, false, "", false
	}

	plain := ansiRe.ReplaceAllString(content, "")
	hasPrompt = isPromptText(plain)

	s.previewMu.Lock()
	defer s.previewMu.Unlock()
	if content == s.preview {
		updated = false
	} else {
		updated = true
		s.preview = content
	}

	return updated, hasPrompt, content, true
}

func isPromptText(content string) bool {
	if strings.Contains(content, "No, and tell Claude what to do differently") {
		return true
	}
	if strings.Contains(content, "Ask anything") {
		return true
	}
	return false
}

// GetPanePID returns the process ID of the managed command.
func (s *Session) GetPanePID() (int, error) {
	if !s.DoesSessionExist() || s.cmd.Process == nil {
		return 0, fmt.Errorf("headless session not running")
	}
	return s.cmd.Process.Pid, nil
}

// Attach is unsupported for headless execution.
func (s *Session) Attach() (chan struct{}, error) { return nil, ErrInteractiveOnly }

// GetPTY is not available for headless execution.
func (s *Session) GetPTY() *os.File { return nil }

// Detach is unsupported for headless execution.
func (s *Session) Detach() {}

// DetachSafely is unsupported for headless execution.
func (s *Session) DetachSafely() error { return ErrInteractiveOnly }

// SetDetachedSize is unsupported for headless execution.
func (s *Session) SetDetachedSize(_ int, _ int) error { return ErrInteractiveOnly }

// GetSanitizedName returns the generated session name used for logs.
func (s *Session) GetSanitizedName() string { return s.name }

// SetAgentType stores the agent type for context.
func (s *Session) SetAgentType(agentType string) { s.agentType = strings.TrimSpace(agentType) }

// SetInitialPrompt stores startup prompt text.
func (s *Session) SetInitialPrompt(prompt string) { s.initialPrompt = prompt }

// SetTaskEnv stores task/peer identities.
func (s *Session) SetTaskEnv(taskNumber, waveNumber, peerCount int) {
	s.taskNumber = taskNumber
	s.waveNumber = waveNumber
	s.peerCount = peerCount
}

// SetSessionTitle is a no-op for headless execution.
func (s *Session) SetSessionTitle(_ string) {}

// SetTitleFunc is a no-op for headless execution.
func (s *Session) SetTitleFunc(_ func(workDir string, beforeStart time.Time, title string)) {}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
