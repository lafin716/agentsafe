package config

import (
	"crypto/sha1"
	"encoding/hex"
)

// FeatureKey derives a stable ASCII-safe folder key from a feature name.
// The key is used for on-disk worktree/agent folders, while the original
// feature name is still used for the Git branch and metadata.
//
// FeatureKey does not guarantee global uniqueness across existing features;
// callers that touch the filesystem should resolve collisions separately.
func FeatureKey(name string) string {
	return "feat-" + shortHash(name)
}

// shortHash returns the first 6 hex chars of the SHA-1 of s.
func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:6]
}
