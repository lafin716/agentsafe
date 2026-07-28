// Package config는 agentsafe 워크스페이스 설정을 다룬다.
// 워크스페이스 루트의 .agentsafe/config.yaml을 생성·저장·로드하고,
// 현재 디렉터리에서 상위로 올라가며 워크스페이스 루트를 탐색하며,
// 저장소·feature 이름과 URL의 유효성을 검사한다.
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

// DirName은 워크스페이스 루트 아래에 놓이는 agentsafe 메타데이터 디렉터리 이름이다.
const DirName = ".agentsafe"

// ConfigFileName은 DirName 안에 저장되는 설정 파일 이름이다.
const ConfigFileName = "config.yaml"

// Config는 .agentsafe/config.yaml에 직렬화되는 워크스페이스 전체 설정이다.
type Config struct {
	Version      int          `yaml:"version"`      // 설정 스키마 버전
	Workspace    Workspace    `yaml:"workspace"`    // 워크스페이스 이름과 루트 경로
	Git          GitConfig    `yaml:"git"`          // 브랜치 기본값
	Repositories []Repository `yaml:"repositories"` // 이 워크스페이스가 관리하는 저장소 목록
	Agent        AgentConfig  `yaml:"agent"`        // 에이전트 사본에 적용할 보안/제외 설정
	GitLab       GitLabConfig `yaml:"gitlab"`       // GitLab MR 연동 설정
	GitHub       GitHubConfig `yaml:"github"`       // GitHub PR 연동 설정
}

// Workspace는 워크스페이스 자체를 식별하는 정보다.
type Workspace struct {
	Name string `yaml:"name"` // 워크스페이스 표시 이름 (미지정 시 루트 디렉터리명)
	Root string `yaml:"root"` // 워크스페이스 루트 절대 경로
}

// GitConfig는 feature 브랜치를 만들 때 쓰는 Git 기본값이다.
type GitConfig struct {
	DefaultBaseBranch string `yaml:"defaultBaseBranch"` // 분기 기준이 되는 기본 브랜치
	BranchPrefix      string `yaml:"branchPrefix"`      // feature 브랜치 이름 앞에 붙일 접두사
}

// Repository는 워크스페이스가 관리하는 저장소 하나를 나타낸다.
type Repository struct {
	Name          string `yaml:"name"`                    // main/ 아래 클론 디렉터리명 겸 식별자
	URL           string `yaml:"url"`                     // 클론에 사용할 원격 URL (https 또는 git@)
	DefaultBranch string `yaml:"defaultBranch,omitempty"` // 이 저장소만 다른 기본 브랜치를 쓸 때 지정
	TestCommand   string `yaml:"testCommand,omitempty"`   // 검증 단계에서 실행할 테스트 명령
}

// AgentConfig는 에이전트에게 넘길 사본을 만들 때 적용되는 보안 설정이다.
type AgentConfig struct {
	// SecurityFileName은 ignore와 mask 규칙을 하나로 합친 설정 파일(agentsafe.yaml)의 이름이다.
	SecurityFileName string `yaml:"securityFileName"`
	// IgnoreFileName과 MaskFileName은 예전에 쓰던 분리형 파일명이다.
	// 하위 호환을 위한 읽기와 통합 파일(SecurityFileName)로의 1회성 마이그레이션 용도로만
	// 남아 있으며, 새로 만드는 워크스페이스에서는 더 이상 생성하지 않는다.
	IgnoreFileName string   `yaml:"ignoreFileName,omitempty"`
	MaskFileName   string   `yaml:"maskFileName,omitempty"`
	DefaultExclude []string `yaml:"defaultExclude"` // 항상 사본에서 제외할 기본 경로/패턴
	// RespectGitignore를 켜면 prepare/diff/sync가 feature 워크트리의 Git이 무시하는 경로
	// (해당 .gitignore 규칙)까지 함께 제외한다. 에이전트가 만든 빌드 산출물이 워크트리로
	// 역유입되는 것을 막기 위함이다. 기존 config.yaml에 이 필드가 없을 때 기본값을 ON으로
	// 두기 위해 포인터로 선언했으므로, 값을 읽을 때는 GitignoreEnabled를 사용한다.
	RespectGitignore *bool `yaml:"respectGitignore,omitempty"`
}

// GitignoreEnabled는 prepare/diff/sync가 feature 워크트리의 Git 무시 규칙을 따를지 알려준다.
// 필드가 설정되지 않은 경우 true를 반환하므로 기존 워크스페이스도 자동으로 이 동작을 따르게 된다.
func (a AgentConfig) GitignoreEnabled() bool {
	return a.RespectGitignore == nil || *a.RespectGitignore
}

// GitLabConfig는 GitLab 머지 리퀘스트 생성에 필요한 연동 설정이다.
type GitLabConfig struct {
	BaseURL      string `yaml:"baseUrl"`      // GitLab 인스턴스 주소
	TokenEnv     string `yaml:"tokenEnv"`     // 액세스 토큰을 읽어올 환경 변수 이름
	TargetBranch string `yaml:"targetBranch"` // MR의 기본 대상 브랜치
}

