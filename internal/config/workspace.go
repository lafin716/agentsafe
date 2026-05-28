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
