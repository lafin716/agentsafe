# 개발업무용 멀티레포 코딩 에이전트 안전 작업 도구 MVP 구현 프롬프트

너는 시니어 시스템 소프트웨어 엔지니어이자 CLI/DevTool 아키텍트다.
아래 요구사항을 바탕으로 사내 개발자가 여러 개의 서비스 저장소를 하나의 기능 단위로 묶어 안전하게 코딩 에이전트를 사용할 수 있게 하는 개발업무용 CLI 도구의 MVP를 설계하고 구현해라.

## 0. 프로젝트 목표

우리 서비스는 현재 다음과 같이 3개의 별도 Git 저장소로 분리되어 있다.

* 백엔드 저장소
* 어드민 프론트 저장소
* 앱 프론트 저장소

각 저장소는 독립적으로 GitLab에 존재하지만, 실제 개발 업무는 하나의 기능이 3개 저장소에 동시에 걸쳐 진행되는 경우가 많다.

예를 들어 `feature/coupon-v2` 기능을 개발할 때 다음 작업이 동시에 필요할 수 있다.

* 백엔드 API 수정
* 어드민 프론트 화면 추가
* 앱 프론트 기능 연동

이 도구는 이런 멀티레포 작업을 하나의 “기능 작업공간”으로 묶고, 코딩 에이전트가 민감한 파일을 읽지 못하도록 별도의 에이전트용 복사본을 만든 뒤, 에이전트가 수정한 결과만 원본 워크트리로 안전하게 동기화하는 것을 목표로 한다.

## 1. 핵심 컨셉

이 앱은 다음 3가지 오픈소스 컨셉을 참고한다.

### 1.1 git-worktree-ctrl 컨셉

* 여러 저장소에 대해 동일한 기능 브랜치를 한 번에 생성한다.
* 각 저장소마다 `git worktree`를 사용하여 기능 브랜치 전용 작업 디렉토리를 만든다.
* 사용자는 브랜치 생성, 워크트리 생성, 워크트리 목록 확인, 삭제를 명령어 한 번으로 처리한다.
* 단일 저장소가 아니라 여러 저장소를 “기능 세트”로 묶는 것이 핵심이다.

### 1.2 agentroom 컨셉

* 코딩 에이전트가 읽으면 안 되는 파일이나 문자열을 숨긴다.
* 통합 보안 설정 파일 `agentsafe.yaml`의 `ignore` 섹션으로 에이전트용 폴더 복사 시 제외할 파일/폴더 패턴을 정의한다.
* 같은 `agentsafe.yaml`의 `mask` 섹션으로 특정 문자열 패턴을 마스킹한다.
* 원본 개발 작업공간과 에이전트 작업공간을 분리한다.
* 에이전트 작업공간에서 수정된 파일을 원본 워크트리로 다시 동기화한다.
* 동기화 전 변경사항을 diff로 보여주고 사용자의 승인을 받는다.

### 1.3 winmux 컨셉

* Windows 환경에서도 tmux처럼 작업 세션을 관리하고 싶다.
* MVP에서는 완전한 터미널 멀티플렉서를 구현하지 않는다.
* 대신 기능 작업공간 단위로 터미널 실행 스크립트, 세션 메타데이터, 에이전트 실행 명령을 관리한다.
* 추후 GUI 또는 백그라운드 터미널 매니저로 확장 가능하도록 구조를 설계한다.

## 2. MVP 제품명

가칭: `agentsafe`

CLI 명령어 이름도 `agentsafe`로 한다.

예시:

```bash
agentsafe init
agentsafe repo add backend https://gitlab.example.com/company/backend.git
agentsafe repo add admin-front https://gitlab.example.com/company/admin-front.git
agentsafe repo add app-front https://gitlab.example.com/company/app-front.git
agentsafe clone
agentsafe feature create coupon-v2 --base develop
agentsafe agent prepare coupon-v2
agentsafe agent sync coupon-v2
agentsafe mr create coupon-v2 --target develop
```

## 3. 기술 스택

MVP는 Go 언어로 구현한다.

### 3.1 이유

* 단일 바이너리 배포가 쉽다.
* Windows, macOS, Linux 크로스 플랫폼 지원이 쉽다.
* CLI 도구 개발에 적합하다.
* 추후 GUI 앱에서 동일한 core 패키지를 재사용할 수 있다.
* Git, 파일시스템, 프로세스 제어를 다루기 좋다.

### 3.2 기본 요구사항

