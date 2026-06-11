package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsafe/agentsafe/internal/agent"
	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/output"
	"github.com/agentsafe/agentsafe/internal/repo"
)

type simpleResult struct {
	Status  string `json:"status"            yaml:"status"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

type repoListResult struct {
	Repositories []repoEntry `json:"repositories" yaml:"repositories"`
}

type repoEntry struct {
	Name string `json:"name" yaml:"name"`
	Type string `json:"type" yaml:"type"`
	URL  string `json:"url"  yaml:"url"`
}

type diffResult struct {
	Feature      string       `json:"feature"      yaml:"feature"`
	Repositories []repoDiff   `json:"repositories" yaml:"repositories"`
}

type repoDiff struct {
	Name    string         `json:"name"    yaml:"name"`
	Changes []agent.Change `json:"changes" yaml:"changes"`
}

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{Use: "agentsafe", Short: "Multi-repository safe workspace manager for AI coding agents"}
	var outputFormat string
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "output format: text, json, yaml")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		f, err := output.Validate(outputFormat)
		if err != nil {
			return err
		}
		output.Set(f)
		return nil
	}
	rootCmd.AddCommand(initCmd(), repoCmd(), pullCmd(), featureCmd(), statusCmd(), agentCmd(), commitCmd(), pushCmd(), mrCmd())
	return rootCmd
}

func cwdConfig() (string, config.Config, error) {
	wd, _ := os.Getwd()
	return config.LoadFrom(wd)
}

func initCmd() *cobra.Command {
	var name, root string
	var templates []string
	c := &cobra.Command{Use: "init", Short: "Initialize an agentsafe workspace", RunE: func(cmd *cobra.Command, args []string) error {
		if root == "" {
			root, _ = os.Getwd()
		}
		cfg, err := config.InitWorkspace(root, name)
		if err != nil {
			return err
		}
		if len(templates) > 0 {
			if _, err := agent.ApplyTemplates(cfg, cfg.Workspace.Root, templates, false); err != nil {
				return err
			}
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: "Initialized agentsafe workspace: " + cfg.Workspace.Root})
		}
		fmt.Printf("Initialized agentsafe workspace: %s\n", cfg.Workspace.Root)
		if len(templates) > 0 {
			fmt.Printf("Applied security templates: %s\n", strings.Join(templates, ", "))
		}
		return nil
	}}
	c.Flags().StringVar(&name, "name", "", "workspace name")
	c.Flags().StringVar(&root, "root", "", "workspace root (default: current directory)")
	c.Flags().StringSliceVar(&templates, "template", nil, "security templates to apply (e.g. spring,react)")
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
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: "Added repository " + args[0]})
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
		if output.IsStructured() {
			result := repoListResult{}
			for _, r := range cfg.Repositories {
				result.Repositories = append(result.Repositories, repoEntry{Name: r.Name, Type: r.Type, URL: r.URL})
			}
			return output.Emit(result)
		}
		repo.List(cfg)
		return nil
	}}
	c.AddCommand(add, list)
	return c
}

func pullCmd() *cobra.Command {
	return &cobra.Command{Use: "pull", Short: "Pull (clone or fetch) all configured repositories", RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		if err := repo.PullAll(root, cfg); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: "pull completed"})
		}
		return nil
	}}
}

func featureCmd() *cobra.Command {
	c := &cobra.Command{Use: "feature", Short: "Manage feature worktrees"}
	var base string
	var force bool
	create := &cobra.Command{
		Use:   "create NAME",
		Short: "Create feature branches and worktrees",
		Long: `Create feature branches and worktrees across all configured repositories.

By default, each repository's current branch is used as the base.
Use --base to specify an explicit base branch for all repositories.

Errors if the feature branch already exists. Use -f to force delete
the local branch and recreate it from the base.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, err := cwdConfig()
			if err != nil {
				return err
			}
			if err := feature.Create(root, cfg, args[0], base, force); err != nil {
				return err
			}
			if output.IsStructured() {
				m, err := feature.Load(root, args[0])
				if err != nil {
					return err
				}
				return output.Emit(m)
			}
			return nil
		}}
	create.Flags().StringVarP(&base, "base", "b", "", "base branch for all repos (default: each repo's current branch)")
	create.Flags().BoolVarP(&force, "force", "f", false, "force recreate if local branch already exists")
	list := &cobra.Command{Use: "list", Short: "List feature workspaces", RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		if output.IsStructured() {
			data, err := feature.ListData(root)
			if err != nil {
				return err
			}
			return output.Emit(data)
		}
		return feature.List(root)
	}}
	var rebaseRepo string
	rebase := &cobra.Command{Use: "rebase NAME", Short: "Rebase feature worktrees onto their base branch", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		res, err := feature.Rebase(root, cfg, args[0], rebaseRepo)
		if err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(res)
		}
		fmt.Printf("Feature: %s\n\n", res.Feature)
		for _, r := range res.Repositories {
			fmt.Printf("[%s] %s", r.Name, r.Status)
			if r.Detail != "" {
				fmt.Printf(" — %s", r.Detail)
			}
			fmt.Println()
		}
		return nil
	}}
	rebase.Flags().StringVar(&rebaseRepo, "repo", "", "limit to repository")
	var deleteBranch, deleteForce bool
	del := &cobra.Command{Use: "delete NAME", Short: "Delete a feature's worktrees and artifacts", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		if err := feature.Delete(root, args[0], feature.DeleteOptions{DeleteBranch: deleteBranch, Force: deleteForce}); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: "deleted feature " + args[0]})
		}
		fmt.Printf("Deleted feature: %s\n", args[0])
		return nil
	}}
	del.Flags().BoolVar(&deleteBranch, "delete-branch", false, "also delete the local feature branch in each repo")
	del.Flags().BoolVar(&deleteForce, "force", false, "remove worktrees even with uncommitted changes")
	c.AddCommand(create, list, rebase, del)
	return c
}

