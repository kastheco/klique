package tmux

import "os/exec"

// PermissionChoice represents the user's response to an opencode permission dialog.
type PermissionChoice int

const (
	// PermissionAllowAlways grants permission permanently (cached for future prompts).
	PermissionAllowAlways PermissionChoice = iota
	// PermissionAllowOnce grants permission for this single invocation only.
	PermissionAllowOnce
	// PermissionReject denies the requested permission.
	PermissionReject
)

// SendPermissionResponse sends the appropriate key sequence to the opencode pane
// to respond to its permission dialog. opencode uses a TUI selector navigated with
// arrow keys and confirmed with Enter; Escape dismisses/rejects.
//
// Key sequences:
//   - AllowAlways: Enter (first option is "allow always" by default)
//   - AllowOnce:   Right then Enter (navigate to second option)
//   - Reject:      Escape
func (t *TmuxSession) SendPermissionResponse(choice PermissionChoice) error {
	var args []string
	switch choice {
	case PermissionAllowAlways:
		// "allow always" is the first option — just confirm with Enter.
		args = []string{"send-keys", "-t", t.sanitizedName, "Enter"}
	case PermissionAllowOnce:
		// Navigate right to "allow once", then confirm.
		args = []string{"send-keys", "-t", t.sanitizedName, "Right", "Enter"}
	case PermissionReject:
		// Escape dismisses the dialog and rejects the permission.
		args = []string{"send-keys", "-t", t.sanitizedName, "Escape"}
	default:
		args = []string{"send-keys", "-t", t.sanitizedName, "Escape"}
	}
	return t.cmdExec.Run(exec.Command("tmux", args...))
}