// GitHubConfig는 GitHub 풀 리퀘스트 생성에 필요한 연동 설정이다.
type GitHubConfig struct {
	TokenEnv     string `yaml:"tokenEnv"`     // 액세스 토큰을 읽어올 환경 변수 이름
	TargetBranch string `yaml:"targetBranch"` // PR의 기본 대상 브랜치
}

// Default는 root를 워크스페이스 루트로 하는 기본 설정을 만든다.
// name이 비어 있으면 root의 디렉터리명을 워크스페이스 이름으로 쓴다.
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
			// 빌드 산출물, 에디터 설정, 자격 증명류를 기본으로 제외한다.
			DefaultExclude:   []string{".git", "node_modules", "build", "dist", "target", ".gradle", ".idea", ".vscode", ".env", ".env.*", "*.pem", "*.key", "*.p12", "*.jks", "application-local.yml", "application-secret.yml", "application-dev.yml", "secrets.yml", "credentials.yml"},
			RespectGitignore: boolPtr(true),
		},
		GitLab: GitLabConfig{BaseURL: "https://gitlab.example.com", TokenEnv: "GITLAB_TOKEN", TargetBranch: "develop"},
		GitHub: GitHubConfig{TokenEnv: "GITHUB_TOKEN", TargetBranch: "main"},
	}
}

// InitWorkspace는 root에 새 워크스페이스를 만든다.
// 표준 디렉터리 구조(.agentsafe, main, feature, agent와 features/sessions 하위 디렉터리)를
// 생성하고 기본 config.yaml을 저장한 뒤, agentsafe.yaml이 없으면 샘플을 함께 써 넣는다.
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

// boolPtr는 불리언 리터럴을 포인터 필드에 넣기 위한 헬퍼다.
func boolPtr(b bool) *bool { return &b }

// writeIfMissing은 path에 파일이 없을 때만 data를 기록한다.
// 이미 있는 사용자 파일을 덮어쓰지 않는 것이 목적이므로 쓰기 실패는 무시한다.
func writeIfMissing(path, data string) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(path, []byte(data), 0644)
	}
}

// Save는 cfg를 YAML로 직렬화해 root/.agentsafe/config.yaml에 기록한다.
// 필요하면 .agentsafe 디렉터리를 먼저 만든다.
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

// Load는 root/.agentsafe/config.yaml을 읽어 Config로 되돌린다.
// 파일에 워크스페이스 루트가 비어 있으면 인자로 받은 root로 채워 준다.
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

// DiscoverRoot는 start에서 시작해 상위 디렉터리로 거슬러 올라가며
// .agentsafe/config.yaml을 가진 첫 디렉터리를 워크스페이스 루트로 찾아낸다.
// 파일시스템 최상위까지 올라가도 못 찾으면 오류를 반환한다.
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

// LoadFrom은 start 기준으로 워크스페이스 루트를 찾아 그 설정까지 한 번에 읽어 온다.
// 찾아낸 루트 경로와 설정을 함께 반환한다.
func LoadFrom(start string) (string, Config, error) {
	root, err := DiscoverRoot(start)
	if err != nil {
		return "", Config{}, err
	}
	cfg, err := Load(root)
	return root, cfg, err
}

// repoNameRE는 저장소 이름으로 허용되는 문자 집합이다.
// 영숫자로 시작하고 이후에는 영숫자와 점·밑줄·하이픈만 올 수 있다.
var repoNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateRepoName은 저장소 이름이 허용 문자만 쓰는지, 상위 디렉터리 탈출("..")을
// 시도하지 않는지 검사한다. 이 이름은 디렉터리명으로 그대로 쓰이기 때문이다.
func ValidateRepoName(name string) error {
	if !repoNameRE.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid repository name %q", name)
	}
	return nil
}

// ValidateFeatureName은 feature 이름이 비어 있지 않고, 경로 구분자나 파일명에
// 쓸 수 없는 문자 및 ".."를 포함하지 않는지 검사한다.
func ValidateFeatureName(name string) error {
	if name == "" || strings.ContainsAny(name, `\/:*?"<>|`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid feature name %q", name)
	}
	return nil
}

// ValidateRepoURL은 저장소 URL이 SCP 형식의 SSH 주소(git@host:path)이거나
// 스킴과 호스트를 모두 갖춘 URL인지 검사한다.
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

// AddRepository는 저장소 r을 설정에 추가하고 저장한다.
// 이름과 URL의 유효성을 검사하며, 같은 이름이 이미 있으면 오류를 반환한다.
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

// RemoveRepository는 지정한 이름의 저장소를 설정에서 빼고 저장한다.
// config.yaml만 수정할 뿐, main/ 아래의 클론 디렉터리와 feature 워크트리는 그대로 남겨 둔다.
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

// SampleSecurityYAML은 새 워크스페이스에 기록되는 통합 에이전트 보안 설정 문서다.
// 예전의 .agentignore 패턴과 mask.json 규칙을 agentsafe.yaml 한 파일로 합친 형태다.
const SampleSecurityYAML = `# Agent security config (agentsafe.yaml)
# ignore: files/folders excluded from the agent copy ("#" comments allowed).
# Patterns without "*" are repo-root relative; use globs such as "*/secret/" for nested matches.
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