* Go 1.22 이상
* CLI 프레임워크: `spf13/cobra`
* 설정 파일 처리: 표준 `encoding/json` 또는 `gopkg.in/yaml.v3`
* Git 실행: 우선은 `os/exec`로 `git` 명령 호출
* 파일 복사/동기화: 직접 구현
* Diff 출력: MVP에서는 `git diff --no-index` 또는 자체 파일 비교 중 단순한 방식 선택
* GitLab MR 생성: MVP에서는 GitLab API를 직접 호출하되, 실패 시 생성 URL 안내

## 4. 모노레포형 프로젝트 구조

확장성을 고려해 다음 구조로 프로젝트를 생성해라.

```text
agentsafe/
  go.mod
  cmd/
    agentsafe/
      main.go
  internal/
    app/
      app.go
    config/
      config.go
      workspace.go
    git/
      git.go
      worktree.go
    repo/
      repo.go
      clone.go
    feature/
      feature.go
    agent/
      prepare.go
      ignore.go
      mask.go
      sync.go
      diff.go
    gitlab/
      client.go
      merge_request.go
    session/
      session.go
    fsutil/
      copy.go
      hash.go
      path.go
    ui/
      prompt.go
      table.go
  docs/
    mvp.md
  examples/
    config.yaml
    agentsafe.yaml
```

## 5. 작업공간 디렉토리 모델

도구는 하나의 루트 작업공간을 기준으로 동작한다.

예시:

```text
D:/workspace/my-service/
  .agentsafe/
    config.yaml
    features/
      coupon-v2.json
    sessions/
      coupon-v2.json
  repos/
    backend/
      .git/
    admin-front/
      .git/
    app-front/
      .git/
  worktrees/
    coupon-v2/
      backend/
      admin-front/
      app-front/
  agent/
    coupon-v2/
      backend/
      admin-front/
      app-front/
```

### 5.1 디렉토리 의미

#### `repos/`

각 저장소의 기본 clone 위치다.

```text
repos/backend
repos/admin-front
repos/app-front
```

#### `worktrees/{featureName}/`

기능 단위 Git worktree 위치다.

```text
worktrees/coupon-v2/backend
worktrees/coupon-v2/admin-front
worktrees/coupon-v2/app-front
```

각 하위 폴더는 실제 Git worktree다.

#### `agent/{featureName}/`

코딩 에이전트가 접근하는 복사본이다.

```text
agent/coupon-v2/backend
agent/coupon-v2/admin-front
agent/coupon-v2/app-front
```

이 위치는 Git worktree가 아니다.
민감 파일 제거 및 마스킹이 적용된 안전한 복사본이다.

#### `.agentsafe/`

도구 내부 메타데이터 저장 위치다.

```text
.agentsafe/config.yaml
.agentsafe/features/coupon-v2.json
.agentsafe/sessions/coupon-v2.json
```

## 6. 설정 파일

루트 작업공간에 `.agentsafe/config.yaml`을 생성한다.

예시:

```yaml
version: 1

workspace:
  name: my-service
  root: D:/workspace/my-service

git:
  defaultBaseBranch: develop
  branchPrefix: feature/

repositories:
  - name: backend
    url: https://gitlab.example.com/company/backend.git
    defaultBranch: develop
    type: backend

  - name: admin-front
    url: https://gitlab.example.com/company/admin-front.git
    defaultBranch: develop
    type: frontend

  - name: app-front
    url: https://gitlab.example.com/company/app-front.git
    defaultBranch: develop
    type: frontend

agent:
  securityFileName: agentsafe.yaml
  defaultExclude:
    - .git
    - node_modules
    - build
    - dist
    - target
    - .gradle
    - .idea
    - .vscode
    - .env
    - .env.*
    - application-local.yml
    - application-secret.yml

gitlab:
  baseUrl: https://gitlab.example.com
  tokenEnv: GITLAB_TOKEN
  targetBranch: develop
```

## 7. `agentsafe.yaml` 보안 설정 파일

에이전트 보안 설정은 단일 파일 `agentsafe.yaml`로 통합되어 있다. 각 저장소 루트 또는 전체 작업공간 루트에 둘 수 있으며, `ignore`(복사 제외 패턴)와 `mask`(내용 마스킹 규칙) 두 섹션으로 구성된다.

> 하위 호환: 기존 `.agentignore` + `mask.json`이 있고 `agentsafe.yaml`이 없으면 그대로 읽어 동작하며, 작업공간 루트에 한해 자동으로 `agentsafe.yaml`로 마이그레이션한다(기존 파일은 비파괴적으로 유지).

