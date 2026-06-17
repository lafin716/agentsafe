//go:build !windows

package main

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type osPTYSession struct {
	cmd  *exec.Cmd
	file *os.File
}

func startPTYProcess(target, name string, args []string, cols, rows int) (ptySession, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = target
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, err
	}
	return &osPTYSession{cmd: cmd, file: file}, nil
}

func (s *osPTYSession) Read(p []byte) (int, error)  { return s.file.Read(p) }
func (s *osPTYSession) Write(p []byte) (int, error) { return s.file.Write(p) }
func (s *osPTYSession) Close() error                { return s.file.Close() }
func (s *osPTYSession) Resize(cols, rows int) error {
	return pty.Setsize(s.file, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}
func (s *osPTYSession) Kill() error {
	if s.cmd.Process == nil {
		return nil
	}
	return s.cmd.Process.Kill()
}
func (s *osPTYSession) Wait() error { return s.cmd.Wait() }
