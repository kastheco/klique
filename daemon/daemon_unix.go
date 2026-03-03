//go:build !windows

package daemon

import "syscall"

// getSysProcAttr returns Unix-specific attributes that detach the child
// process from the parent by placing it in a new session.
func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
