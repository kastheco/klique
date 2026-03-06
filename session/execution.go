package session

import (
	"errors"
	"time"

	"github.com/kastheco/kasmos/session/headless"
	"github.com/kastheco/kasmos/session/tmux"
)

// ExecutionMode determines how an instance's agent process is hosted.
type ExecutionMode string

const (
	// ExecutionModeTmux uses tmux as the process host (default).
	ExecutionModeTmux ExecutionMode = "tmux"
	// ExecutionModeHeadless runs the agent directly as an exec.Cmd without tmux.
	ExecutionModeHeadless ExecutionMode = "headless"
)

// ErrInteractiveOnly is returned by headless sessions when an interactive
// operation (e.g. Attach, SendKeys) is requested.
var ErrInteractiveOnly = errors.New("interactive operation requires tmux execution")

// ExecutionSession abstracts the process host (tmux or headless) behind a common interface.
// All methods are equivalent to those on *tmux.TmuxSession; headless implementations
// return ErrInteractiveOnly for operations that require a live terminal.
type ExecutionSession interface {
	// Lifecycle
	Start(workDir string) error
	Restore() error
	Close() error
	DoesSessionExist() bool

	// I/O
	SendKeys(keys string) error
	TapEnter() error
	SendPermissionResponse(choice tmux.PermissionChoice) error
	CapturePaneContent() (string, error)
	CapturePaneContentWithOptions(start, end string) (string, error)
	HasUpdated() (bool, bool)
	HasUpdatedWithContent() (bool, bool, string, bool)
	GetPanePID() (int, error)

	// Attach/Detach
	Attach() (chan struct{}, error)
	DetachSafely() error
	SetDetachedSize(width, height int) error

	// Accessors
	GetSanitizedName() string

	// Configuration (builder-style, called before Start)
	SetAgentType(agentType string)
	SetInitialPrompt(prompt string)
	SetTaskEnv(taskNumber, waveNumber, peerCount int)
	SetSessionTitle(title string)
	SetTitleFunc(fn func(workDir string, beforeStart time.Time, title string))
}

// progressReporter is optionally implemented by session types that support
// a progress callback hook. The instance layer uses a type assertion to set
// it without requiring all ExecutionSession implementations to support it.
type progressReporter interface {
	SetProgressFunc(fn func(int, string))
}

// NormalizeExecutionMode returns ExecutionModeHeadless when mode is
// ExecutionModeHeadless, and ExecutionModeTmux for all other values including "".
func NormalizeExecutionMode(mode ExecutionMode) ExecutionMode {
	if mode == ExecutionModeHeadless {
		return ExecutionModeHeadless
	}
	return ExecutionModeTmux
}

// NewExecutionSession constructs the appropriate ExecutionSession for the given mode.
func NewExecutionSession(mode ExecutionMode, name, program string, skipPermissions bool) ExecutionSession {
	switch NormalizeExecutionMode(mode) {
	case ExecutionModeHeadless:
		return headless.New(name, program, skipPermissions)
	default:
		return newTmuxExecutionSession(name, program, skipPermissions)
	}
}

// --- tmux adapter -------------------------------------------------------

// tmuxExecutionSession wraps *tmux.TmuxSession and implements ExecutionSession.
// It also satisfies progressReporter so the instance layer can set ProgressFunc
// without depending on the concrete *TmuxSession type.
type tmuxExecutionSession struct {
	s *tmux.TmuxSession
}

func newTmuxExecutionSession(name, program string, skipPermissions bool) *tmuxExecutionSession {
	return &tmuxExecutionSession{s: tmux.NewTmuxSession(name, program, skipPermissions)}
}

// Lifecycle
func (w *tmuxExecutionSession) Start(workDir string) error { return w.s.Start(workDir) }
func (w *tmuxExecutionSession) Restore() error             { return w.s.Restore() }
func (w *tmuxExecutionSession) Close() error               { return w.s.Close() }
func (w *tmuxExecutionSession) DoesSessionExist() bool     { return w.s.DoesSessionExist() }

// I/O
func (w *tmuxExecutionSession) SendKeys(keys string) error { return w.s.SendKeys(keys) }
func (w *tmuxExecutionSession) TapEnter() error            { return w.s.TapEnter() }
func (w *tmuxExecutionSession) SendPermissionResponse(choice tmux.PermissionChoice) error {
	return w.s.SendPermissionResponse(choice)
}
func (w *tmuxExecutionSession) CapturePaneContent() (string, error) {
	return w.s.CapturePaneContent()
}
func (w *tmuxExecutionSession) CapturePaneContentWithOptions(start, end string) (string, error) {
	return w.s.CapturePaneContentWithOptions(start, end)
}
func (w *tmuxExecutionSession) HasUpdated() (bool, bool) { return w.s.HasUpdated() }
func (w *tmuxExecutionSession) HasUpdatedWithContent() (bool, bool, string, bool) {
	return w.s.HasUpdatedWithContent()
}
func (w *tmuxExecutionSession) GetPanePID() (int, error) { return w.s.GetPanePID() }

// Attach/Detach
func (w *tmuxExecutionSession) Attach() (chan struct{}, error) { return w.s.Attach() }
func (w *tmuxExecutionSession) DetachSafely() error            { return w.s.DetachSafely() }
func (w *tmuxExecutionSession) SetDetachedSize(width, height int) error {
	return w.s.SetDetachedSize(width, height)
}

// Accessors
func (w *tmuxExecutionSession) GetSanitizedName() string { return w.s.GetSanitizedName() }

// Configuration
func (w *tmuxExecutionSession) SetAgentType(agentType string)  { w.s.SetAgentType(agentType) }
func (w *tmuxExecutionSession) SetInitialPrompt(prompt string) { w.s.SetInitialPrompt(prompt) }
func (w *tmuxExecutionSession) SetTaskEnv(taskNumber, waveNumber, peerCount int) {
	w.s.SetTaskEnv(taskNumber, waveNumber, peerCount)
}
func (w *tmuxExecutionSession) SetSessionTitle(title string) { w.s.SetSessionTitle(title) }
func (w *tmuxExecutionSession) SetTitleFunc(fn func(workDir string, beforeStart time.Time, title string)) {
	w.s.SetTitleFunc(fn)
}

// SetProgressFunc implements progressReporter, allowing the instance layer to
// inject a progress hook without knowing the concrete TmuxSession type.
func (w *tmuxExecutionSession) SetProgressFunc(fn func(int, string)) {
	w.s.ProgressFunc = fn
}
