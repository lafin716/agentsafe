package config

import "path/filepath"

func RepoPath(root, name string) string { return filepath.Join(root, "main", name) }
func WorktreePath(root, feature, repo string) string {
	return filepath.Join(root, "feature", feature, repo)
}
func AgentPath(root, feature, repo string) string { return filepath.Join(root, "agent", feature, repo) }
func FeatureMetaPath(root, feature string) string {
	return filepath.Join(root, DirName, "features", feature+".json")
}
func SessionMetaPath(root, feature string) string {
	return filepath.Join(root, DirName, "sessions", feature+".json")
}

// HistoryDir is the root of the sync-history store.
func HistoryDir(root string) string { return filepath.Join(root, DirName, "history") }

// HistoryRepoDir holds the per-repository stack of sync records.
func HistoryRepoDir(root, feature, repo string) string {
	return filepath.Join(HistoryDir(root), feature, repo)
}

// HistoryEntryDir is one sync record (manifest.json + backup/).
func HistoryEntryDir(root, feature, repo, id string) string {
	return filepath.Join(HistoryRepoDir(root, feature, repo), id)
}
