//go:build !windows

package cmd

import "syscall"

// daemonSysProcAttr returns Unix-specific process attributes that detach the
// child daemon process from the parent by placing it in a new session.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
