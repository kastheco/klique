package tmux

import (
	"os"
	"time"
)

// Session defines the contract for a managed terminal multiplexer session.
// The instance layer depends on this interface, not the concrete TmuxSession.
type Session interface {
	// Lifecycle
	Start(workDir string) error
	Restore() error
	Close() error
	DoesSessionExist() bool

	// I/O
	SendKeys(keys string) error
	TapEnter() error
	TapRight() error
	SendPermissionResponse(choice PermissionChoice) error
	CapturePaneContent() (string, error)
	CapturePaneContentWithOptions(start, end string) (string, error)
	HasUpdated() (updated bool, hasPrompt bool)
	HasUpdatedWithContent() (updated bool, hasPrompt bool, content string, captured bool)
	GetPanePID() (int, error)

	// Attach/Detach
	Attach() (chan struct{}, error)
	Detach()
	DetachSafely() error
	SetDetachedSize(width, height int) error

	// Accessors
	GetPTY() *os.File
	GetSanitizedName() string

	// Configuration (builder-style, called before Start)
	SetAgentType(agentType string)
	SetInitialPrompt(prompt string)
	SetTaskEnv(taskNumber, waveNumber, peerCount int)
	SetSessionTitle(title string)
	SetTitleFunc(fn func(workDir string, beforeStart time.Time, title string))
	NewReset(name, program string, skipPermissions bool) Session
}

// PermissionChoice represents the user's response to a permission prompt.
type PermissionChoice int

const (
	PermissionAllowOnce PermissionChoice = iota
	PermissionAllowAlways
	PermissionReject
)
