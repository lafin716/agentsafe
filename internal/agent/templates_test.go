package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
)

func TestTemplatesParseAndNonEmpty(t *testing.T) {
	infos := TemplateList()
	if len(infos) == 0 {
		t.Fatal("expected at least one template")
	}
	wantKeys := map[string]bool{"spring": false, "react": false, "vue": false, "next": false, "nuxt": false, "k8s": false}
	for _, info := range infos {
		tpl, err := GetTemplate(info.Key)
		if err != nil {
			t.Fatalf("GetTemplate(%q): %v", info.Key, err)
		}
		if len(tpl.Ignore) == 0 && len(tpl.Mask) == 0 {
			t.Fatalf("template %q is empty", info.Key)
		}
		if tpl.Label == "" {
			t.Fatalf("template %q missing label", info.Key)
		}
		if _, ok := wantKeys[info.Key]; ok {
			wantKeys[info.Key] = true
		}
	}
	for k, found := range wantKeys {
		if !found {
			t.Fatalf("expected template %q to exist", k)
		}
	}
}

func TestGetTemplateUnknown(t *testing.T) {
	if _, err := GetTemplate("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestMergeSecurityDedupe(t *testing.T) {
	base := SecurityFile{
		Ignore: []string{".env", "build/"},
		Mask: []MaskRule{
			{Name: "Existing", Type: "regex", Pattern: "AKIA[0-9A-Z]{16}", Replacement: "__KEEP__"},
		},
	}
	add := SecurityFile{
		Ignore: []string{"build/", "node_modules/", ".env"},
		Mask: []MaskRule{
			// Same (type, pattern) as base — should be dropped, base wins.
			{Name: "Template", Type: "regex", Pattern: "AKIA[0-9A-Z]{16}", Replacement: "__OVERRIDE__"},
			{Name: "New", Type: "plain", Pattern: "secret", Replacement: "__X__"},
		},
	}
	out := MergeSecurity(base, add)

	wantIgnore := []string{".env", "build/", "node_modules/"}
	if len(out.Ignore) != len(wantIgnore) {
		t.Fatalf("ignore = %v, want %v", out.Ignore, wantIgnore)
	}
	for i, p := range wantIgnore {
		if out.Ignore[i] != p {
			t.Fatalf("ignore[%d] = %q, want %q (order preserved)", i, out.Ignore[i], p)
		}
	}

	if len(out.Mask) != 2 {
		t.Fatalf("mask len = %d, want 2: %+v", len(out.Mask), out.Mask)
	}
	if out.Mask[0].Name != "Existing" || out.Mask[0].Replacement != "__KEEP__" {
		t.Fatalf("first mask should be the base rule, got %+v", out.Mask[0])
	}
	if out.Mask[1].Name != "New" {
		t.Fatalf("second mask should be the new rule, got %+v", out.Mask[1])
	}
}

func TestApplyTemplatesMergeAndReplace(t *testing.T) {
	dir := t.TempDir()
	// Seed an existing unified file with a manual rule.
	existing := SecurityFile{
		Ignore: []string{"my-custom-secret.txt"},
		Mask:   []MaskRule{{Name: "Manual", Type: "plain", Pattern: "hunter2", Replacement: "__X__"}},
	}
	if err := WriteSecurity(config.Config{}, dir, existing); err != nil {
		t.Fatal(err)
	}

	// Merge: existing content preserved + template added.
	merged, err := ApplyTemplates(config.Config{}, dir, []string{"spring"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(merged.Ignore, "my-custom-secret.txt") {
		t.Fatalf("merge dropped existing ignore: %v", merged.Ignore)
	}
	if !hasMaskNamed(merged.Mask, "Manual") {
		t.Fatalf("merge dropped existing mask: %+v", merged.Mask)
	}
	if !contains(merged.Ignore, "*.jks") {
		t.Fatalf("merge missing spring ignore: %v", merged.Ignore)
	}
	// Persisted to disk.
	onDisk, err := LoadSecurityFile(filepath.Join(dir, "agentsafe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(onDisk.Ignore) != len(merged.Ignore) {
		t.Fatalf("disk/return mismatch: %d vs %d", len(onDisk.Ignore), len(merged.Ignore))
	}

	// Replace: existing manual content dropped.
	replaced, err := ApplyTemplates(config.Config{}, dir, []string{"react"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if contains(replaced.Ignore, "my-custom-secret.txt") {
		t.Fatalf("replace should drop existing ignore: %v", replaced.Ignore)
	}
	if hasMaskNamed(replaced.Mask, "Manual") {
		t.Fatalf("replace should drop existing mask: %+v", replaced.Mask)
	}
	if !contains(replaced.Ignore, "node_modules/") {
		t.Fatalf("replace missing react ignore: %v", replaced.Ignore)
	}
}

func TestApplyTemplatesUnknownKey(t *testing.T) {
	dir := t.TempDir()
	if _, err := ApplyTemplates(config.Config{}, dir, []string{"spring", "bogus"}, false); err == nil {
		t.Fatal("expected error for unknown template key")
	}
	// No file should be written on error.
	if _, err := os.Stat(filepath.Join(dir, "agentsafe.yaml")); !os.IsNotExist(err) {
		t.Fatalf("agentsafe.yaml should not be written when a key is invalid")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func hasMaskNamed(rules []MaskRule, name string) bool {
	for _, r := range rules {
		if r.Name == name {
			return true
		}
	}
	return false
}
