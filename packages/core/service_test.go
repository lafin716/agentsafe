package core

import (
	"context"
	"testing"
)

func TestServiceRun(t *testing.T) {
	svc := NewService()
	result, err := svc.Run(context.Background(), RunInput{Text: "hello"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Output != "processed: hello" {
		t.Fatalf("Output = %q, want %q", result.Output, "processed: hello")
	}
}

func TestServiceRunRequiresText(t *testing.T) {
	svc := NewService()
	if _, err := svc.Run(context.Background(), RunInput{Text: "   "}); err == nil {
		t.Fatal("Run returned nil error for blank text")
	}
}