func statusCmd() *cobra.Command {
	return &cobra.Command{Use: "status FEATURE", Short: "Show git status for each feature worktree", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		if output.IsStructured() {
			data, err := feature.StatusData(root, args[0])
			if err != nil {
				return err
			}
			return output.Emit(data)
		}
		return feature.Status(root, args[0])
	}}
}

func agentCmd() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Manage sanitized agent workspaces"}
	var noBackup bool
	agentInit := &cobra.Command{Use: "init FEATURE", Short: "Create a sanitized agent workspace", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		if err := agent.Init(root, cfg, args[0], agent.PrepareOptions{Backup: !noBackup}); err != nil {
			return err
		}
		if output.IsStructured() {
			meta := agent.LoadPrepareMetadata(root, args[0])
			return output.Emit(meta)
		}
		return nil
	}}
	agentInit.Flags().BoolVar(&noBackup, "no-backup", false, "delete the existing agent workspace instead of backing it up")
	del := &cobra.Command{Use: "delete FEATURE", Short: "Delete the agent workspace for a feature", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		if err := agent.Delete(root, args[0]); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: "deleted agent workspace for " + args[0]})
		}
		fmt.Printf("Deleted agent workspace: %s\n", args[0])
		return nil
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
		if output.IsStructured() {
			result := diffResult{Feature: args[0]}
			repos := make([]string, 0, len(byRepo))
			for r := range byRepo {
				repos = append(repos, r)
			}
			sort.Strings(repos)
			for _, r := range repos {
				changes := byRepo[r]
				if changes == nil {
					changes = []agent.Change{}
				}
				result.Repositories = append(result.Repositories, repoDiff{Name: r, Changes: changes})
			}
			return output.Emit(result)
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
	c.AddCommand(agentInit, del, diff, sync, open, templateCmd())
	return c
}

func templateCmd() *cobra.Command {
	c := &cobra.Command{Use: "template", Short: "Manage agent security templates (agentsafe.yaml presets)"}
	list := &cobra.Command{Use: "list", Short: "List available security templates", RunE: func(cmd *cobra.Command, args []string) error {
		templates := agent.TemplateList()
		if output.IsStructured() {
			return output.Emit(struct {
				Templates []agent.TemplateInfo `json:"templates" yaml:"templates"`
			}{Templates: templates})
		}
		for _, t := range templates {
			fmt.Printf("%-8s %s (ignore: %d, mask: %d)\n          %s\n", t.Key, t.Label, t.IgnoreCount, t.MaskCount, t.Description)
		}
		return nil
	}}
	var replace bool
	apply := &cobra.Command{Use: "apply STACK [STACK...]", Short: "Apply security templates to the workspace agentsafe.yaml", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		merged, err := agent.ApplyTemplates(cfg, root, args, replace)
		if err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(merged)
		}
		fmt.Printf("Applied templates %s: %d ignore patterns, %d mask rules\n", strings.Join(args, ", "), len(merged.Ignore), len(merged.Mask))
		return nil
	}}
	apply.Flags().BoolVar(&replace, "replace", false, "replace existing agentsafe.yaml instead of merging")
	c.AddCommand(list, apply)
	return c
}

func commitCmd() *cobra.Command {
	var msg, repoFilter string
	c := &cobra.Command{Use: "commit FEATURE", Short: "Commit changes in feature worktrees", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		if err := feature.Commit(root, args[0], msg, repoFilter); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: msg})
		}
		return nil
	}}
	c.Flags().StringVarP(&msg, "message", "m", "", "commit message")
	c.Flags().StringVar(&repoFilter, "repo", "", "limit to repository")
	return c
}

func pushCmd() *cobra.Command {
	var repoFilter string
	c := &cobra.Command{Use: "push FEATURE", Short: "Push feature branches", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		if err := feature.Push(root, args[0], repoFilter); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok"})
		}
		return nil
	}}
	c.Flags().StringVar(&repoFilter, "repo", "", "limit to repository")
	return c
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
		if output.IsStructured() {
			type mrSkeleton struct {
				BaseURL  string `json:"baseUrl"   yaml:"baseUrl"`
				TokenEnv string `json:"tokenEnv"  yaml:"tokenEnv"`
				Source   string `json:"source"    yaml:"source"`
				Target   string `json:"target"    yaml:"target"`
				Title    string `json:"title"     yaml:"title"`
			}
			return output.Emit(mrSkeleton{
				BaseURL:  cfg.GitLab.BaseURL,
				TokenEnv: cfg.GitLab.TokenEnv,
				Source:   cfg.Git.BranchPrefix + args[0],
				Target:   target,
				Title:    title,
			})
		}
		fmt.Printf("GitLab MR skeleton: baseUrl=%s tokenEnv=%s source=%s%s target=%s title=%s\n", cfg.GitLab.BaseURL, cfg.GitLab.TokenEnv, cfg.Git.BranchPrefix, args[0], target, title)
		return nil
	}}
	create.Flags().StringVar(&target, "target", "", "target branch")
	create.Flags().StringVar(&title, "title", "", "MR title")
	c.AddCommand(create)
	return c
}
