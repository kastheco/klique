package session

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/kastheco/kasmos/session/headless"
	"github.com/kastheco/kasmos/session/tmux"
)

type ExecutionMode = string

const (
	ExecutionModeTmux     ExecutionMode = "tmux"
	ExecutionModeHeadless ExecutionMode = "headless"
)

var ErrInteractiveOnly = errors.New("interactive operation requires tmux execution")

//go:generate mockery --name ExecutionSession
type ExecutionSession interface {
	SetProgressFunc(func(stage int, desc string))
	Start(workDir string) error
	Restore() error
	Close() error
	DoesSessionExist() bool
	GetPTY() *os.File
	SendKeys(keys string) error
	TapEnter() error
	SendPermissionResponse(choice tmux.PermissionChoice) error
	CapturePaneContent() (string, error)
	CapturePaneContentWithOptions(start, end string) (string, error)
	HasUpdated() (bool, bool)
	HasUpdatedWithContent() (bool, bool, string, bool)
	GetPanePID() (int, error)
	Attach() (chan struct{}, error)
	DetachSafely() error
	SetDetachedSize(width, height int) error
	GetSanitizedName() string
	SetAgentType(agentType string)
	SetInitialPrompt(prompt string)
	SetTaskEnv(taskNumber, waveNumber, peerCount int)
	SetSessionTitle(title string)
	SetTitleFunc(fn func(workDir string, beforeStart time.Time, title string))
}

// NormalizeExecutionMode returns the canonical mode for execution scheduling.
func NormalizeExecutionMode(mode ExecutionMode) ExecutionMode {
	switch strings.TrimSpace(mode) {
	case "", ExecutionModeTmux:
		return ExecutionModeTmux
	case ExecutionModeHeadless:
		return ExecutionModeHeadless
	default:
		return ExecutionModeTmux
	}
}

// NewExecutionSession constructs the backend session implementation for the given mode.
func NewExecutionSession(mode ExecutionMode, name, program string, skipPermissions bool) ExecutionSession {
	mode = NormalizeExecutionMode(mode)
	if mode == ExecutionModeHeadless {
		return headless.NewSession(name, program, skipPermissions)
	}
	return tmux.NewTmuxSession(name, program, skipPermissions)
}
