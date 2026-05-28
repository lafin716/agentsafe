package fsutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

func EnsureInside(base, target string) error {
	b, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	t, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(b, t)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %s escapes base %s", t, b)
	}
	return nil
}
