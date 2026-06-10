package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentsafe/agentsafe/internal/config"
	aggit "github.com/agentsafe/agentsafe/internal/git"
	"github.com/agentsafe/agentsafe/internal/output"
)

func PullAll(root string, cfg config.Config) error {
	if err := EnsureConfigured(cfg); err != nil {
		return err
	}
	var failed int
	for _, r := range cfg.Repositories {
		dest := config.RepoPath(root, r.Name)
		output.Printf("[%s] ", r.Name)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			output.Printf("cloning %s...\n", r.URL)
			if _, err := aggit.Run(root, "clone", r.URL, dest); err != nil {
				failed++
				output.Printf("failed: %v\n", err)
			} else {
				output.Println("cloned")
			}
		} else {
			output.Printf("fetching...")
			if err := aggit.FetchAll(dest); err != nil {
				failed++
				output.Printf(" failed: %v\n", err)
				continue
			}
			branch := r.DefaultBranch
			if branch == "" {
				branch = cfg.Git.DefaultBaseBranch
			}
			cur, _ := aggit.CurrentBranch(dest)
			if cur == branch && !aggit.HasChanges(dest) {
				if err := aggit.Pull(dest, "origin", branch); err != nil {
					failed++
					output.Printf(" fetched, but pull failed: %v\n", err)
				} else {
					output.Printf(" pulled (ff) %s\n", branch)
				}
			} else {
				output.Printf(" fetched (skipped pull: not on %s or has local changes)\n", branch)
			}
		}
	}
	if failed > 0 {
		return fmt.Errorf("pull completed with %d failure(s)", failed)
	}
	return nil
}
