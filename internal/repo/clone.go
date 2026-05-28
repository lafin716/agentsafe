package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentsafe/agentsafe/internal/config"
	aggit "github.com/agentsafe/agentsafe/internal/git"
)

func CloneAll(root string, cfg config.Config) error {
	if err := EnsureConfigured(cfg); err != nil {
		return err
	}
	var failed int
	for _, r := range cfg.Repositories {
		dest := config.RepoPath(root, r.Name)
		fmt.Printf("[%s] ", r.Name)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			fmt.Printf("cloning %s...\n", r.URL)
			if _, err := aggit.Run(root, "clone", r.URL, dest); err != nil {
				failed++
				fmt.Printf("failed: %v\n", err)
			} else {
				fmt.Println("cloned")
			}
		} else {
			fmt.Println("fetching...")
			if err := aggit.FetchAll(dest); err != nil {
				failed++
				fmt.Printf("failed: %v\n", err)
			} else {
				fmt.Println("fetched")
			}
		}
	}
	if failed > 0 {
		return fmt.Errorf("clone/fetch completed with %d failure(s)", failed)
	}
	return nil
}
