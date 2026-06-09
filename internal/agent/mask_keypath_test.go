package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyKeyPathsJSON(t *testing.T) {
	m := MaskFile{Rules: []MaskRule{
		{Name: "nested", Type: "keypath", Pattern: "main.sub", Replacement: "__MASKED__"},
	}}
	in := `{"main":{"sub":"1234","keep":"ok"},"other":"x"}`
	out, changed := m.ApplyKeyPaths(in, ".json")
	if !changed {
		t.Fatalf("expected change")
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not valid json: %v", err)
	}
	main := got["main"].(map[string]interface{})
	if main["sub"] != "__MASKED__" {
		t.Errorf("sub = %v, want __MASKED__", main["sub"])
	}
	if main["keep"] != "ok" {
		t.Errorf("keep changed: %v", main["keep"])
	}
	if got["other"] != "x" {
		t.Errorf("other changed: %v", got["other"])
	}
}

func TestApplyKeyPathsYAML(t *testing.T) {
	m := MaskFile{Rules: []MaskRule{
		{Name: "db", Type: "key", Pattern: "spring.datasource.password"}, // empty repl -> default
	}}
	in := "spring:\n  datasource:\n    password: secret\n    url: jdbc://x\n"
	out, changed := m.ApplyKeyPaths(in, ".yml")
	if !changed {
		t.Fatalf("expected change")
	}
	if !strings.Contains(out, "__MASKED__") {
		t.Errorf("expected masked value, got: %s", out)
	}
	if !strings.Contains(out, "jdbc://x") {
		t.Errorf("unrelated value lost: %s", out)
	}
}

func TestApplyKeyPathsMultiDocYAML(t *testing.T) {
	m := MaskFile{Rules: []MaskRule{
		{Name: "pw", Type: "keypath", Pattern: "db.password", Replacement: "__MASKED__"},
	}}
	in := "env: local\ndb:\n  password: localpw\n  url: u1\n" +
		"---\n" +
		"env: dev\ndb:\n  password: devpw\n  url: u2\n" +
		"---\n" +
		"env: prod\ndb:\n  password: prodpw\n  url: u3\n"
	out, changed := m.ApplyKeyPaths(in, ".yaml")
	if !changed {
		t.Fatalf("expected change")
	}
	// all three documents must survive
	for _, env := range []string{"local", "dev", "prod"} {
		if !strings.Contains(out, "env: "+env) {
			t.Errorf("document for env=%s missing in output:\n%s", env, out)
		}
	}
	// all passwords masked, none of the originals remain
	for _, pw := range []string{"localpw", "devpw", "prodpw"} {
		if strings.Contains(out, pw) {
			t.Errorf("password %s not masked:\n%s", pw, out)
		}
	}
	if strings.Count(out, "__MASKED__") != 3 {
		t.Errorf("expected 3 masked values, got:\n%s", out)
	}
	// unrelated values preserved
	for _, u := range []string{"u1", "u2", "u3"} {
		if !strings.Contains(out, u) {
			t.Errorf("unrelated value %s lost:\n%s", u, out)
		}
	}
}

func TestApplyKeyPathsNoMatchOrNonStructured(t *testing.T) {
	m := MaskFile{Rules: []MaskRule{
		{Name: "missing", Type: "keypath", Pattern: "a.b.c"},
	}}
	in := `{"main":{"sub":"1234"}}`
	if out, changed := m.ApplyKeyPaths(in, ".json"); changed || out != in {
		t.Errorf("missing path should not change content")
	}
	// non-structured extension is skipped
	if out, changed := m.ApplyKeyPaths(in, ".txt"); changed || out != in {
		t.Errorf("non-structured ext should not change content")
	}
	// no keypath rules -> unchanged
	plain := MaskFile{Rules: []MaskRule{{Type: "plain", Pattern: "x", Replacement: "y"}}}
	if out, changed := plain.ApplyKeyPaths(in, ".json"); changed || out != in {
		t.Errorf("no keypath rules should not change content")
	}
}
