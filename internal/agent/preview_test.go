package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
)

// writePreviewRepo lays out a main clone with a mix of files and a saved policy
// at the workspace root, returning the workspace root.
func writePreviewRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repo := config.RepoPath(root, "demo")
	mk := func(rel, content string) {
		full := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mk("app.go", "package main\nfunc main() {}\n")           // copied as-is
	mk(".env", "TOKEN=abc\n")                                 // ignored (.env)
	mk("config.yaml", "url: http://x\npass: topsecret\n")     // masked (plain)
	mk("src/keys.txt", "key=AKIA0000000000000000 end\n")      // masked (regex)
	mk("node_modules/left-pad/index.js", "module.exports=1\n") // under ignored dir
	// Binary file: contains a NUL byte so IsTextFile reports false.
	if err := os.WriteFile(filepath.Join(repo, "logo.png"), []byte{0x89, 'P', 'N', 'G', 0x00, 0x01}, 0644); err != nil {
		t.Fatal(err)
	}

	policy := `ignore:
  - .env
  - node_modules/
mask:
  - name: pass
    type: plain
    pattern: topsecret
    replacement: __MASKED__
  - name: aws
    type: regex
    pattern: AKIA[0-9A-Z]{16}
    replacement: __MASKED__
`
	if err := os.WriteFile(filepath.Join(root, "agentsafe.yaml"), []byte(policy), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func entryByPath(res PreviewResult, path string) (PreviewEntry, bool) {
	for _, e := range res.Entries {
		if e.Path == path {
			return e, true
		}
	}
	return PreviewEntry{}, false
}

func TestScanPreviewClassifiesFiles(t *testing.T) {
	root := writePreviewRepo(t)
	res, err := ScanPreview(root, config.Config{}, "demo")
	if err != nil {
		t.Fatal(err)
	}

	// .env ignored by the ".env" pattern.
	if e, ok := entryByPath(res, ".env"); !ok || e.Status != PreviewIgnored || e.IgnorePattern != ".env" {
		t.Fatalf("expected .env ignored by .env, got %+v (ok=%v)", e, ok)
	}
	// node_modules ignored as a directory; its contents must not be enumerated.
	if e, ok := entryByPath(res, "node_modules"); !ok || e.Status != PreviewIgnored || !e.IsDir {
		t.Fatalf("expected node_modules ignored dir, got %+v (ok=%v)", e, ok)
	}
	if _, ok := entryByPath(res, "node_modules/left-pad/index.js"); ok {
		t.Fatal("contents of ignored directory should not be listed")
	}
	// config.yaml masked by the plain rule.
	if e, ok := entryByPath(res, "config.yaml"); !ok || e.Status != PreviewMasked || e.Replacements < 1 {
		t.Fatalf("expected config.yaml masked, got %+v (ok=%v)", e, ok)
	} else if len(e.MaskMatches) != 1 || e.MaskMatches[0].Name != "pass" {
		t.Fatalf("expected pass rule match, got %+v", e.MaskMatches)
	}
	// src/keys.txt masked by the regex rule.
	if e, ok := entryByPath(res, "src/keys.txt"); !ok || e.Status != PreviewMasked {
		t.Fatalf("expected src/keys.txt masked, got %+v (ok=%v)", e, ok)
	}
	// app.go copied as-is.
	if e, ok := entryByPath(res, "app.go"); !ok || e.Status != PreviewCopied {
		t.Fatalf("expected app.go copied, got %+v (ok=%v)", e, ok)
	}
	// logo.png copied (binary, not mask-evaluated).
	if e, ok := entryByPath(res, "logo.png"); !ok || e.Status != PreviewCopied || !e.Binary {
		t.Fatalf("expected logo.png copied binary, got %+v (ok=%v)", e, ok)
	}

	if res.Ignored != 2 || res.Masked != 2 || res.Copied != 2 {
		t.Fatalf("unexpected counts: ignored=%d masked=%d copied=%d", res.Ignored, res.Masked, res.Copied)
	}
	if res.Total != len(res.Entries) || res.Total != res.Ignored+res.Masked+res.Copied {
		t.Fatalf("total mismatch: total=%d entries=%d", res.Total, len(res.Entries))
	}
}

func TestScanPreviewMissingRepo(t *testing.T) {
	root := t.TempDir()
	if _, err := ScanPreview(root, config.Config{}, "nope"); err == nil {
		t.Fatal("expected error for missing repo")
	}
}

func TestPreviewFileDiff(t *testing.T) {
	root := writePreviewRepo(t)

	before, after, err := PreviewFileDiff(root, config.Config{}, "demo", "config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(before, "topsecret") {
		t.Fatalf("before should contain secret: %q", before)
	}
	if strings.Contains(after, "topsecret") || !strings.Contains(after, "__MASKED__") {
		t.Fatalf("after should be masked: %q", after)
	}
	if before == after {
		t.Fatal("masked file should differ before/after")
	}

	// Unmatched file: before == after.
	b2, a2, err := PreviewFileDiff(root, config.Config{}, "demo", "app.go")
	if err != nil {
		t.Fatal(err)
	}
	if b2 != a2 {
		t.Fatalf("unmatched file should be unchanged: %q vs %q", b2, a2)
	}
}
