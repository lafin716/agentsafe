package config

import "path/filepath"

// RepoPath는 원본 클론이 놓이는 main/<repo> 경로를 돌려준다.
func RepoPath(root, name string) string { return filepath.Join(root, "main", name) }

// WorktreePath는 특정 feature의 Git 워크트리 경로 feature/<feature>/<repo>를 돌려준다.
func WorktreePath(root, feature, repo string) string {
	return filepath.Join(root, "feature", feature, repo)
}

// AgentPath는 에이전트에게 노출되는 사본 디렉터리 agent/<feature>/<repo>를 돌려준다.
func AgentPath(root, feature, repo string) string { return filepath.Join(root, "agent", feature, repo) }

// FeatureMetaPath는 feature 메타데이터 파일 .agentsafe/features/<feature>.json 경로를 돌려준다.
func FeatureMetaPath(root, feature string) string {
	return filepath.Join(root, DirName, "features", feature+".json")
}

// SessionMetaPath는 feature의 세션 상태 파일 .agentsafe/sessions/<feature>.json 경로를 돌려준다.
func SessionMetaPath(root, feature string) string {
	return filepath.Join(root, DirName, "sessions", feature+".json")
}

// HistoryDir은 sync 히스토리 저장소의 루트 디렉터리다.
func HistoryDir(root string) string { return filepath.Join(root, DirName, "history") }

// HistoryRepoDir은 저장소별 sync 기록 스택이 쌓이는 디렉터리다.
func HistoryRepoDir(root, feature, repo string) string {
	return filepath.Join(HistoryDir(root), feature, repo)
}

// HistoryEntryDir은 sync 기록 하나(manifest.json + backup/)가 담기는 디렉터리다.
func HistoryEntryDir(root, feature, repo, id string) string {
	return filepath.Join(HistoryRepoDir(root, feature, repo), id)
}
