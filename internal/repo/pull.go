package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/fsutil"
	aggit "github.com/agentsafe/agentsafe/internal/git"
	"github.com/agentsafe/agentsafe/internal/output"
)

func PullAll(root string, cfg config.Config) error {
	if err := EnsureConfigured(cfg); err != nil {
		return err
	}
	var failed int
	for _, r := range cfg.Repositories {
		if err := pullRepository(root, cfg, r, "", ""); err != nil {
			failed++
			output.Printf("failed: %v\n", err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("pull completed with %d failure(s)", failed)
	}
	return nil
}

// PullOne clones or updates one configured repository.
func PullOne(root string, cfg config.Config, name string) error {
	return PullOneWithCredentials(root, cfg, name, "", "")
}

// PullOneWithCredentials clones or updates one repository using command-scoped
// HTTPS credentials. Empty credentials preserve the normal non-interactive
// behavior.
func PullOneWithCredentials(root string, cfg config.Config, name, username, secret string) error {
	for _, r := range cfg.Repositories {
		if r.Name == name {
			return pullRepository(root, cfg, r, username, secret)
		}
	}
	return fmt.Errorf("repository %q not found", name)
}

func pullRepository(root string, cfg config.Config, r config.Repository, username, secret string) error {
	dest := config.RepoPath(root, r.Name)
	run := func(dir string, args ...string) error {
		var err error
		if username != "" || secret != "" {
			_, err = aggit.RunWithHTTPAuth(dir, r.URL, username, secret, args...)
		} else {
			_, err = aggit.Run(dir, args...)
		}
		return err
	}
	output.Printf("[%s] ", r.Name)
	st, statErr := os.Stat(dest)
	// An empty leftover directory (e.g. from a failed clone or partial removal)
	// is treated as "not cloned" so the pull action clones into it. git clone
	// accepts an existing empty target directory.
	empty := false
	if statErr == nil && st.IsDir() {
		if ok, e := fsutil.IsEmptyDir(dest); e == nil {
			empty = ok
		}
	}
	if os.IsNotExist(statErr) || empty {
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		output.Printf("cloning %s...\n", r.URL)
		if err := run(root, "clone", r.URL, dest); err != nil {
			return err
		}
		output.Println("cloned")
		return nil
	} else if statErr != nil {
		return fmt.Errorf("inspect repository path: %w", statErr)
	}

	output.Printf("fetching...")
	if err := run(dest, "fetch", "--all", "--prune"); err != nil {
		return err
	}
	branch := r.DefaultBranch
	if branch == "" {
		branch = cfg.Git.DefaultBaseBranch
	}
	cur, _ := aggit.CurrentBranch(dest)
	if cur == branch && !aggit.HasChanges(dest) {
		if err := run(dest, "pull", "--ff-only", "origin", branch); err != nil {
			return fmt.Errorf("fetched, but pull failed: %w", err)
		}
		output.Printf(" pulled (ff) %s\n", branch)
	} else {
		output.Printf(" fetched (skipped pull: not on %s or has local changes)\n", branch)
	}
	return nil
}
