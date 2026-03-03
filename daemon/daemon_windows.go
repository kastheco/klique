//go:build windows

package daemon

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// getSysProcAttr returns Windows-specific attributes that detach the child
// process from the parent console and place it in a new process group.
func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
}
