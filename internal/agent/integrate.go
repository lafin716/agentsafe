package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	aggit "github.com/agentsafe/agentsafe/internal/git"
)

// Rewriting a Repo Worktree under a prepared agent workspace is how reviewed
// work gets silently thrown away.
//
// Diff compares the agent copy against the Repo Worktree file by file. After a
// rebase or merge the worktree's files have moved on, so every file the
// integration touched now reads as an Agent Change — and Sync, doing what it is
// told, copies the pre-integration agent copy back over them. The integration
// disappears with no error anywhere.
//
// So an integration refuses any repository holding unreviewed Agent Changes, and
// is allowed where there are none: the agent copy is then merely stale, which
// re-preparing fixes. This is the same shape as feature's existing rule that a
// worktree with uncommitted changes is skipped.

// GuardIntegrationInProgress refuses an agent workspace operation on a
// repository holding an Interrupted Integration. repoFilter limits the check to
// one repository; empty checks them all.
//
// The block runs the other way round from CheckIntegration, and for the mirror
// reason (docs/adr/0002): mid-rebase the working tree is a partial replay that
// corresponds to no commit, so preparing from it would copy half-applied files
// into the agent workspace, and syncing into it would mix reviewed changes with
// a conflict the user has not finished resolving.
func GuardIntegrationInProgress(root, featureName, repoFilter string) error {
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return err
	}
	for _, r := range fm.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		p := filepath.Join(root, r.WorktreePath)
		state, stateErr := aggit.IntegrationStateOf(p)
		if stateErr != nil || !state.InProgress() {
			continue
		}
		return fmt.Errorf(
			"repository %s has a %s in progress; resolve or abort it before working on the agent workspace",
			r.Name, state.Kind)
	}
	return nil
}

// RepoIntegrationReadiness is whether one repository can have its Repo Worktree
// rewritten, and why not when it cannot.
type RepoIntegrationReadiness struct {
	Repo string `json:"repo" yaml:"repo"`
	// AgentPrepared is whether an agent workspace exists for this repository.
	AgentPrepared bool `json:"agentPrepared" yaml:"agentPrepared"`
	// AgentChanges is the number of unreviewed Agent Changes found. Zero when
	// nothing is prepared.
	AgentChanges int `json:"agentChanges" yaml:"agentChanges"`
	// StaleAfter is true when integrating will leave the agent workspace needing
	// to be prepared again.
	StaleAfter bool   `json:"staleAfter"      yaml:"staleAfter"`
	Blocked    bool   `json:"blocked"         yaml:"blocked"`
	Reason     string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// IntegrationReadiness is the per-repository verdict for a whole Feature, which
// the confirm dialog shows before anything runs.
type IntegrationReadiness struct {
	Feature      string                     `json:"feature"      yaml:"feature"`
	Repositories []RepoIntegrationReadiness `json:"repositories" yaml:"repositories"`
}

// Blockers reduces the readiness to the repo→reason map feature.IntegrateOptions
// takes.
func (r IntegrationReadiness) Blockers() map[string]string {
	blocked := map[string]string{}
	for _, repo := range r.Repositories {
		if repo.Blocked {
			blocked[repo.Repo] = repo.Reason
		}
	}
	return blocked
}

// CheckIntegration reports which repositories of a Feature may be rebased or
// merged. repoFilter limits it to one repository; empty checks them all.
func CheckIntegration(root string, cfg config.Config, featureName, repoFilter string) (IntegrationReadiness, error) {
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return IntegrationReadiness{}, err
	}
	readiness := IntegrationReadiness{Feature: fm.Name}
	folderKey := fm.FolderKey()
	for _, r := range fm.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		repo := RepoIntegrationReadiness{Repo: r.Name}
		if st, statErr := os.Stat(config.AgentPath(root, folderKey, r.Name)); statErr == nil && st.IsDir() {
			repo.AgentPrepared = true
		}
		if !repo.AgentPrepared {
			// Nothing to strand.
			readiness.Repositories = append(readiness.Repositories, repo)
			continue
		}
		repo.StaleAfter = true
		byRepo, diffErr := Diff(root, cfg, featureName, r.Name)
		if diffErr != nil {
			// An agent workspace we cannot read is not one we should quietly
			// overwrite, so treat it as blocking and say why.
			repo.Blocked = true
			repo.Reason = fmt.Sprintf("agent workspace could not be compared: %v", diffErr)
			readiness.Repositories = append(readiness.Repositories, repo)
			continue
		}
		repo.AgentChanges = len(byRepo[r.Name])
		if repo.AgentChanges > 0 {
			repo.Blocked = true
			repo.Reason = fmt.Sprintf(
				"%d unreviewed agent change(s); review or discard them first, "+
					"otherwise a later sync would overwrite the integration",
				repo.AgentChanges)
		}
		readiness.Repositories = append(readiness.Repositories, repo)
	}
	return readiness, nil
}

