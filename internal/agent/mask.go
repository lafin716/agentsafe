package agent

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

type MaskFile struct {
	Rules []MaskRule `json:"rules"`
}
type MaskRule struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
}

func LoadMask(path string) (MaskFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return MaskFile{}, err
	}
	var m MaskFile
	return m, json.Unmarshal(b, &m)
}

func LoadFirstMask(paths ...string) MaskFile {
	for _, p := range paths {
		if m, err := LoadMask(p); err == nil {
			return m
		}
	}
	return MaskFile{}
}

func (m MaskFile) Apply(s string) (string, bool) {
	changed := false
	out := s
	for _, r := range m.Rules {
		repl := r.Replacement
		if repl == "" {
			repl = "__MASKED__"
		}
		switch strings.ToLower(r.Type) {
		case "plain":
			n := strings.ReplaceAll(out, r.Pattern, repl)
			if n != out {
				changed = true
				out = n
			}
		case "regex":
			re, err := regexp.Compile(r.Pattern)
			if err == nil {
				n := re.ReplaceAllString(out, repl)
				if n != out {
					changed = true
					out = n
				}
			}
		}
	}
	return out, changed
}