### 7.1 `ignore` 섹션

복사 제외 패턴 우선순위:

1. 저장소별 `agentsafe.yaml`의 `ignore`
2. 루트 `agentsafe.yaml`의 `ignore`
3. config의 `agent.defaultExclude`

gitignore 스타일 패턴을 사용하며 `#`로 시작하는 항목은 주석으로 무시된다.

### 7.2 `mask` 섹션

에이전트용 폴더 복사 시 텍스트 파일 내용에서 특정 문자열을 치환한다. 마스킹 규칙은 저장소별 → 루트 순으로 우선 적용된다.

지원 타입:

* `plain` — 문자열 그대로 치환
* `regex` — 정규식 치환
* `keypath`(=`key`) — JSON/YAML 구조 내 점(`.`) 경로의 값 치환

마스킹은 텍스트 파일로 판단되는 파일에만 적용하며, 바이너리 파일은 복사하거나 제외만 하고 내용 마스킹을 하지 않는다.

### 7.3 예시 (`agentsafe.yaml`)

```yaml
# ignore: 에이전트 사본에서 제외할 파일/폴더 (gitignore 스타일, "#" 주석 허용)
# mask:   복사된 텍스트 파일에 적용할 마스킹 규칙 (plain | regex | keypath)

ignore:
  # secrets
  - .env
  - .env.*
  - "*.pem"
  - "*.key"
  - "*.p12"
  - "*.jks"
  # local config
  - application-local.yml
  - application-secret.yml
  - application-dev.yml
  # build outputs
  - build/
  - dist/
  - target/
  - node_modules/
  # IDE / git
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
```

## 9. 주요 명령어

## 9.1 `agentsafe init`

현재 디렉토리를 agentsafe 작업공간으로 초기화한다.

```bash
agentsafe init --name my-service
```

동작:

1. `.agentsafe/` 디렉토리 생성
2. `.agentsafe/config.yaml` 기본 생성
3. `repos/`, `worktrees/`, `agent/` 디렉토리 생성
4. 예시 `agentsafe.yaml`(통합 보안 설정) 생성 여부를 묻는다

옵션:

```bash
agentsafe init --name my-service --root D:/workspace/my-service
```

## 9.2 `agentsafe repo add`

저장소를 설정에 추가한다.

```bash
agentsafe repo add backend https://gitlab.example.com/company/backend.git --type backend
agentsafe repo add admin-front https://gitlab.example.com/company/admin-front.git --type frontend
agentsafe repo add app-front https://gitlab.example.com/company/app-front.git --type frontend
```

동작:

1. `.agentsafe/config.yaml`에 repository 항목 추가
2. 같은 name이 있으면 에러
3. URL 형식 검증
4. 저장소 이름은 파일 경로로 안전한 문자열만 허용

## 9.3 `agentsafe repo list`

등록된 저장소 목록 출력.

```bash
agentsafe repo list
```

출력 예시:

```text
NAME          TYPE       URL
backend       backend    https://gitlab.example.com/company/backend.git
admin-front   frontend   https://gitlab.example.com/company/admin-front.git
app-front     frontend   https://gitlab.example.com/company/app-front.git
```

## 9.4 `agentsafe clone`

등록된 모든 저장소를 `repos/` 하위에 clone한다.

```bash
agentsafe clone
```

동작:

1. config의 repositories 목록 순회
2. `repos/{repoName}`이 없으면 clone
3. 이미 있으면 fetch 수행
4. 실패한 저장소가 있으면 전체 결과 요약 출력

예시 내부 명령:

```bash
git clone {url} repos/{repoName}
git -C repos/{repoName} fetch --all --prune
```

## 9.5 `agentsafe feature create`

기능 단위로 모든 저장소에 브랜치와 worktree를 생성한다.

```bash
agentsafe feature create coupon-v2 --base develop
```

동작:

1. feature name을 검증한다.
2. 브랜치명은 기본적으로 `feature/{featureName}`으로 만든다.
3. 각 저장소에서 base 브랜치를 최신화한다.
4. 각 저장소에 worktree를 생성한다.
5. `.agentsafe/features/{featureName}.json` 메타데이터를 저장한다.

예시 내부 명령:

```bash
git -C repos/backend fetch origin
git -C repos/backend checkout develop
git -C repos/backend pull origin develop
git -C repos/backend worktree add ../../worktrees/coupon-v2/backend -b feature/coupon-v2 develop
```

프론트 저장소도 동일하게 처리한다.

