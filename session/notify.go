package session

import (
	"os/exec"
	"runtime"
	"strings"
)

// NotificationsEnabled controls whether desktop notifications are sent.
// Set from config at startup.
var NotificationsEnabled = true

// SendNotification fires a desktop notification. The underlying command is
// started but not awaited — callers do not block on OS notification delivery.
func SendNotification(title, body string) {
	if !NotificationsEnabled {
		return
	}
	switch runtime.GOOS {
	case "darwin":
		sendDarwin(title, body)
	case "linux":
		sendLinux(title, body)
	}
}

// sendDarwin delivers a notification via osascript on macOS.
func sendDarwin(title, body string) {
	script := `display notification "` + escapeAppleScript(body) +
		`" with title "` + escapeAppleScript(title) + `"`
	_ = exec.Command("osascript", "-e", script).Start()
}

// sendLinux delivers a notification via notify-send on Linux.
// The call is a no-op when notify-send is not installed.
func sendLinux(title, body string) {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		return
	}
	_ = exec.Command(path, title, body).Start()
}

// escapeAppleScript escapes backslashes and double-quotes for use inside
// an AppleScript string literal.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
