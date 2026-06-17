package core

import (
	"context"
	"fmt"
	"strings"
)

// RunInput is the stable input shape shared by CLI, desktop, and mobile callers.
type RunInput struct {
	Text string `json:"text"`
}

// RunResult is the stable output shape shared by CLI, desktop, and mobile callers.
type RunResult struct {
	Output string `json:"output"`
}

// Service hosts business logic that can be reused by CLI, Wails desktop, and Android.
type Service struct{}

// NewService constructs a reusable core service.
func NewService() *Service { return &Service{} }

// Run executes the initial shared core operation.
func (s *Service) Run(ctx context.Context, input RunInput) (*RunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	text := strings.TrimSpace(input.Text)
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	return &RunResult{Output: "processed: " + text}, nil
}