// RepoRebasePreflight is what one repository would do if a rebase ran now, and
// why it would not. Read-only: producing it changes nothing.
type RepoRebasePreflight struct {
	Repo       string `json:"repo"       yaml:"repo"`
	Branch     string `json:"branch"     yaml:"branch"`
	BaseBranch string `json:"baseBranch" yaml:"baseBranch"`
	// Upstream is the ref the rebase would replay onto, resolved exactly as the
	// rebase itself resolves it.
	Upstream string `json:"upstream,omitempty" yaml:"upstream,omitempty"`
	// Behind is how many commits the upstream has that this branch does not —
	// the work a rebase would replay over. Zero means already up to date.
	Behind int `json:"behind" yaml:"behind"`
	// Unpushed is how many commits would need force-pushing afterwards.
	Unpushed int `json:"unpushed" yaml:"unpushed"`
	// Dirty repo worktrees are skipped rather than stashed.
	Dirty bool `json:"dirty" yaml:"dirty"`
	// Integration is any Interrupted Integration already open here.
	Integration aggit.IntegrationState `json:"integration" yaml:"integration"`
	// AgentChanges is the number of unreviewed Agent Changes; any is blocking.
	AgentChanges int    `json:"agentChanges"     yaml:"agentChanges"`
	Blocked      bool   `json:"blocked"          yaml:"blocked"`
	Reason       string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// RebasePreflightResult is the per-repository verdict for a whole Feature.
type RebasePreflightResult struct {
	Feature      string                `json:"feature"      yaml:"feature"`
	Repositories []RepoRebasePreflight `json:"repositories" yaml:"repositories"`
}

// RebasePreflight inspects every selected repository without changing any of
// them, so a dialog can say up front what a rebase would do and which
// repositories it would refuse.
//
// The refusals it reports are the ones the rebase itself applies — it asks
// feature for them rather than restating them, because a preflight that
// disagrees with the operation is worse than no preflight.
func RebasePreflight(root string, cfg config.Config, featureName, repoFilter string) (RebasePreflightResult, error) {
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return RebasePreflightResult{}, err
	}
	readiness, err := CheckIntegration(root, cfg, featureName, repoFilter)
	if err != nil {
		return RebasePreflightResult{}, err
	}
	agentByRepo := map[string]RepoIntegrationReadiness{}
	for _, r := range readiness.Repositories {
		agentByRepo[r.Repo] = r
	}

	out := RebasePreflightResult{Feature: fm.Name, Repositories: []RepoRebasePreflight{}}
	for _, r := range fm.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		out.Repositories = append(out.Repositories,
			rebasePreflightFor(root, cfg, r, agentByRepo[r.Name]))
	}
	return out, nil
}

func rebasePreflightFor(root string, cfg config.Config, r feature.RepoMeta,
	readiness RepoIntegrationReadiness,
) RepoRebasePreflight {
	p := filepath.Join(root, r.WorktreePath)
	check := feature.CheckIntegrateRepo(p, r, cfg, feature.IntegrateOptions{
		Blocked: map[string]string{r.Name: readiness.Reason},
	})
	return RepoRebasePreflight{
		Repo:         r.Name,
		Branch:       r.Branch,
		BaseBranch:   check.BaseBranch,
		Upstream:     check.Upstream,
		Behind:       check.Behind,
		Unpushed:     check.Unpushed,
		Dirty:        check.Dirty,
		Integration:  check.Integration,
		AgentChanges: readiness.AgentChanges,
		Blocked:      check.Blocked,
		Reason:       check.Reason,
	}
}

// RebaseFeature replays a Feature's branches onto their Base Branch, refusing
// repositories that hold unreviewed Agent Changes. Both frontends go through
// here so the CLI cannot bypass the check.
func RebaseFeature(root string, cfg config.Config, featureName, repoFilter string, opts feature.IntegrateOptions) (feature.IntegrationResult, error) {
	opts, err := withAgentBlockers(root, cfg, featureName, repoFilter, opts)
	if err != nil {
		return feature.IntegrationResult{}, err
	}
	return feature.Rebase(root, cfg, featureName, repoFilter, opts)
}

// MergeFeature takes a Feature's Base Branch into its branches, under the same
// refusal as RebaseFeature.
func MergeFeature(root string, cfg config.Config, featureName, repoFilter string, opts feature.IntegrateOptions) (feature.IntegrationResult, error) {
	opts, err := withAgentBlockers(root, cfg, featureName, repoFilter, opts)
	if err != nil {
		return feature.IntegrationResult{}, err
	}
	return feature.Merge(root, cfg, featureName, repoFilter, opts)
}

// withAgentBlockers merges the agent-workspace refusals into whatever the caller
// already wanted blocked, without dropping the caller's own reasons.
func withAgentBlockers(root string, cfg config.Config, featureName, repoFilter string, opts feature.IntegrateOptions) (feature.IntegrateOptions, error) {
	readiness, err := CheckIntegration(root, cfg, featureName, repoFilter)
	if err != nil {
		return opts, err
	}
	blocked := map[string]string{}
	for repo, reason := range opts.Blocked {
		blocked[repo] = reason
	}
	for repo, reason := range readiness.Blockers() {
		if _, exists := blocked[repo]; !exists {
			blocked[repo] = reason
		}
	}
	opts.Blocked = blocked
	return opts, nil
}
