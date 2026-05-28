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
	Type          string `yaml:"type,omitempty"`
	TestCommand   string `yaml:"testCommand,omitempty"`
}
type AgentConfig struct {
	IgnoreFileName string   `yaml:"ignoreFileName"`
	MaskFileName   string   `yaml:"maskFileName"`
	DefaultExclude []string `yaml:"defaultExclude"`
}
type GitLabConfig struct {
	BaseURL      string `yaml:"baseUrl"`
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
			IgnoreFileName: ".agentignore",
			MaskFileName:   "mask.json",
			DefaultExclude: []string{".git", "node_modules", "build", "dist", "target", ".gradle", ".idea", ".vscode", ".env", ".env.*", "*.pem", "*.key", "*.p12", "*.jks", "application-local.yml", "application-secret.yml", "application-dev.yml", "secrets.yml", "credentials.yml"},
		},
		GitLab: GitLabConfig{BaseURL: "https://gitlab.example.com", TokenEnv: "GITLAB_TOKEN", TargetBranch: "develop"},
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
	writeIfMissing(filepath.Join(abs, ".agentignore"), SampleAgentIgnore)
	writeIfMissing(filepath.Join(abs, "mask.json"), SampleMaskJSON)
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
	if r.DefaultBranch == "" {
		r.DefaultBranch = cfg.Git.DefaultBaseBranch
	}
	cfg.Repositories = append(cfg.Repositories, r)
	return cfg, Save(root, cfg)
}

const SampleAgentIgnore = `# secrets
.env
.env.*
*.pem
*.key
*.p12
*.jks
application-local.yml
application-secret.yml
application-dev.yml
secrets.yml
credentials.yml
build/
dist/
target/
node_modules/
.idea/
.vscode/
.git/
`

const SampleMaskJSON = `{"rules":[{"name":"AWS Access Key","type":"regex","pattern":"AKIA[0-9A-Z]{16}","replacement":"__MASKED_AWS_ACCESS_KEY__"},{"name":"JWT Token","type":"regex","pattern":"eyJ[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+","replacement":"__MASKED_JWT__"},{"name":"Internal Domain","type":"plain","pattern":"internal.company.local","replacement":"__MASKED_INTERNAL_DOMAIN__"}]}`
