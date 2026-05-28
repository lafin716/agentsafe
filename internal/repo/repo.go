package repo

import (
	"fmt"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/ui"
)

func List(cfg config.Config) {
	rows := [][]string{}
	for _, r := range cfg.Repositories {
		rows = append(rows, []string{r.Name, r.Type, r.URL})
	}
	ui.PrintRows([]string{"NAME", "TYPE", "URL"}, rows)
}

func EnsureConfigured(cfg config.Config) error {
	if len(cfg.Repositories) == 0 {
		return fmt.Errorf("no repositories configured; use `agentsafe repo add ...` first")
	}
	return nil
}
