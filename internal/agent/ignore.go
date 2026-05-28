package agent

import (
	"os"
	"path/filepath"
	"strings"
)

type IgnoreMatcher struct{ patterns []string }

func NewIgnoreMatcher(patterns []string) IgnoreMatcher {
	var p []string
	for _, s := range patterns {
		s = strings.TrimSpace(s)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		p = append(p, filepath.ToSlash(s))
	}
	return IgnoreMatcher{p}
}

func LoadIgnoreFiles(paths ...string) []string {
	var out []string
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				out = append(out, strings.TrimSpace(line))
			}
		}
	}
	return out
}

func (m IgnoreMatcher) Match(rel string, isDir bool) bool {
	rel = filepath.ToSlash(rel)
	base := pathBase(rel)
	for _, pat := range m.patterns {
		p := strings.TrimPrefix(pat, "/")
		dirOnly := strings.HasSuffix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if dirOnly && !isDir && !strings.Contains(rel, p+"/") {
			continue
		}
		if p == rel || p == base || strings.HasPrefix(rel, p+"/") {
			return true
		}
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
		if ok, _ := filepath.Match(p, rel); ok {
			return true
		}
	}
	return false
}

func pathBase(p string) string {
	parts := strings.Split(p, "/")
	return parts[len(parts)-1]
}
