package taskfsm

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// DefaultNotifyFunc is the default notification backend used by NewNotifyHook.
// It must be wired at application startup when a fully-functional notification
// backend is desired, e.g.:
//
//	taskfsm.DefaultNotifyFunc = session.SendNotification
//
// A direct import of the session package is not possible here because of an
// existing import cycle: session → cmd → config/taskfsm.
// The default implementation below mirrors session.SendNotification so that
// the hook works out-of-the-box without external wiring.
var DefaultNotifyFunc func(title, body string) = sendDesktopNotification

// sendDesktopNotification is a self-contained platform-aware notification
// dispatcher that mirrors the behaviour of session.SendNotification without
// importing that package (import cycle: session → cmd → config/taskfsm).
func sendDesktopNotification(title, body string) {
	switch runtime.GOOS {
	case "darwin":
		escaped := func(s string) string {
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, `"`, `\"`)
			return s
		}
		script := `display notification "` + escaped(body) + `" with title "` + escaped(title) + `"`
		_ = exec.Command("osascript", "-e", script).Start()
	case "linux":
		path, err := exec.LookPath("notify-send")
		if err != nil {
			return
		}
		_ = exec.Command(path, title, body).Start()
	}
}

// NotifyHook delivers a desktop notification on each FSM transition.
type NotifyHook struct {
	// notifyFunc is replaceable in tests; defaults to DefaultNotifyFunc.
	notifyFunc func(title, body string)
}

// NewNotifyHook returns a NotifyHook wired to DefaultNotifyFunc.
func NewNotifyHook() *NotifyHook {
	return &NotifyHook{notifyFunc: DefaultNotifyFunc}
}

// Name satisfies HookRunner.
func (n *NotifyHook) Name() string { return "notify" }

// Run formats and fires a desktop notification for the given TransitionEvent.
// Always returns nil — missing OS notification support is already a no-op
// inside the default notify func.
func (n *NotifyHook) Run(_ context.Context, ev TransitionEvent) error {
	title := fmt.Sprintf("kasmos: %s", ev.PlanFile)
	body := fmt.Sprintf("%s -> %s (%s)", ev.FromStatus, ev.ToStatus, ev.Event)
	n.notifyFunc(title, body)
	return nil
}
