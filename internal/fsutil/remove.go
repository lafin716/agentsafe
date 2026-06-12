package fsutil

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// ForceRemoveAll removes path and all of its children. If the first attempt
// fails it clears the read-only attribute on every entry and retries, so it
// also succeeds on Windows where os.RemoveAll fails on read-only files (for
// example Git pack/idx files in a clone).
func ForceRemoveAll(path string) error {
	if err := os.RemoveAll(path); err == nil {
		return nil
	}
	_ = filepath.Walk(path, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr == nil && info != nil {
			_ = os.Chmod(p, info.Mode()|0o200) // clear the read-only bit
		}
		return nil
	})
	return os.RemoveAll(path)
}

// IsEmptyDir reports whether path is an existing directory with no entries.
func IsEmptyDir(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(names) == 0, nil
}
