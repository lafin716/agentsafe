package main

import (
	"io"
	"strings"
)

type ptySession interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Kill() error
	Wait() error
}

func isPTYUnsupported(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unsupported")
}
