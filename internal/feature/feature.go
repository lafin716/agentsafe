package feature

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentsafe/agentsafe/internal/config"
	aggit "github.com/agentsafe/agentsafe/internal/git"
	"github.com/agentsafe/agentsafe/internal/ui"
)

type Metadata struct {
	Name         string     `json:"name"`
	Branch       string     `json:"branch"`
	BaseBranch   string     `json:"baseBranch"`
	CreatedAt    string     `json:"createdAt"`
	Repositories []RepoMeta `json:"repositories"`
}
type RepoMeta struct {
	Name         string `json:"name"`
	WorktreePath string `json:"worktreePath"`
	Branch       string `json:"branch"`
}

func BranchName(cfg config.Config, featureName string) string {
	return cfg.Git.BranchPrefix + featureName
}

func Load(root, name string) (Metadata, error) {
	b, err := os.ReadFile(config.FeatureMetaPath(root, name))
	if err != nil {
		return Metadata{}, err
	}
	var m Metadata
	return m, json.Unmarshal(b, &m)
}

func Save(root string, m Metadata) error {
	if err := os.MkdirAll(filepath.Dir(config.FeatureMetaPath(root, m.Name)), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(config.FeatureMetaPath(root, m.Name), b, 0644)
}

func Create(root string, cfg config.Config, name, base string) error {
	if err := config.ValidateFeatureName(name); err != nil {
		return err
	}
	if base == "" {
		base = cfg.Git.DefaultBaseBranch
	}
	branch := BranchName(cfg, name)
	meta := Metadata{Name: name, Branch: branch, BaseBranch: base, CreatedAt: time.Now().Format(time.RFC3339)}
	for _, r := range cfg.Repositories {
		repoPath := config.RepoPath(root, r.Name)
		dest := config.WorktreePath(root, name, r.Name)
		rel, _ := filepath.Rel(root, dest)
		fmt.Printf("[%s] creating worktree %s\n", r.Name, rel)
		if _, err := os.Stat(repoPath); err != nil {
			return fmt.Errorf("repository %s is not cloned at %s; run `agentsafe clone`", r.Name, repoPath)
		}
		_ = aggit.Fetch(repoPath)
		_ = aggit.Checkout(repoPath, base)
		_ = aggit.Pull(repoPath, "origin", base)
		if _, err := os.Stat(dest); err == nil {
			fmt.Println("exists, skipping git worktree add")
		} else {
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			local := aggit.LocalBranchExists(repoPath, branch)
			remote := aggit.RemoteBranchExists(repoPath, branch)
			var err error
			switch {
			case local:
				err = aggit.AddWorktree(repoPath, dest, branch, "", false)
			case remote:
				err = aggit.AddWorktree(repoPath, dest, branch, "origin/"+branch, true)
			default:
				err = aggit.AddWorktree(repoPath, dest, branch, base, true)
			}
			if err != nil {
				return fmt.Errorf("failed to create worktree for repository %s: %w", r.Name, err)
			}
		}
		meta.Repositories = append(meta.Repositories, RepoMeta{Name: r.Name, WorktreePath: filepath.ToSlash(rel), Branch: branch})
	}
	return Save(root, meta)
}

func List(root string) error {
	dir := filepath.Join(root, config.DirName, "features")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	rows := [][]string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		var m Metadata
		if json.Unmarshal(b, &m) == nil {
			ready := "no"
			if st, err := os.Stat(filepath.Join(root, "agent", m.Name)); err == nil && st.IsDir() {
				ready = "yes"
			}
			rows = append(rows, []string{m.Name, m.Branch, m.BaseBranch, fmt.Sprint(len(m.Repositories)), ready})
		}
	}
	ui.PrintRows([]string{"FEATURE", "BRANCH", "BASE", "REPOS", "AGENT_READY"}, rows)
	return nil
}

func Status(root, name string) error {
	m, err := Load(root, name)
	if err != nil {
		return err
	}
	fmt.Printf("Feature: %s\nBranch: %s\n\n", m.Name, m.Branch)
	for _, r := range m.Repositories {
		p := filepath.Join(root, r.WorktreePath)
		fmt.Printf("[%s]\n", r.Name)
		s, err := aggit.StatusShort(p)
		if err != nil {
			fmt.Printf("ERROR: %v\n\n", err)
			continue
		}
		if s == "" {
			fmt.Println("clean")
		} else {
			fmt.Println(s)
		}
		fmt.Println()
	}
	return nil
}

func Commit(root, name, message string) error {
	if message == "" {
		return fmt.Errorf("commit message is required (-m)")
	}
	m, err := Load(root, name)
	if err != nil {
		return err
	}
	for _, r := range m.Repositories {
		p := filepath.Join(root, r.WorktreePath)
		fmt.Printf("[%s] ", r.Name)
		if !aggit.HasChanges(p) {
			fmt.Println("clean, skipped")
			continue
		}
		if err := aggit.CommitAll(p, message); err != nil {
			fmt.Printf("failed: %v\n", err)
		} else {
			fmt.Println("committed")
		}
	}
	return nil
}

func Push(root, name string) error {
	m, err := Load(root, name)
	if err != nil {
		return err
	}
	for _, r := range m.Repositories {
		p := filepath.Join(root, r.WorktreePath)
		fmt.Printf("[%s] pushing %s\n", r.Name, r.Branch)
		if err := aggit.Push(p, r.Branch); err != nil {
			fmt.Printf("failed: %v\n", err)
		} else {
			fmt.Println("pushed")
		}
	}
	return nil
}
