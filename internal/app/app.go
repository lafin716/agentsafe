package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/agentsafe/agentsafe/internal/agent"
	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/repo"
)

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{Use: "agentsafe", Short: "Multi-repository safe workspace manager for AI coding agents"}
	rootCmd.AddCommand(initCmd(), repoCmd(), cloneCmd(), featureCmd(), statusCmd(), agentCmd(), commitCmd(), pushCmd(), mrCmd())
	return rootCmd
}

func cwdConfig() (string, config.Config, error) {
	wd, _ := os.Getwd()
	return config.LoadFrom(wd)
}

func initCmd() *cobra.Command {
	var name, root string
	c := &cobra.Command{Use: "init", Short: "Initialize an agentsafe workspace", RunE: func(cmd *cobra.Command, args []string) error {
		if root == "" {
			root, _ = os.Getwd()
		}
		cfg, err := config.InitWorkspace(root, name)
		if err != nil {
			return err
		}
		fmt.Printf("Initialized agentsafe workspace: %s\n", cfg.Workspace.Root)
		return nil
	}}
	c.Flags().StringVar(&name, "name", "", "workspace name")
	c.Flags().StringVar(&root, "root", "", "workspace root (default: current directory)")
	return c
}

func repoCmd() *cobra.Command {
	c := &cobra.Command{Use: "repo", Short: "Manage repositories"}
	var typ, defaultBranch string
	add := &cobra.Command{Use: "add NAME URL", Short: "Add a repository", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		_, err = config.AddRepository(root, cfg, config.Repository{Name: args[0], URL: args[1], Type: typ, DefaultBranch: defaultBranch})
		if err != nil {
			return err
		}
		fmt.Printf("Added repository %s\n", args[0])
		return nil
	}}
	add.Flags().StringVar(&typ, "type", "", "repository type")
	add.Flags().StringVar(&defaultBranch, "default-branch", "", "default branch")
	list := &cobra.Command{Use: "list", Short: "List repositories", RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		repo.List(cfg)
		return nil
	}}
	c.AddCommand(add, list)
	return c
}

func cloneCmd() *cobra.Command {
	return &cobra.Command{Use: "clone", Short: "Clone or fetch all configured repositories", RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		return repo.CloneAll(root, cfg)
	}}
}

func featureCmd() *cobra.Command {
	c := &cobra.Command{Use: "feature", Short: "Manage feature worktrees"}
	var base string
	create := &cobra.Command{Use: "create NAME", Short: "Create feature branches and worktrees", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		return feature.Create(root, cfg, args[0], base)
	}}
	create.Flags().StringVar(&base, "base", "", "base branch")
	list := &cobra.Command{Use: "list", Short: "List feature workspaces", RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		return feature.List(root)
	}}
	c.AddCommand(create, list)
	return c
}

func statusCmd() *cobra.Command {
	return &cobra.Command{Use: "status FEATURE", Short: "Show git status for each feature worktree", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		return feature.Status(root, args[0])
	}}
}

func agentCmd() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Manage sanitized agent workspaces"}
	prepare := &cobra.Command{Use: "prepare FEATURE", Short: "Create a sanitized agent workspace", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		return agent.Prepare(root, cfg, args[0])
	}}
	var repoFilter string
	diff := &cobra.Command{Use: "diff FEATURE", Short: "Show differences between agent workspace and worktree", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		byRepo, err := agent.Diff(root, cfg, args[0], repoFilter)
		if err != nil {
			return err
		}
		agent.PrintChanges(args[0], byRepo)
		return nil
	}}
	diff.Flags().StringVar(&repoFilter, "repo", "", "limit to repository")
	var opt agent.Options
	sync := &cobra.Command{Use: "sync FEATURE", Short: "Sync reviewed agent changes back to worktrees", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		return agent.Sync(root, cfg, args[0], opt)
	}}
	sync.Flags().StringVar(&opt.Repo, "repo", "", "limit to repository")
	sync.Flags().BoolVar(&opt.DryRun, "dry-run", false, "show changes without applying")
	sync.Flags().BoolVar(&opt.IncludeRisky, "include-risky", false, "allow risky files to sync")
	sync.Flags().BoolVar(&opt.AllowMaskedSync, "allow-masked-sync", false, "allow masked files to sync")
	sync.Flags().BoolVar(&opt.Yes, "yes", false, "skip confirmation")
	var editor string
	open := &cobra.Command{Use: "open FEATURE", Short: "Print or open the agent workspace path", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		p := filepath.Join(root, "agent", args[0])
		if editor == "" {
			fmt.Println(p)
			return nil
		}
		e := exec.Command(editor, p)
		e.Stdout, e.Stderr, e.Stdin = os.Stdout, os.Stderr, os.Stdin
		return e.Start()
	}}
	open.Flags().StringVar(&editor, "editor", "", "editor command (code/cursor)")
	c.AddCommand(prepare, diff, sync, open)
	return c
}

func commitCmd() *cobra.Command {
	var msg string
	c := &cobra.Command{Use: "commit FEATURE", Short: "Commit changes in all feature worktrees", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		return feature.Commit(root, args[0], msg)
	}}
	c.Flags().StringVarP(&msg, "message", "m", "", "commit message")
	return c
}

func pushCmd() *cobra.Command {
	return &cobra.Command{Use: "push FEATURE", Short: "Push all feature branches", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		return feature.Push(root, args[0])
	}}
}

func mrCmd() *cobra.Command {
	var target, title string
	c := &cobra.Command{Use: "mr", Short: "GitLab merge request helpers"}
	create := &cobra.Command{Use: "create FEATURE", Short: "Print GitLab MR creation guidance", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		if target == "" {
			target = cfg.GitLab.TargetBranch
		}
		if title == "" {
			title = "[" + args[0] + "] merge request"
		}
		fmt.Printf("GitLab MR skeleton: baseUrl=%s tokenEnv=%s source=%s%s target=%s title=%s\n", cfg.GitLab.BaseURL, cfg.GitLab.TokenEnv, cfg.Git.BranchPrefix, args[0], target, title)
		return nil
	}}
	create.Flags().StringVar(&target, "target", "", "target branch")
	create.Flags().StringVar(&title, "title", "", "MR title")
	c.AddCommand(create)
	return c
}
