//go:build windows

package git

import (
	"os/exec"
	"syscall"
)

// hideWindow prevents Windows from allocating a console window for the git
// child process. Without this, a GUI-subsystem parent (the desktop app) makes
// every git invocation flash a cmd window. Output is captured via pipes, so no
// console is needed.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
