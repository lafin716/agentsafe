package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/output"
)

func Delete(root, featureName string) error {
	agentDir := filepath.Join(root, "agent", featureName)
	sessionMeta := config.SessionMetaPath(root, featureName)

	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		return fmt.Errorf("agent workspace for %q does not exist", featureName)
	}

	output.Printf("Deleting agent workspace: %s\n", agentDir)
	if err := os.RemoveAll(agentDir); err != nil {
		return fmt.Errorf("failed to remove agent workspace: %w", err)
	}

	if _, err := os.Stat(sessionMeta); err == nil {
		output.Printf("Deleting session metadata: %s\n", sessionMeta)
		if err := os.Remove(sessionMeta); err != nil {
			return fmt.Errorf("failed to remove session metadata: %w", err)
		}
	}
	return nil
}