이미 브랜치가 존재하는 경우:

* 로컬 브랜치가 있으면 해당 브랜치로 worktree 생성
* 원격 브랜치가 있으면 `origin/feature/coupon-v2` 기반으로 생성
* 둘 다 없으면 base branch 기반으로 새 브랜치 생성

메타데이터 예시:

```json
{
  "name": "coupon-v2",
  "branch": "feature/coupon-v2",
  "baseBranch": "develop",
  "createdAt": "2026-05-28T10:00:00+09:00",
  "repositories": [
    {
      "name": "backend",
      "worktreePath": "worktrees/coupon-v2/backend",
      "branch": "feature/coupon-v2"
    },
    {
      "name": "admin-front",
      "worktreePath": "worktrees/coupon-v2/admin-front",
      "branch": "feature/coupon-v2"
    },
    {
      "name": "app-front",
      "worktreePath": "worktrees/coupon-v2/app-front",
      "branch": "feature/coupon-v2"
    }
  ]
}
```

## 9.6 `agentsafe feature list`

기능 작업공간 목록을 출력한다.

```bash
agentsafe feature list
```

출력 예시:

```text
FEATURE      BRANCH             BASE      REPOS  AGENT_READY
coupon-v2    feature/coupon-v2   develop   3      yes
```

## 9.7 `agentsafe status`

현재 기능 작업공간의 변경 상태를 저장소별로 보여준다.

```bash
agentsafe status coupon-v2
```

동작:

각 worktree에서 다음 명령 수행:

```bash
git status --short
git branch --show-current
```

출력 예시:

```text
Feature: coupon-v2
Branch: feature/coupon-v2

[backend]
 M src/main/java/com/company/coupon/CouponService.java
?? src/main/java/com/company/coupon/CouponPolicy.java

[admin-front]
 M src/pages/CouponPage.vue

[app-front]
 clean
```

## 9.8 `agentsafe agent prepare`

에이전트용 안전 복사본을 생성한다.

```bash
agentsafe agent prepare coupon-v2
```

동작:

1. `worktrees/{featureName}/{repoName}`을 읽는다.
2. `agent/{featureName}/{repoName}`을 새로 만든다.
3. 기존 agent 폴더가 있으면 백업 또는 삭제 여부를 묻는다.
4. `agentsafe.yaml`의 `ignore`와 config exclude 규칙을 적용해 민감 파일을 제외한다.
5. `agentsafe.yaml`의 `mask` 규칙을 적용해 텍스트 파일 내용을 마스킹한다.
6. 복사 결과 요약을 출력한다.

출력 예시:

```text
Agent workspace prepared: agent/coupon-v2

[backend]
copied: 320 files
ignored: 18 files
masked: 4 files

[admin-front]
copied: 210 files
ignored: 42 files
masked: 1 files

[app-front]
copied: 180 files
ignored: 39 files
masked: 0 files
```

주의:

* agent 폴더에는 `.git`을 복사하지 않는다.
* agent 폴더에서는 Git 명령을 수행하지 않는다.
* agent 폴더는 코딩 에이전트가 수정하는 작업 공간이다.
* agent 폴더 내의 마스킹된 값이 원본으로 되돌아가는 기능은 MVP에서 구현하지 않는다.
* 따라서 마스킹된 파일은 기본적으로 sync 대상에서 제외하거나, sync 시 경고를 띄워야 한다.

## 9.9 `agentsafe agent open`

에이전트용 작업공간 경로를 출력하거나 VSCode/Cursor 등으로 연다.

```bash
agentsafe agent open coupon-v2
agentsafe agent open coupon-v2 --editor code
agentsafe agent open coupon-v2 --editor cursor
```

동작:

```bash
code agent/coupon-v2
```

또는

```bash
cursor agent/coupon-v2
```

## 9.10 `agentsafe agent diff`

에이전트 작업공간과 원본 worktree 사이의 변경사항을 보여준다.

```bash
agentsafe agent diff coupon-v2
```

비교 기준:

* source: `agent/{featureName}/{repoName}`
* target: `worktrees/{featureName}/{repoName}`

감지해야 하는 변경 유형:

* 추가된 파일
* 수정된 파일
* 삭제된 파일

출력 예시:

```text
Feature: coupon-v2

[backend]
MODIFIED src/main/java/com/company/coupon/CouponService.java
ADDED    src/main/java/com/company/coupon/CouponPolicy.java
DELETED  src/main/resources/old-coupon.yml

[admin-front]
MODIFIED src/pages/CouponPage.vue

[app-front]
NO CHANGES
```

