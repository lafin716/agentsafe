package app

import (
	"context"
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
	aggit "github.com/agentsafe/agentsafe/internal/git"
	"github.com/agentsafe/agentsafe/internal/output"
	"github.com/agentsafe/agentsafe/internal/repo"
	"github.com/agentsafe/agentsafe/internal/wttemplate"
	"github.com/agentsafe/agentsafe/packages/core"
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
	URL  string `json:"url"  yaml:"url"`
}

// registeredTemplate reports one path registered as a worktree template. Tracked
// state is informational: registering is aimed at untracked content, but a
// tracked path is still allowed, so callers can see what they registered.
type registeredTemplate struct {
	Path     string              `json:"path"     yaml:"path"`
	Tracked  bool                `json:"tracked"  yaml:"tracked"`
	Template wttemplate.Template `json:"template" yaml:"template"`
}

type diffResult struct {
	Feature      string     `json:"feature"      yaml:"feature"`
	Repositories []repoDiff `json:"repositories" yaml:"repositories"`
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
	rootCmd.AddCommand(initCmd(), repoCmd(), pullCmd(), featureCmd(), worktreeTemplateCmd(), statusCmd(), agentCmd(), commitCmd(), pushCmd(), mrCmd(), coreCmd())
	return rootCmd
}

func coreCmd() *cobra.Command {
	c := &cobra.Command{Use: "core", Short: "Run shared core operations"}
	run := &cobra.Command{
		Use:   "run TEXT",
		Short: "Run the shared Go core service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := core.NewService().Run(context.Background(), core.RunInput{Text: args[0]})
			if err != nil {
				return err
			}
			if output.IsStructured() {
				return output.Emit(result)
			}
			fmt.Println(result.Output)
			return nil
		},
	}
	c.AddCommand(run)
	return c
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
	var defaultBranch string
	add := &cobra.Command{Use: "add NAME URL", Short: "Add a repository", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		_, err = config.AddRepository(root, cfg, config.Repository{Name: args[0], URL: args[1], DefaultBranch: defaultBranch})
		if err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: "Added repository " + args[0]})
		}
		fmt.Printf("Added repository %s\n", args[0])
		return nil
	}}
	add.Flags().StringVar(&defaultBranch, "default-branch", "", "default branch")
	list := &cobra.Command{Use: "list", Short: "List repositories", RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		if output.IsStructured() {
			result := repoListResult{}
			for _, r := range cfg.Repositories {
				result.Repositories = append(result.Repositories, repoEntry{Name: r.Name, URL: r.URL})
			}
			return output.Emit(result)
		}
		repo.List(cfg)
		return nil
	}}
	var deleteFiles bool
	remove := &cobra.Command{Use: "remove NAME", Short: "Remove a repository from the workspace", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		_, res, err := repo.Remove(root, cfg, args[0], deleteFiles)
		if err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(res)
		}
		fmt.Printf("Removed repository %s\n", args[0])
		for _, w := range res.Warnings {
			fmt.Printf("  warning: %s\n", w)
		}
		return nil
	}}
	remove.Flags().BoolVar(&deleteFiles, "delete-files", false, "also delete cloned files (main/<repo> and feature worktrees)")
	c.AddCommand(add, list, remove)
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
	var existingBranch string
	create := &cobra.Command{
		Use:   "create NAME",
		Short: "Create feature branches and worktrees",
		Long: `Create feature branches and worktrees across all configured repositories.

By default, each repository's current branch is used as the base.
Use --base to specify an explicit base branch for all repositories.

When the feature branch already exists, use --existing-branch to choose
whether to error, reuse it, or recreate the local branch from the base.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, err := cwdConfig()
			if err != nil {
				return err
			}
			policy := existingBranch
			if force {
				policy = string(feature.ExistingBranchRecreate)
			}
			parsed, err := feature.ParseExistingBranchPolicy(policy)
			if err != nil {
				return err
			}
			if err := feature.CreateWithOptions(root, cfg, args[0], feature.CreateOptions{
				Base: base, ExistingBranch: parsed,
			}); err != nil {
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
	create.Flags().StringVar(&existingBranch, "existing-branch", "error", "existing branch policy: error, reuse, or recreate")
	create.Flags().BoolVarP(&force, "force", "f", false, "deprecated alias for --existing-branch recreate")
	var checkBase string
	check := &cobra.Command{
		Use:   "check NAME",
		Short: "Check for branch conflicts before creating a feature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, err := cwdConfig()
			if err != nil {
				return err
			}
			result, err := feature.CheckCreate(root, cfg, args[0], checkBase)
			if err != nil {
				return err
			}
			if output.IsStructured() {
				return output.Emit(result)
			}
			fmt.Printf("Feature branch: %s\n", result.Branch)
			for _, repo := range result.Repositories {
				status := "ready"
				if repo.Conflict {
					status = "existing branch"
				}
				if repo.BlockedReason != "" {
					status = "blocked: " + repo.BlockedReason
				}
				fmt.Printf("[%s] %s (base: %s)\n", repo.Name, status, repo.BaseBranch)
			}
			return nil
		},
	}
	check.Flags().StringVarP(&checkBase, "base", "b", "", "base branch for all repos (default: each repo's current branch)")
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

	repoWorktree := &cobra.Command{Use: "repo", Short: "Manage one repository in a feature"}
	var addPolicy string
	var addForce bool
	addRepo := &cobra.Command{
		Use: "add FEATURE REPO", Short: "Add a configured repository to an existing feature", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, err := cwdConfig()
			if err != nil {
				return err
			}
			policy, err := feature.ParseExistingBranchPolicy(addPolicy)
			if err != nil {
				return err
			}
			rm, err := feature.ConfigureRepositoryWorktree(root, cfg, args[0], args[1], feature.RepositoryWorktreeOptions{
				ExistingBranch: policy, Force: addForce,
			})
			if err != nil {
				return err
			}
			if output.IsStructured() {
				return output.Emit(rm)
			}
			fmt.Printf("Added repository %s to feature %s\n", args[1], args[0])
			return nil
		},
	}
	addRepo.Flags().StringVar(&addPolicy, "existing-branch", "reuse", "existing branch policy: error, reuse, or recreate")
	addRepo.Flags().BoolVar(&addForce, "force", false, "delete an existing worktree directory and recreate")

	var recreatePolicy string
	var recreateForce bool
	recreateRepo := &cobra.Command{
		Use: "recreate FEATURE REPO", Short: "Recreate one repository worktree", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, err := cwdConfig()
			if err != nil {
				return err
			}
			policy, err := feature.ParseExistingBranchPolicy(recreatePolicy)
			if err != nil {
				return err
			}
			rm, err := feature.ConfigureRepositoryWorktree(root, cfg, args[0], args[1], feature.RepositoryWorktreeOptions{
				ExistingBranch: policy, Recreate: true, Force: recreateForce,
			})
			if err != nil {
				return err
			}
			if output.IsStructured() {
				return output.Emit(rm)
			}
			fmt.Printf("Recreated repository %s in feature %s\n", args[1], args[0])
			return nil
		},
	}
	recreateRepo.Flags().StringVar(&recreatePolicy, "existing-branch", "reuse", "existing branch policy: error, reuse, or recreate")
	recreateRepo.Flags().BoolVar(&recreateForce, "force", false, "discard uncommitted worktree changes")
	repoWorktree.AddCommand(addRepo, recreateRepo)

	c.AddCommand(create, check, list, rebase, del, repoWorktree)
	return c
}

func worktreeTemplateCmd() *cobra.Command {
	c := &cobra.Command{Use: "worktree-template", Short: "Manage files copied into new worktrees"}
	list := &cobra.Command{Use: "list", Short: "List worktree templates", RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		items, err := wttemplate.List(root)
		if err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(struct {
				Templates []wttemplate.Template `json:"templates" yaml:"templates"`
			}{Templates: items})
		}
		for _, t := range items {
			state := "enabled"
			if !t.Enabled {
				state = "disabled"
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", t.ID, t.Name, t.TargetMode, state)
		}
		return nil
	}}

	var target string
	var repos []string
	var overwrite bool
	var disabled bool
	applyFlags := func(cmd *cobra.Command) {
		cmd.Flags().StringVar(&target, "target", wttemplate.TargetAllRepos, "target: workspaceRoot, featureRoot, allRepos, selectedRepos, agentRoot, agentAllRepos, or agentSelectedRepos")
		cmd.Flags().StringSliceVar(&repos, "repo", nil, "repository name for selectedRepos/agentSelectedRepos (repeat or comma-separate)")
		cmd.Flags().BoolVar(&overwrite, "overwrite", false, "overwrite existing files")
		cmd.Flags().BoolVar(&disabled, "disabled", false, "import as disabled")
	}
	normalizeAdded := func(root string, items []wttemplate.Template) error {
		for i := range items {
			items[i].TargetMode = target
			items[i].RepoNames = repos
			items[i].Overwrite = overwrite
			items[i].Enabled = !disabled
			if err := wttemplate.Update(root, items[i]); err != nil {
				return err
			}
		}
		return nil
	}
	addFile := &cobra.Command{Use: "add-file PATH [PATH...]", Short: "Import template file(s)", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		items, err := wttemplate.ImportFiles(root, args)
		if err != nil {
			return err
		}
		if err := normalizeAdded(root, items); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(struct {
				Templates []wttemplate.Template `json:"templates" yaml:"templates"`
			}{Templates: items})
		}
		fmt.Printf("Imported %d template item(s)\n", len(items))
		return nil
	}}
	applyFlags(addFile)

	addFolder := &cobra.Command{Use: "add-folder PATH", Short: "Import a template folder", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		item, err := wttemplate.ImportFolder(root, args[0])
		if err != nil {
			return err
		}
		if err := normalizeAdded(root, []wttemplate.Template{item}); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(item)
		}
		fmt.Printf("Imported template %s\n", item.Name)
		return nil
	}}
	applyFlags(addFolder)

	register := &cobra.Command{Use: "register PATH [PATH...]", Short: "Register existing workspace path(s) as worktree templates", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		opts := wttemplate.RegisterOptions{
			RepoNames:      repos,
			Overwrite:      overwrite,
			Enabled:        !disabled,
			WorkspaceRepos: wttemplate.WorkspaceRepoNames(root),
		}
		// Without an explicit --target the destination is inferred from where the
		// source lives, which is what registering an existing path usually wants.
		if cmd.Flags().Changed("target") {
			opts.TargetMode = target
		}
		items := make([]registeredTemplate, 0, len(args))
		for _, path := range args {
			tracked, err := aggit.IsTracked(path)
			if err != nil {
				return err
			}
			t, err := wttemplate.RegisterPath(root, path, opts)
			if err != nil {
				return err
			}
			items = append(items, registeredTemplate{Path: path, Tracked: tracked, Template: t})
		}
		if output.IsStructured() {
			return output.Emit(struct {
				Registered []registeredTemplate `json:"registered" yaml:"registered"`
			}{Registered: items})
		}
		for _, item := range items {
			state := "untracked"
			if item.Tracked {
				state = "tracked"
			}
			fmt.Printf("Registered %s (%s) as template %s -> %s\n", item.Path, state, item.Template.Name, item.Template.TargetMode)
		}
		return nil
	}}
	applyFlags(register)

	del := &cobra.Command{Use: "delete ID", Short: "Delete a worktree template", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		if err := wttemplate.Delete(root, args[0]); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: "deleted worktree template " + args[0]})
		}
		fmt.Printf("Deleted worktree template %s\n", args[0])
		return nil
	}}
	clear := &cobra.Command{Use: "clear", Short: "Delete all worktree templates", RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		if err := wttemplate.Clear(root); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: "cleared worktree templates"})
		}
		fmt.Println("Cleared worktree templates")
		return nil
	}}
	var applyTarget string
	apply := &cobra.Command{Use: "apply FEATURE", Short: "Apply templates to an existing worktree/agent workspace", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, _, err := cwdConfig()
		if err != nil {
			return err
		}
		fm, err := feature.Load(root, args[0])
		if err != nil {
			return err
		}
		switch applyTarget {
		case "worktree":
			err = wttemplate.Apply(root, fm.FolderKey(), cliWorktreeTemplateRepos(root, fm.Repositories))
		case "agent":
			err = wttemplate.ApplyAgent(root, fm.FolderKey(), cliAgentTemplateRepos(root, fm.FolderKey(), fm.Repositories))
		case "all":
			if err = wttemplate.Apply(root, fm.FolderKey(), cliWorktreeTemplateRepos(root, fm.Repositories)); err == nil {
				err = wttemplate.ApplyAgent(root, fm.FolderKey(), cliAgentTemplateRepos(root, fm.FolderKey(), fm.Repositories))
			}
		default:
			return fmt.Errorf("invalid apply target %q (expected worktree, agent, or all)", applyTarget)
		}
		if err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok", Message: "templates applied"})
		}
		fmt.Printf("Applied templates to %s\n", args[0])
		return nil
	}}
	apply.Flags().StringVar(&applyTarget, "target", "all", "apply target: worktree, agent, or all")
	c.AddCommand(list, addFile, addFolder, register, del, clear, apply)
	return c
}

func cliWorktreeTemplateRepos(root string, repos []feature.RepoMeta) []wttemplate.Repo {
	out := make([]wttemplate.Repo, 0, len(repos))
	for _, r := range repos {
		out = append(out, wttemplate.Repo{
			Name:         r.Name,
			WorktreePath: filepath.Join(root, filepath.FromSlash(r.WorktreePath)),
		})
	}
	return out
}

func cliAgentTemplateRepos(root, featureKey string, repos []feature.RepoMeta) []wttemplate.Repo {
	out := make([]wttemplate.Repo, 0, len(repos))
	for _, r := range repos {
		out = append(out, wttemplate.Repo{
			Name:         r.Name,
			WorktreePath: config.AgentPath(root, featureKey, r.Name),
		})
	}
	return out
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
	var prepareRepo string
	agentInit := &cobra.Command{Use: "init FEATURE", Short: "Create a sanitized agent workspace", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		if prepareRepo != "" {
			if _, err := agent.PrepareRepository(root, cfg, args[0], prepareRepo, agent.PrepareOptions{Backup: !noBackup}); err != nil {
				return err
			}
		} else {
			if err := agent.Init(root, cfg, args[0], agent.PrepareOptions{Backup: !noBackup}); err != nil {
				return err
			}
		}
		if output.IsStructured() {
			meta := agent.LoadPrepareMetadata(root, args[0])
			return output.Emit(meta)
		}
		return nil
	}}
	agentInit.Flags().BoolVar(&noBackup, "no-backup", false, "delete the existing agent workspace instead of backing it up")
	agentInit.Flags().StringVar(&prepareRepo, "repo", "", "prepare only one repository")
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
	var syncMessage string
	sync := &cobra.Command{Use: "sync FEATURE", Short: "Sync reviewed agent changes back to worktrees", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		if strings.TrimSpace(syncMessage) != "" {
			return agent.SyncAndCommit(root, cfg, args[0], syncMessage, opt)
		}
		return agent.Sync(root, cfg, args[0], opt)
	}}
	sync.Flags().StringVar(&opt.Repo, "repo", "", "limit to repository")
	sync.Flags().BoolVar(&opt.DryRun, "dry-run", false, "show changes without applying")
	sync.Flags().BoolVar(&opt.IncludeRisky, "include-risky", false, "allow risky files to sync")
	sync.Flags().BoolVar(&opt.AllowMaskedSync, "allow-masked-sync", false, "allow masked files to sync")
	sync.Flags().BoolVar(&opt.Yes, "yes", false, "skip confirmation")
	sync.Flags().StringVarP(&syncMessage, "message", "m", "", "commit the synced changes with this message")
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
	var shipOpt agent.Options
	var shipMessage string
	var shipNoPush bool
	ship := &cobra.Command{Use: "ship FEATURE", Short: "Sync reviewed agent changes back to worktrees, commit, then push — in one step", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		shipOpt.Yes = true // non-interactive: intended for one-shot automation / Stop hooks
		if shipNoPush {
			msg := shipMessage
			if strings.TrimSpace(msg) == "" {
				msg = agent.DefaultCommitMessage(args[0])
			}
			if err := agent.SyncAndCommit(root, cfg, args[0], msg, shipOpt); err != nil {
				return err
			}
		} else if err := agent.SyncCommitPush(root, cfg, args[0], shipMessage, shipOpt); err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(simpleResult{Status: "ok"})
		}
		return nil
	}}
	ship.Flags().StringVar(&shipOpt.Repo, "repo", "", "limit to repository")
	ship.Flags().BoolVar(&shipOpt.DryRun, "dry-run", false, "show changes without applying (no commit/push)")
	ship.Flags().BoolVar(&shipOpt.IncludeRisky, "include-risky", false, "allow risky files to sync")
	ship.Flags().BoolVar(&shipOpt.AllowMaskedSync, "allow-masked-sync", false, "allow masked files to sync")
	ship.Flags().StringVarP(&shipMessage, "message", "m", "", "commit message (default: templated auto-sync message)")
	ship.Flags().BoolVar(&shipNoPush, "no-push", false, "sync and commit only, skip push")
	preview := &cobra.Command{Use: "preview REPO", Short: "Preview how the saved ignore/mask policy would treat a repository's files", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, err := cwdConfig()
		if err != nil {
			return err
		}
		res, err := agent.ScanPreview(root, cfg, args[0])
		if err != nil {
			return err
		}
		if output.IsStructured() {
			return output.Emit(res)
		}
		fmt.Printf("Repository: %s (%s)\n", res.Repo, res.Source)
		fmt.Printf("ignored: %d  masked: %d  copied: %d  total: %d\n\n", res.Ignored, res.Masked, res.Copied, res.Total)
		for _, e := range res.Entries {
			detail := ""
			switch e.Status {
			case agent.PreviewIgnored:
				if e.IgnorePattern != "" {
					detail = "[" + e.IgnorePattern + "]"
				}
			case agent.PreviewMasked:
				parts := make([]string, 0, len(e.MaskMatches))
				for _, m := range e.MaskMatches {
					parts = append(parts, fmt.Sprintf("%s x%d", m.Name, m.Count))
				}
				detail = "(" + strings.Join(parts, ", ") + ")"
			default:
				if e.Binary {
					detail = "(binary)"
				}
			}
			fmt.Printf("%-8s %s %s\n", strings.ToUpper(e.Status), e.Path, detail)
		}
		return nil
	}}
	c.AddCommand(agentInit, del, diff, sync, open, ship, preview, templateCmd())
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
