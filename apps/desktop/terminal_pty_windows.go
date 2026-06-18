//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
)

type conPTYSession struct {
	pty    *conpty.ConPty
	mu     sync.Mutex
	handle windows.Handle
}

func startPTYProcess(target, name string, args []string, cols, rows int) (ptySession, error) {
	c, err := conpty.New(cols, rows, 0)
	if err != nil {
		return nil, err
	}
	resolvedName, err := exec.LookPath(name)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	argv := append([]string{name}, args...)
	_, handle, err := c.Spawn(resolvedName, argv, &syscall.ProcAttr{
		Dir: target,
		Env: terminalEnv(cols, rows),
	})
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	return &conPTYSession{pty: c, handle: windows.Handle(handle)}, nil
}

func terminalEnv(cols, rows int) []string {
	env := os.Environ()
	set := func(key, value string) {
		prefix := key + "="
		for i, item := range env {
			if len(item) >= len(prefix) && strings.EqualFold(item[:len(prefix)], prefix) {
				env[i] = prefix + value
				return
			}
		}
		env = append(env, prefix+value)
	}
	set("TERM", "xterm-256color")
	set("COLORTERM", "truecolor")
	set("TERM_PROGRAM", "agentsafe")
	set("TERM_PROGRAM_VERSION", "agentsafe-desktop")
	set("LANG", "C.UTF-8")
	set("LC_ALL", "C.UTF-8")
	set("PYTHONIOENCODING", "utf-8")
	set("LESSCHARSET", "utf-8")
	set("FORCE_COLOR", "1")
	if cols > 0 {
		set("COLUMNS", strconv.Itoa(cols))
	}
	if rows > 0 {
		set("LINES", strconv.Itoa(rows))
	}
	return env
}

func (s *conPTYSession) Read(p []byte) (int, error)  { return s.pty.Read(p) }
func (s *conPTYSession) Write(p []byte) (int, error) { return s.pty.Write(p) }
func (s *conPTYSession) Close() error {
	err := s.pty.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == 0 {
		return nil
	}
	return windows.TerminateProcess(s.handle, 1)
}
func (s *conPTYSession) Wait() error {
	s.mu.Lock()
	handle := s.handle
	s.mu.Unlock()
	if handle == 0 {
		return nil
	}
	status, err := windows.WaitForSingleObject(handle, 5000)
	if err != nil {
		return err
	}
	if status == uint32(windows.WAIT_TIMEOUT) {
		_ = windows.TerminateProcess(handle, 1)
		_, _ = windows.WaitForSingleObject(handle, 1000)
		return fmt.Errorf("exit wait timed out")
	}
	if status != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("unexpected wait status %d", status)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit status %d", code)
	}
	return nil
}