MVP에서는 파일 단위 변경 목록만 출력해도 된다.
가능하면 `--patch` 옵션으로 내용 diff도 지원한다.

```bash
agentsafe agent diff coupon-v2 --patch
```

## 9.11 `agentsafe agent sync`

에이전트 작업공간의 변경사항을 원본 worktree로 반영한다.

```bash
agentsafe agent sync coupon-v2
```

동작:

1. agent diff를 계산한다.
2. 변경사항 목록을 출력한다.
3. 위험 파일 여부를 검사한다.
4. 마스킹된 파일이 변경되었으면 경고한다.
5. 사용자에게 승인 여부를 묻는다.
6. 승인 시 파일을 worktree로 복사/삭제한다.
7. sync 결과를 출력한다.

위험 파일 예시:

* `.env`
* `*.pem`
* `*.key`
* `*.jks`
* `application-secret.yml`
* `application-local.yml`
* `agentsafe.yaml`

기본 정책:

* 위험 파일은 기본적으로 sync하지 않는다.
* `--include-risky` 옵션이 있을 때만 sync 가능하다.
* 그래도 사용자에게 한 번 더 확인을 받는다.

예시:

```bash
agentsafe agent sync coupon-v2
```

출력:

```text
Changes to sync:

[backend]
MODIFIED src/main/java/com/company/coupon/CouponService.java
ADDED    src/main/java/com/company/coupon/CouponPolicy.java

[admin-front]
MODIFIED src/pages/CouponPage.vue

Proceed? [y/N]:
```

옵션:

```bash
agentsafe agent sync coupon-v2 --repo backend
agentsafe agent sync coupon-v2 --dry-run
agentsafe agent sync coupon-v2 --include-risky
```

## 9.12 `agentsafe test`

기능 작업공간에서 저장소별 테스트 명령을 실행한다.

```bash
agentsafe test coupon-v2
```

설정 파일에 저장소별 테스트 명령을 추가할 수 있게 한다.

예시:

```yaml
repositories:
  - name: backend
    url: https://gitlab.example.com/company/backend.git
    type: backend
    testCommand: ./gradlew test

  - name: admin-front
    url: https://gitlab.example.com/company/admin-front.git
    type: frontend
    testCommand: npm run test

  - name: app-front
    url: https://gitlab.example.com/company/app-front.git
    type: frontend
    testCommand: npm run test
```

동작:

```bash
cd worktrees/coupon-v2/backend
./gradlew test
```

```bash
cd worktrees/coupon-v2/admin-front
npm run test
```

```bash
cd worktrees/coupon-v2/app-front
npm run test
```

MVP에서는 순차 실행으로 충분하다.
추후 병렬 실행을 고려해 내부 구조는 확장 가능하게 만든다.

## 9.13 `agentsafe commit`

모든 저장소의 변경사항을 저장소별로 커밋한다.

```bash
agentsafe commit coupon-v2 -m "feat: add coupon v2"
```

동작:

각 worktree에서:

```bash
git add .
git commit -m "feat: add coupon v2"
```

주의:

* 변경사항이 없는 저장소는 skip한다.
* 커밋 실패 시 해당 저장소만 실패로 표시한다.
* 전체 결과 요약을 출력한다.

## 9.14 `agentsafe push`

모든 저장소의 feature branch를 origin으로 push한다.

```bash
agentsafe push coupon-v2
```

동작:

각 worktree에서:

```bash
git push -u origin feature/coupon-v2
```

## 9.15 `agentsafe mr create`

각 저장소에 GitLab Merge Request를 생성한다.

```bash
agentsafe mr create coupon-v2 --target develop
```

동작:

1. config의 GitLab baseUrl과 tokenEnv를 읽는다.
2. 환경변수에서 GitLab token을 읽는다.
3. 각 저장소별 project id를 URL로부터 추론하거나 GitLab API로 검색한다.
4. source branch는 `feature/{featureName}`.
5. target branch는 `develop`.
6. MR 제목은 기본적으로 `[coupon-v2] {repoName}` 형태.
7. MR 설명에는 멀티레포 기능 세트 정보를 포함한다.

MR 설명 예시:

```markdown
## Feature Set

Feature: coupon-v2

This MR is part of a multi-repository feature set.

Related repositories:

- backend: feature/coupon-v2
- admin-front: feature/coupon-v2
- app-front: feature/coupon-v2

## Notes

Generated by agentsafe.
```

옵션:

