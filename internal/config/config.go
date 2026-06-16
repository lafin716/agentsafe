package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const DirName = ".agentsafe"
const ConfigFileName = "config.yaml"

type Config struct {
	Version      int          `yaml:"version"`
	Workspace    Workspace    `yaml:"workspace"`
	Git          GitConfig    `yaml:"git"`
	Repositories []Repository `yaml:"repositories"`
	Agent        AgentConfig  `yaml:"agent"`
	GitLab       GitLabConfig `yaml:"gitlab"`
	GitHub       GitHubConfig `yaml:"github"`
}

type Workspace struct {
	Name string `yaml:"name"`
	Root string `yaml:"root"`
}
type GitConfig struct {
	DefaultBaseBranch string `yaml:"defaultBaseBranch"`
	BranchPrefix      string `yaml:"branchPrefix"`
}
type Repository struct {
	Name          string `yaml:"name"`
	URL           string `yaml:"url"`
	DefaultBranch string `yaml:"defaultBranch,omitempty"`
	TestCommand   string `yaml:"testCommand,omitempty"`
}
type AgentConfig struct {
	// SecurityFileName is the unified ignore+mask config (agentsafe.yaml).
	SecurityFileName string `yaml:"securityFileName"`
	// IgnoreFileName and MaskFileName are the legacy split-file names, retained
	// for backward-compatible reading and one-time migration into the unified
	// SecurityFileName. New workspaces no longer create them.
	IgnoreFileName string   `yaml:"ignoreFileName,omitempty"`
	MaskFileName   string   `yaml:"maskFileName,omitempty"`
	DefaultExclude []string `yaml:"defaultExclude"`
}
type GitLabConfig struct {
	BaseURL      string `yaml:"baseUrl"`
	TokenEnv     string `yaml:"tokenEnv"`
	TargetBranch string `yaml:"targetBranch"`
}
type GitHubConfig struct {
	TokenEnv     string `yaml:"tokenEnv"`
	TargetBranch string `yaml:"targetBranch"`
}

func Default(root, name string) Config {
	if name == "" {
		name = filepath.Base(root)
	}
	return Config{
		Version:      1,
		Workspace:    Workspace{Name: name, Root: root},
		Git:          GitConfig{DefaultBaseBranch: "develop", BranchPrefix: "feature/"},
		Repositories: []Repository{},
		Agent: AgentConfig{
			SecurityFileName: "agentsafe.yaml",
			DefaultExclude:   []string{".git", "node_modules", "build", "dist", "target", ".gradle", ".idea", ".vscode", ".env", ".env.*", "*.pem", "*.key", "*.p12", "*.jks", "application-local.yml", "application-secret.yml", "application-dev.yml", "secrets.yml", "credentials.yml"},
		},
		GitLab: GitLabConfig{BaseURL: "https://gitlab.example.com", TokenEnv: "GITLAB_TOKEN", TargetBranch: "develop"},
		GitHub: GitHubConfig{TokenEnv: "GITHUB_TOKEN", TargetBranch: "main"},
	}
}

func InitWorkspace(root, name string) (Config, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Config{}, err
	}
	cfg := Default(abs, name)
	for _, d := range []string{DirName, "main", "feature", "agent", filepath.Join(DirName, "features"), filepath.Join(DirName, "sessions")} {
		if err := os.MkdirAll(filepath.Join(abs, d), 0755); err != nil {
			return Config{}, err
		}
	}
	if err := Save(abs, cfg); err != nil {
		return Config{}, err
	}
	writeIfMissing(filepath.Join(abs, "agentsafe.yaml"), SampleSecurityYAML)
	return cfg, nil
}

func writeIfMissing(path, data string) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(path, []byte(data), 0644)
	}
}

func Save(root string, cfg Config) error {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, DirName), 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, DirName, ConfigFileName), b, 0644)
}

func Load(root string) (Config, error) {
	b, err := os.ReadFile(filepath.Join(root, DirName, ConfigFileName))
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Workspace.Root == "" {
		cfg.Workspace.Root = root
	}
	return cfg, nil
}

func DiscoverRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	cur := abs
	for {
		if _, err := os.Stat(filepath.Join(cur, DirName, ConfigFileName)); err == nil {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", fmt.Errorf("agentsafe workspace not found from %s; run `agentsafe init` first", abs)
}

func LoadFrom(start string) (string, Config, error) {
	root, err := DiscoverRoot(start)
	if err != nil {
		return "", Config{}, err
	}
	cfg, err := Load(root)
	return root, cfg, err
}

var repoNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func ValidateRepoName(name string) error {
	if !repoNameRE.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid repository name %q", name)
	}
	return nil
}

func ValidateFeatureName(name string) error {
	if name == "" || strings.ContainsAny(name, `\/:*?"<>|`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid feature name %q", name)
	}
	return nil
}

func ValidateRepoURL(raw string) error {
	if strings.HasPrefix(raw, "git@") && strings.Contains(raw, ":") {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid repository URL %q", raw)
	}
	return nil
}

func AddRepository(root string, cfg Config, r Repository) (Config, error) {
	if err := ValidateRepoName(r.Name); err != nil {
		return cfg, err
	}
	if err := ValidateRepoURL(r.URL); err != nil {
		return cfg, err
	}
	for _, existing := range cfg.Repositories {
		if existing.Name == r.Name {
			return cfg, fmt.Errorf("repository %q already exists", r.Name)
		}
	}
	cfg.Repositories = append(cfg.Repositories, r)
	return cfg, Save(root, cfg)
}

// RemoveRepository drops the named repository from the config and persists it.
// It only edits config.yaml; cloned directories under main/ and any feature
// worktrees are left untouched.
func RemoveRepository(root string, cfg Config, name string) (Config, error) {
	idx := -1
	for i, existing := range cfg.Repositories {
		if existing.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return cfg, fmt.Errorf("repository %q not found", name)
	}
	cfg.Repositories = append(cfg.Repositories[:idx], cfg.Repositories[idx+1:]...)
	return cfg, Save(root, cfg)
}

// SampleSecurityYAML is the unified agent security config written to new
// workspaces. It merges the former .agentignore patterns and mask.json rules
// into a single agentsafe.yaml document.
const SampleSecurityYAML = `# Agent security config (agentsafe.yaml)
# ignore: files/folders excluded from the agent copy (gitignore-style, "#" comments allowed)
# mask:   content masking rules applied to copied text files (type: plain | regex | keypath)

ignore:
  - .env
  - .env.*
  - "*.pem"
  - "*.key"
  - "*.p12"
  - "*.jks"
  - application-local.yml
  - application-secret.yml
  - application-dev.yml
  - secrets.yml
  - credentials.yml
  - build/
  - dist/
  - target/
  - node_modules/
  - .idea/
  - .vscode/
  - .git/

mask:
  - name: AWS Access Key
    type: regex
    pattern: AKIA[0-9A-Z]{16}
    replacement: __MASKED_AWS_ACCESS_KEY__
  - name: JWT Token
    type: regex
    pattern: eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+
    replacement: __MASKED_JWT__
  - name: Internal Domain
    type: plain
    pattern: internal.company.local
    replacement: __MASKED_INTERNAL_DOMAIN__
  - name: DB Password
    type: keypath
    pattern: spring.datasource.password
    replacement: __MASKED__
`
