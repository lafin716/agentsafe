package applog

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// rotatingWriter appends to a log file and, when it would exceed maxSize,
// rotates once: the current file is renamed to "<path>.1" (replacing any prior
// rotation) and a fresh file is started. Keeping a single generation bounds
// disk use while preserving the most recent prior session for inspection.
type rotatingWriter struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	size    int64
	f       *os.File
}

func newRotatingWriter(path string, maxSize int64) (*rotatingWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &rotatingWriter{path: path, maxSize: maxSize, size: info.Size(), f: f}, nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return 0, errors.New("applog: writer is closed")
	}
	if w.maxSize > 0 && w.size+int64(len(p)) > w.maxSize && w.size > 0 {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate closes the current file, moves it aside to "<path>.1", and opens a
// fresh truncated file. Caller holds w.mu.
func (w *rotatingWriter) rotate() error {
	if err := w.f.Close(); err != nil {
		return err
	}
	rotated := w.path + ".1"
	_ = os.Remove(rotated)
	_ = os.Rename(w.path, rotated)
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	w.f = f
	w.size = 0
	return nil
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}