```bash
agentsafe mr create coupon-v2 --target develop --title "feat: coupon v2"
```

MVP에서 GitLab API 구현이 복잡하면 다음 중 하나로 처리해도 된다.

1. API 구현
2. MR 생성 URL 출력
3. `glab` CLI가 설치되어 있으면 `glab mr create` 호출

우선순위는 다음과 같다.

1. GitLab API 직접 호출
2. `glab` CLI fallback
3. MR 생성 URL 안내

## 9.16 `agentsafe cleanup`

기능 작업공간을 정리한다.

```bash
agentsafe cleanup coupon-v2
```

동작:

1. agent 폴더 삭제 여부 확인
2. worktree 삭제 여부 확인
3. Git worktree remove 수행
4. feature 메타데이터 삭제 여부 확인

예시 내부 명령:

```bash
git -C repos/backend worktree remove ../../worktrees/coupon-v2/backend
```

옵션:

```bash
agentsafe cleanup coupon-v2 --agent-only
agentsafe cleanup coupon-v2 --force
```

## 10. MVP에서 반드시 구현할 기능

다음 기능은 반드시 구현한다.

1. `init`
2. `repo add`
3. `repo list`
4. `clone`
5. `feature create`
6. `feature list`
7. `status`
8. `agent prepare`
9. `agent diff`
10. `agent sync`
11. `commit`
12. `push`

GitLab MR 생성은 MVP 후반부 기능으로 두되, 인터페이스와 config 구조는 미리 설계한다.

## 11. MVP에서 제외할 기능

다음은 MVP에서 구현하지 않는다.

1. 완전한 tmux 스타일 터미널 멀티플렉서
2. GUI
3. 백그라운드 데몬
4. 실시간 파일 감시 자동 sync
5. 양방향 conflict merge
6. 마스킹된 값을 원본 secret으로 복원하는 기능
7. 복잡한 권한 관리
8. 여러 사용자의 협업 세션 공유
9. 원격 개발 서버 접속 관리

단, 추후 확장 가능하도록 패키지 구조는 분리한다.

## 12. 보안 정책

이 도구의 핵심은 코딩 에이전트가 민감정보를 보지 못하게 하는 것이다.

따라서 다음 정책을 지켜라.

### 12.1 에이전트 작업공간에는 `.git` 복사 금지

에이전트가 Git history, remote URL, commit metadata 등을 읽지 못하도록 `.git`은 항상 제외한다.

### 12.2 민감 파일 기본 제외

다음 파일은 기본 제외한다.

```text
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
```

### 12.3 마스킹된 파일 sync 주의

마스킹된 파일을 agent workspace에서 수정한 경우, 원본으로 sync하면 실제 설정이 깨질 수 있다.

따라서:

* 마스킹된 파일 목록을 메타데이터로 저장한다.
* sync 시 마스킹된 파일이 변경되었으면 기본적으로 차단한다.
* `--allow-masked-sync` 옵션이 있을 때만 허용한다.
* 허용하더라도 강한 경고를 출력한다.

### 12.4 위험 파일 sync 차단

`agentsafe.yaml`, `.env`, secret 계열 파일은 기본적으로 sync하지 않는다.

### 12.5 작업 전 diff 필수

`agent sync`는 항상 diff를 먼저 보여줘야 한다.

`--yes` 옵션이 없는 한 사용자 승인을 받아야 한다.

## 13. 파일 복사 정책

`agent prepare`는 단순 복사가 아니라 “안전 복사”다.

동작 순서:

1. 원본 worktree 경로 확인
2. 대상 agent 경로 초기화
3. ignore rule 로딩
4. 파일 트리 순회
5. 제외 대상이면 skip
6. 디렉토리 생성
7. 텍스트 파일이면 마스킹 적용 후 복사
8. 바이너리 파일이면 그대로 복사
9. 복사 결과 메타데이터 저장

메타데이터 예시:

```json
{
  "feature": "coupon-v2",
  "preparedAt": "2026-05-28T10:30:00+09:00",
  "repositories": [
    {
      "name": "backend",
      "source": "worktrees/coupon-v2/backend",
      "agent": "agent/coupon-v2/backend",
      "copiedFiles": 320,
      "ignoredFiles": 18,
      "maskedFiles": [
        "src/main/resources/application.yml"
      ]
    }
  ]
}
```

## 14. sync 정책

`agent sync`는 agent 폴더에서 원본 worktree로 변경사항을 반영한다.

### 14.1 변경 감지

다음 기준으로 파일 변경을 감지한다.

