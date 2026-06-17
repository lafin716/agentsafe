package mobilebridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agentsafe/agentsafe/packages/core"
)

// MobileService exposes a gomobile-friendly API for Android callers.
type MobileService struct {
	svc *core.Service
}

// NewMobileService constructs a service for gomobile/Android callers.
func NewMobileService() *MobileService {
	return &MobileService{svc: core.NewService()}
}

// RunJson accepts a JSON-encoded core.RunInput and returns a JSON-encoded core.RunResult.
func (m *MobileService) RunJson(inputJson string) (string, error) {
	if m == nil {
		m = NewMobileService()
	}

	var input core.RunInput
	if err := json.Unmarshal([]byte(inputJson), &input); err != nil {
		return "", fmt.Errorf("invalid RunInput JSON: %w", err)
	}

	result, err := m.svc.Run(context.Background(), input)
	if err != nil {
		return "", err
	}

	output, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal RunResult JSON: %w", err)
	}
	return string(output), nil
}
