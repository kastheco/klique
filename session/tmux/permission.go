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
// to respond to its permission dialog. opencode presents three choices in this order:
//
//	Allow once | Allow always | Reject
//
// The first option ("Allow once") is already highlighted by default. Arrow keys
// navigate horizontally; Enter confirms the highlighted option.
//
// Key sequences:
//   - AllowOnce:   Enter (first option is already selected)
//   - AllowAlways: Right → Enter → Enter (navigate to second option, confirm)
//   - Reject:      Right → Right → Enter (navigate to third option, confirm)
func (t *TmuxSession) SendPermissionResponse(choice PermissionChoice) error {
	switch choice {
	case PermissionAllowOnce:
		// "Allow once" is the first (default) option — just confirm with Enter.
		return t.cmdExec.Run(exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Enter"))
	case PermissionAllowAlways:
		// Navigate right to "Allow always" (second option), then confirm twice.
		if err := t.cmdExec.Run(exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Right")); err != nil {
			return err
		}
		if err := t.cmdExec.Run(exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Enter")); err != nil {
			return err
		}
		return t.cmdExec.Run(exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Enter"))
	case PermissionReject:
		// Navigate right twice to "Reject" (third option), then confirm.
		if err := t.cmdExec.Run(exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Right")); err != nil {
			return err
		}
		if err := t.cmdExec.Run(exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Right")); err != nil {
			return err
		}
		return t.cmdExec.Run(exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Enter"))
	default:
		// Unknown choice — default to rejecting for safety.
		return t.cmdExec.Run(exec.Command("tmux", "send-keys", "-t", t.sanitizedName, "Right", "Right", "Enter"))
	}
}
