package agent

import (
	"os"
	"path"
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
	base := path.Base(rel)
	for _, pat := range m.patterns {
		p := strings.TrimPrefix(pat, "/")
		dirOnly := strings.HasSuffix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			continue
		}
		glob := hasGlob(p)
		if dirOnly && !isDir {
			if glob {
				if globDirContains(p, rel) {
					return true
				}
				continue
			} else if !rootPathContains(p, rel) {
				continue
			}
		}
		if !glob {
			if rootPathContains(p, rel) {
				return true
			}
			continue
		}
		if p == rel || strings.HasPrefix(rel, p+"/") {
			return true
		}
		if ok, _ := path.Match(p, base); ok {
			return true
		}
		if ok, _ := path.Match(p, rel); ok {
			return true
		}
	}
	return false
}

func rootPathContains(pattern, rel string) bool {
	return pattern == rel || strings.HasPrefix(rel, pattern+"/")
}

func hasGlob(pattern string) bool {
	return strings.Contains(pattern, "*")
}

func globDirContains(pattern, rel string) bool {
	for dir := path.Dir(rel); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if ok, _ := path.Match(pattern, dir); ok {
			return true
		}
	}
	return false
}