* 파일 존재 여부
* 파일 크기
* SHA-256 hash

변경 유형:

* `ADDED`
* `MODIFIED`
* `DELETED`

### 14.2 삭제 처리

agent 폴더에서 삭제된 파일은 원본 worktree에서도 삭제 대상으로 판단한다.

단, 다음 파일은 삭제하지 않는다.

* `.git`
* config에서 제외된 위험 파일
* ignore 대상 파일
* sync 보호 대상 파일

### 14.3 dry-run

```bash
agentsafe agent sync coupon-v2 --dry-run
```

실제 반영하지 않고 변경 목록만 출력한다.

## 15. Windows 지원

MVP는 Windows 개발자를 주요 대상으로 한다.

주의사항:

1. 경로 구분자는 `filepath.Join`을 사용한다.
2. 절대 경로와 상대 경로를 안전하게 처리한다.
3. PowerShell, CMD, Git Bash 환경에서 동작해야 한다.
4. 심볼릭 링크는 MVP에서 복사하지 않고 일반 파일로 처리하거나 skip한다.
5. 긴 경로 문제를 고려해 에러 메시지를 명확히 출력한다.

## 16. 에러 처리 원칙

사용자 친화적인 에러 메시지를 제공한다.

나쁜 예:

```text
exit status 128
```

좋은 예:

```text
Failed to create worktree for repository backend.
Command: git worktree add ...
Reason: branch feature/coupon-v2 already exists.
Suggestion: run `agentsafe feature create coupon-v2 --reuse-branch` or delete the existing branch.
```

## 17. 내부 패키지 책임

### 17.1 `internal/config`

* config 파일 로딩/저장
* workspace root 탐색
* 기본 설정 생성
* repository 설정 추가/삭제

### 17.2 `internal/git`

* git 명령 실행 wrapper
* fetch, pull, branch 확인
* worktree add/remove/list
* status, commit, push

### 17.3 `internal/repo`

* repository clone
* repository list
* repository path 계산

### 17.4 `internal/feature`

* feature metadata 생성/조회/삭제
* feature name 검증
* branch name 생성

### 17.5 `internal/agent`

* agent workspace prepare
* ignore rule 처리
* mask rule 처리
* diff 계산
* sync 수행

### 17.6 `internal/gitlab`

* GitLab API client
* project 검색
* MR 생성
* MR URL 생성

### 17.7 `internal/session`

* 추후 terminal session 확장을 위한 메타데이터 구조
* MVP에서는 session 파일 생성 정도만 구현

### 17.8 `internal/fsutil`

* 파일 복사
* 디렉토리 순회
* hash 계산
* 텍스트 파일 판단
* 안전 경로 처리

### 17.9 `internal/ui`

* 사용자 확인 prompt
* 표 출력
* 경고 메시지 출력

## 18. CLI UX 예시

전체 사용 흐름은 다음과 같아야 한다.

```bash
mkdir D:/workspace/my-service
cd D:/workspace/my-service

agentsafe init --name my-service

agentsafe repo add backend https://gitlab.example.com/company/backend.git --type backend
agentsafe repo add admin-front https://gitlab.example.com/company/admin-front.git --type frontend
agentsafe repo add app-front https://gitlab.example.com/company/app-front.git --type frontend

agentsafe clone

agentsafe feature create coupon-v2 --base develop

agentsafe agent prepare coupon-v2

agentsafe agent open coupon-v2 --editor cursor
```

이후 사용자는 Cursor, Claude Code, Codex 등의 코딩 에이전트를 `agent/coupon-v2` 폴더에서 실행한다.

에이전트 작업 후:

```bash
agentsafe agent diff coupon-v2
agentsafe agent sync coupon-v2
agentsafe status coupon-v2
agentsafe test coupon-v2
agentsafe commit coupon-v2 -m "feat: add coupon v2"
agentsafe push coupon-v2
agentsafe mr create coupon-v2 --target develop
```

## 19. 구현 순서

다음 순서로 구현해라.

### Phase 1: CLI 골격

1. Go module 생성
2. cobra 기반 CLI 생성
3. `agentsafe --help` 동작
4. root command와 하위 command 구조 생성

### Phase 2: workspace/config

1. `agentsafe init`
2. config 파일 생성
3. workspace root 탐색
4. `repo add`
5. `repo list`

### Phase 3: clone/worktree

1. `clone`
2. git command wrapper
3. `feature create`
4. feature metadata 저장
5. `feature list`
6. `status`

### Phase 4: agent prepare

