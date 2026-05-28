# agentsafe MVP

## 설계 요약

agentsafe는 여러 Git 저장소를 하나의 기능 세트로 취급한다. 등록한 원본 저장소는 `main/`에 clone하고, 기능별 작업은 `feature/{feature}/`의 Git worktree에서 수행하며, 코딩 에이전트는 민감정보가 제거/마스킹된 `agent/{feature}/` 복사본만 사용한다.

## 디렉토리 구조

```text
.agentsafe/config.yaml
.agentsafe/features/{feature}.json
.agentsafe/sessions/{feature}.json
main/{repo}
feature/{feature}/{repo}
agent/{feature}/{repo}
```

## 명령어 목록

- `init`
- `repo add`, `repo list`
- `clone`
- `feature create`, `feature list`
- `status`
- `agent prepare`, `agent diff`, `agent sync`, `agent open`
- `commit`, `push`
- `mr create` skeleton

## 데이터 모델

- config: workspace/git/repository/agent/gitlab 설정
- feature metadata: feature name, branch, base branch, repo worktree paths
- session metadata: prepare 시 copied/ignored/masked file list

## sync 정책

`agent sync`는 항상 diff를 표시한다. 위험 파일과 마스킹된 파일은 기본 차단하며, `--include-risky`, `--allow-masked-sync`를 명시해야 한다.
