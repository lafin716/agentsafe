package mobilebridge

import (
	"encoding/json"
	"testing"
)

func TestRunJson(t *testing.T) {
	svc := NewMobileService()
	out, err := svc.RunJson(`{"text":"hello"}`)
	if err != nil {
		t.Fatalf("RunJson returned error: %v", err)
	}
	var decoded struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if decoded.Output != "processed: hello" {
		t.Fatalf("Output = %q", decoded.Output)
	}
}
