package agent

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/agentsafe/agentsafe/internal/config"
	"gopkg.in/yaml.v3"
)

// SecurityFile is the unified agent security config (agentsafe.yaml). It merges
// the former ".agentignore" (gitignore-style copy-exclusion patterns) and
// "mask.json" (content masking rules) into a single YAML document.
type SecurityFile struct {
	Ignore []string   `json:"ignore" yaml:"ignore"`
	Mask   []MaskRule `json:"mask"   yaml:"mask"`
}

// securityName returns the configured unified filename, defaulting to
// "agentsafe.yaml" when unset.
func securityName(cfg config.Config) string {
	if cfg.Agent.SecurityFileName != "" {
		return cfg.Agent.SecurityFileName
	}
	return "agentsafe.yaml"
}

// legacyNames returns the configured legacy ignore/mask filenames, defaulting to
// ".agentignore" and "mask.json" when unset.
func legacyNames(cfg config.Config) (ignore, mask string) {
	ignore = cfg.Agent.IgnoreFileName
	if ignore == "" {
		ignore = ".agentignore"
	}
	mask = cfg.Agent.MaskFileName
	if mask == "" {
		mask = "mask.json"
	}
	return ignore, mask
}

// LoadSecurityFile parses a unified agentsafe.yaml file at path.
func LoadSecurityFile(path string) (SecurityFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SecurityFile{}, err
	}
	var sf SecurityFile
	if err := yaml.Unmarshal(b, &sf); err != nil {
		return SecurityFile{}, err
	}
	return sf, nil
}

// LoadSecurity loads the unified security config from dir. It prefers
// dir/agentsafe.yaml; when that is absent it falls back to the legacy
// dir/.agentignore + dir/mask.json pair so existing workspaces keep working.
func LoadSecurity(cfg config.Config, dir string) SecurityFile {
	if sf, err := LoadSecurityFile(filepath.Join(dir, securityName(cfg))); err == nil {
		return sf
	}
	ignoreName, maskName := legacyNames(cfg)
	var sf SecurityFile
	for _, line := range LoadIgnoreFiles(filepath.Join(dir, ignoreName)) {
		if line != "" { // drop blank lines; comments ("#") are kept
			sf.Ignore = append(sf.Ignore, line)
		}
	}
	if m, err := LoadMask(filepath.Join(dir, maskName)); err == nil {
		sf.Mask = m.Rules
	}
	return sf
}

// WriteSecurity marshals sf to dir/agentsafe.yaml.
func WriteSecurity(cfg config.Config, dir string, sf SecurityFile) error {
	b, err := yaml.Marshal(sf)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, securityName(cfg)), b, 0644)
}

// EnsureSecurityFile migrates legacy ignore/mask files into a unified
// agentsafe.yaml at the workspace root. It is a no-op when the unified file
// already exists or when no legacy file is present. Legacy files are left in
// place (non-destructive). It must only be called on the workspace root, never
// on a repository source directory.
func EnsureSecurityFile(cfg config.Config, root string) error {
	unified := filepath.Join(root, securityName(cfg))
	if _, err := os.Stat(unified); err == nil {
		return nil
	}
	ignoreName, maskName := legacyNames(cfg)
	_, ierr := os.Stat(filepath.Join(root, ignoreName))
	_, merr := os.Stat(filepath.Join(root, maskName))
	if errors.Is(ierr, os.ErrNotExist) && errors.Is(merr, os.ErrNotExist) {
		return nil
	}
	sf := LoadSecurity(cfg, root)
	return WriteSecurity(cfg, root, sf)
}