1. ignore rule 로딩
2. 파일 복사
3. default exclude 처리
4. mask rule 처리
5. prepare metadata 저장

### Phase 5: diff/sync

1. agent/worktree 파일 트리 비교
2. added/modified/deleted 감지
3. diff 출력
4. sync dry-run
5. sync 실제 반영
6. 위험 파일 보호

### Phase 6: commit/push

1. 저장소별 변경 확인
2. commit
3. push
4. 결과 요약

### Phase 7: GitLab MR skeleton

1. config에 GitLab 설정 추가
2. token env 읽기
3. MR 생성 URL 출력
4. 가능하면 API 직접 구현

## 20. 테스트 시나리오

다음 테스트가 가능해야 한다.

### 20.1 기본 초기화

```bash
agentsafe init --name demo
```

기대:

* `.agentsafe/config.yaml` 생성
* 기본 디렉토리 생성

### 20.2 저장소 등록

```bash
agentsafe repo add backend https://gitlab.example.com/demo/backend.git --type backend
agentsafe repo list
```

기대:

* config에 저장소 추가
* list에서 출력

### 20.3 기능 워크트리 생성

```bash
agentsafe feature create login-v2 --base develop
```

기대:

* 모든 저장소에 `feature/login-v2` worktree 생성
* `.agentsafe/features/login-v2.json` 생성

### 20.4 에이전트 폴더 생성

```bash
agentsafe agent prepare login-v2
```

기대:

* `agent/login-v2` 폴더 생성
* `.git`, `.env`, `node_modules` 제외
* agentsafe.yaml의 mask 규칙 적용

### 20.5 에이전트 변경 sync

agent 폴더에서 파일 수정 후:

```bash
agentsafe agent diff login-v2
agentsafe agent sync login-v2
```

기대:

* 변경 목록 출력
* 승인 후 worktree에 반영

## 21. 구현 시 주의사항

1. MVP라도 코드 구조는 확장 가능하게 작성한다.
2. 비즈니스 로직을 cobra command 안에 직접 많이 넣지 않는다.
3. command는 input parsing과 output만 담당한다.
4. 실제 로직은 internal 패키지로 분리한다.
5. Git 명령 실패 시 stdout/stderr를 함께 수집한다.
6. Windows path 처리를 반드시 고려한다.
7. 파일 복사 시 경로 traversal 취약점이 없도록 한다.
8. agent sync가 작업공간 밖의 파일을 수정하지 못하게 한다.
9. destructive action에는 confirmation을 둔다.
10. 모든 명령어는 dry-run 옵션을 고려해 설계한다.

## 22. 산출물

다음을 산출해라.

1. 전체 프로젝트 코드
2. `README.md`
3. `docs/mvp.md`
4. `examples/config.yaml`
5. `examples/agentsafe.yaml`
6. 기본 사용 예시
7. 주요 명령어 help 문구
8. Windows 기준 실행 예시

## 23. README에 포함할 내용

README에는 다음 내용을 포함해라.

```markdown
# agentsafe

agentsafe is a multi-repository safe workspace manager for AI coding agents.

## Why

Many enterprise services are split across multiple repositories.
A single feature often requires changes in backend, admin frontend, and app frontend repositories.
At the same time, AI coding agents should not access secrets, local configs, or sensitive files.

agentsafe solves this by:

- managing multiple repositories as one feature workspace
- creating Git worktrees for each repository
- creating sanitized agent workspaces
- syncing agent changes back to real worktrees after review
- preparing future support for GitLab merge requests and terminal sessions

## Basic Workflow

...
```

## 24. 최종 구현 목표

최종적으로 다음 명령 흐름이 실제로 동작해야 한다.

```bash
agentsafe init --name my-service

agentsafe repo add backend https://gitlab.example.com/company/backend.git --type backend
agentsafe repo add admin-front https://gitlab.example.com/company/admin-front.git --type frontend
agentsafe repo add app-front https://gitlab.example.com/company/app-front.git --type frontend

agentsafe clone

agentsafe feature create coupon-v2 --base develop

agentsafe agent prepare coupon-v2

# user runs coding agent in agent/coupon-v2

agentsafe agent diff coupon-v2
agentsafe agent sync coupon-v2

agentsafe status coupon-v2
agentsafe commit coupon-v2 -m "feat: add coupon v2"
agentsafe push coupon-v2
```

구현을 시작하기 전에 먼저 전체 설계 요약, 디렉토리 구조, 명령어 목록, 데이터 모델을 제안한 뒤 코드를 작성해라.
