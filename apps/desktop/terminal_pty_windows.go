//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
)

type conPTYSession struct {
	pty    *conpty.ConPty
	handle windows.Handle
}

func startPTYProcess(target, name string, args []string, cols, rows int) (ptySession, error) {
	c, err := conpty.New(cols, rows, 0)
	if err != nil {
		return nil, err
	}
	argv := append([]string{name}, args...)
	_, handle, err := c.Spawn(name, argv, &syscall.ProcAttr{
		Dir: target,
		Env: append(os.Environ(), "TERM=xterm-256color"),
	})
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	return &conPTYSession{pty: c, handle: windows.Handle(handle)}, nil
}

func (s *conPTYSession) Read(p []byte) (int, error)  { return s.pty.Read(p) }
func (s *conPTYSession) Write(p []byte) (int, error) { return s.pty.Write(p) }
func (s *conPTYSession) Close() error {
	err := s.pty.Close()
	if s.handle != 0 {
		if closeErr := windows.CloseHandle(s.handle); err == nil {
			err = closeErr
		}
		s.handle = 0
	}
	return err
}
func (s *conPTYSession) Resize(cols, rows int) error { return s.pty.Resize(cols, rows) }
func (s *conPTYSession) Kill() error {
	if s.handle == 0 {
		return nil
	}
	return windows.TerminateProcess(s.handle, 1)
}
func (s *conPTYSession) Wait() error {
	if s.handle == 0 {
		return nil
	}
	status, err := windows.WaitForSingleObject(s.handle, windows.INFINITE)
	if err != nil {
		return err
	}
	if status != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("unexpected wait status %d", status)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(s.handle, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit status %d", code)
	}
	return nil
}
