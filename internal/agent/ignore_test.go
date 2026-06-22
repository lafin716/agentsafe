package agent

import "testing"

func TestIgnoreMatcherTreatsNonGlobPatternsAsRepoRootRelative(t *testing.T) {
	matcher := NewIgnoreMatcher([]string{"membership/"})

	if !matcher.Match("membership", true) {
		t.Fatal("root membership directory should match")
	}
	if !matcher.Match("membership/file.txt", false) {
		t.Fatal("files inside root membership directory should match")
	}
	if matcher.Match("test/membership", true) {
		t.Fatal("nested membership directory should not match without a glob")
	}
	if matcher.Match("test/membership/file.txt", false) {
		t.Fatal("files inside nested membership directory should not match without a glob")
	}
}

func TestIgnoreMatcherKeepsGlobPatternsUnanchored(t *testing.T) {
	matcher := NewIgnoreMatcher([]string{"*.pem", "*/membership/"})

	if !matcher.Match("secret.pem", false) {
		t.Fatal("root glob match failed")
	}
	if !matcher.Match("test/secret.pem", false) {
		t.Fatal("basename glob should match nested files")
	}
	if !matcher.Match("test/membership", true) {
		t.Fatal("directory glob should match nested membership directory")
	}
	if !matcher.Match("test/membership/file.txt", false) {
		t.Fatal("directory glob should match files inside nested membership directory")
	}
}
