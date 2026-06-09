package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type MaskFile struct {
	Rules []MaskRule `json:"rules"`
}
type MaskRule struct {
	Name        string `json:"name"        yaml:"name"`
	Type        string `json:"type"        yaml:"type"`
	Pattern     string `json:"pattern"     yaml:"pattern"`
	Replacement string `json:"replacement" yaml:"replacement"`
}

func LoadMask(path string) (MaskFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return MaskFile{}, err
	}
	var m MaskFile
	return m, json.Unmarshal(b, &m)
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

// ApplyKeyPaths masks values addressed by a dotted key path inside structured
// (JSON/YAML) content. A rule of type "keypath" (alias "key") with pattern
// "main.sub" replaces the value at that path with its replacement (default
// "__MASKED__"). ext selects the parser (.json / .yaml / .yml); other
// extensions and parse failures leave the content untouched. Multi-document
// YAML (separated by "---") is fully preserved — every document is masked and
// re-emitted.
//
// The content is parsed and re-serialized, so key order, indentation, and YAML
// comments are not preserved — acceptable for the sanitized agent copy.
func (m MaskFile) ApplyKeyPaths(content, ext string) (string, bool) {
	var rules []MaskRule
	for _, r := range m.Rules {
		switch strings.ToLower(r.Type) {
		case "keypath", "key":
			rules = append(rules, r)
		}
	}
	if len(rules) == 0 {
		return content, false
	}
	switch strings.ToLower(ext) {
	case ".json":
		return applyKeyPathsJSON(content, rules)
	case ".yaml", ".yml":
		return applyKeyPathsYAML(content, rules)
	default:
		return content, false
	}
}

func applyKeyPathsJSON(content string, rules []MaskRule) (string, bool) {
	var data map[string]interface{}
	if json.Unmarshal([]byte(content), &data) != nil || data == nil {
		return content, false
	}
	if !applyRulesToMap(data, rules) {
		return content, false
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return content, false
	}
	return string(b), true
}

func applyKeyPathsYAML(content string, rules []MaskRule) (string, bool) {
	dec := yaml.NewDecoder(strings.NewReader(content))
	var docs []interface{}
	for {
		var d interface{}
		err := dec.Decode(&d)
		if err == io.EOF {
			break
		}
		if err != nil {
			return content, false
		}
		if d == nil { // empty document (e.g. trailing "---")
			continue
		}
		docs = append(docs, d)
	}
	if len(docs) == 0 {
		return content, false
	}

	changed := false
	for _, d := range docs {
		if mp, ok := d.(map[string]interface{}); ok {
			if applyRulesToMap(mp, rules) {
				changed = true
			}
		}
	}
	if !changed {
		return content, false
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	for _, d := range docs {
		if err := enc.Encode(d); err != nil {
			_ = enc.Close()
			return content, false
		}
	}
	if err := enc.Close(); err != nil {
		return content, false
	}
	return buf.String(), true
}

// applyRulesToMap applies all keypath rules to a parsed map, returning whether
// any value was replaced.
func applyRulesToMap(data map[string]interface{}, rules []MaskRule) bool {
	changed := false
	for _, r := range rules {
		repl := r.Replacement
		if repl == "" {
			repl = "__MASKED__"
		}
		if setKeyPath(data, strings.Split(r.Pattern, "."), repl) {
			changed = true
		}
	}
	return changed
}

// setKeyPath walks a dotted path through nested maps and sets the leaf to repl.
// Returns false (no change) when any segment is missing or not a map.
func setKeyPath(m map[string]interface{}, path []string, repl string) bool {
	if len(path) == 0 {
		return false
	}
	key := path[0]
	v, ok := m[key]
	if !ok {
		return false
	}
	if len(path) == 1 {
		m[key] = repl
		return true
	}
	child, ok := v.(map[string]interface{})
	if !ok {
		return false
	}
	return setKeyPath(child, path[1:], repl)
}
