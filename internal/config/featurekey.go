package config

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

// FeatureKey derives an ASCII-safe folder key from a (possibly non-ASCII)
// feature name. The key is used for on-disk worktree/agent folders so paths
// never contain characters that break tools like IntelliJ; the original
// feature name is still used for the Git branch and metadata.
//
//   - An already-clean ASCII name (e.g. "coupon-v2") is returned unchanged.
//   - Otherwise the name is slugified to its ASCII parts and a short hash of
//     the original name is appended to keep the key unique and stable, e.g.
//     "쿠폰 결제" -> "feature-1a2b3c", "쿠폰-v2" -> "v2-1a2b3c".
//
// FeatureKey does not guarantee global uniqueness across existing features;
// callers that touch the filesystem should resolve collisions separately.
func FeatureKey(name string) string {
	trimmed := strings.TrimSpace(name)
	if repoNameRE.MatchString(trimmed) && !strings.Contains(trimmed, "..") {
		// Already a safe ASCII key; keep it readable and unchanged.
		return trimmed
	}
	base := slugify(trimmed)
	hash := shortHash(trimmed)
	if base == "" {
		return "feature-" + hash
	}
	return base + "-" + hash
}

// slugify lowercases name and keeps only [a-z0-9._-], turning spaces into
// dashes and dropping everything else (including Hangul and other non-ASCII).
func slugify(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ' || r == '\t':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-._")
}

// shortHash returns the first 6 hex chars of the SHA-1 of s.
func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:6]
}
