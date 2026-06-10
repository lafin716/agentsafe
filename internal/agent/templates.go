package agent

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed templates/*.yaml
var templateFS embed.FS

// Template is a stack-specific preset of ignore patterns and masking rules,
// loaded from an embedded YAML file under templates/.
type Template struct {
	Label       string     `yaml:"label"`
	Description string     `yaml:"description"`
	Ignore      []string   `yaml:"ignore"`
	Mask        []MaskRule `yaml:"mask"`
}

// Security returns the template's content as a SecurityFile.
func (t Template) Security() SecurityFile {
	return SecurityFile{Ignore: t.Ignore, Mask: t.Mask}
}

// TemplateInfo is a lightweight summary of a template for listing in the CLI
// and desktop UI.
type TemplateInfo struct {
	Key         string `json:"key"         yaml:"key"`
	Label       string `json:"label"       yaml:"label"`
	Description string `json:"description"  yaml:"description"`
	IgnoreCount int    `json:"ignoreCount" yaml:"ignoreCount"`
	MaskCount   int    `json:"maskCount"   yaml:"maskCount"`
}

// templateKeys returns the embedded template keys (filename without extension),
// sorted alphabetically.
func templateKeys() []string {
	entries, _ := templateFS.ReadDir("templates")
	var keys []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		keys = append(keys, strings.TrimSuffix(name, ".yaml"))
	}
	sort.Strings(keys)
	return keys
}

// GetTemplate loads and parses the embedded template for key (e.g. "spring").
func GetTemplate(key string) (Template, error) {
	b, err := templateFS.ReadFile(path.Join("templates", key+".yaml"))
	if err != nil {
		return Template{}, fmt.Errorf("unknown template %q", key)
	}
	var t Template
	if err := yaml.Unmarshal(b, &t); err != nil {
		return Template{}, fmt.Errorf("invalid template %q: %w", key, err)
	}
	return t, nil
}

// TemplateList returns a summary of all available templates, sorted by key.
func TemplateList() []TemplateInfo {
	var out []TemplateInfo
	for _, key := range templateKeys() {
		t, err := GetTemplate(key)
		if err != nil {
			continue
		}
		out = append(out, TemplateInfo{
			Key:         key,
			Label:       t.Label,
			Description: t.Description,
			IgnoreCount: len(t.Ignore),
			MaskCount:   len(t.Mask),
		})
	}
	return out
}
