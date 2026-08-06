package feature

import (
	"fmt"
	"path/filepath"

	"github.com/agentsafe/agentsafe/internal/config"
	aggit "github.com/agentsafe/agentsafe/internal/git"
)

// Integration operations on Repo Worktrees: replaying a Feature's branch onto
// its Base Branch (rebase), taking the Base Branch into it (merge), and
// resolving an Interrupted Integration left by either.
//
// Only this direction exists. Merging a Feature into its Base Branch locally
// would leave the Main Clone diverged from origin, which permanently breaks the
// --ff-only pull it is maintained with; that integration is a PR or MR instead.
// See docs/adr/0001-main-clone-is-not-a-merge-target.md.

// Integration outcome for one repository.
//
// Status is one of:
//
//	rebased     the branch was replayed onto the upstream
//	merged      the upstream was merged into the branch
//	up-to-date  the branch already contained the upstream
//	skipped     preconditions were not met; the worktree was not touched
//	conflicted  an Interrupted Integration is open and needs resolving
//	failed      the operation could not run
type IntegrationRepoResult struct {
	Name       string `json:"name"       yaml:"name"`
	Branch     string `json:"branch"     yaml:"branch"`
	BaseBranch string `json:"baseBranch" yaml:"baseBranch"`
	// Upstream is the ref actually used, which is origin/<base> when it resolves
	// and the local base otherwise.
	Upstream string `json:"upstream,omitempty" yaml:"upstream,omitempty"`
	Status   string `json:"status"             yaml:"status"`
	Detail   string `json:"detail"             yaml:"detail"`
	// Conflicts lists the unmerged paths when Status is conflicted.
	Conflicts []string `json:"conflicts,omitempty" yaml:"conflicts,omitempty"`
	// GitOutput is the command that failed with its stdout and stderr, verbatim.
	// Kept apart from Detail so a screen can show the one-line summary and put
	// the raw output behind a "details" toggle: git's own message is often the
	// only place the real reason appears, and Detail used to overwrite it with a
	// fixed sentence.
	GitOutput string `json:"gitOutput,omitempty" yaml:"gitOutput,omitempty"`
}

// IntegrationResult is the per-repository outcome for a whole Feature.
type IntegrationResult struct {
	Feature string `json:"feature" yaml:"feature"`
	// Operation is rebase, merge, continue or abort.
	Operation    string                  `json:"operation"    yaml:"operation"`
	Repositories []IntegrationRepoResult `json:"repositories" yaml:"repositories"`
}

// Kept so existing callers and the desktop bindings compile unchanged; the JSON
// shape is unchanged apart from added fields.
type (
	RebaseResult     = IntegrationResult
	RebaseRepoResult = IntegrationRepoResult
)

// IntegrateOptions tunes how an integration behaves.
type IntegrateOptions struct {
	// AbortOnConflict restores the worktree instead of leaving an Interrupted
	// Integration behind. Opt-in: leaving the conflict is the default so a user
	// can resolve it rather than redo it. See docs/adr/0002.
	AbortOnConflict bool
	// Blocked maps a repository name to why it must not be touched. Those
	// repositories are reported as skipped with that reason. Callers that know
	// about agent workspaces use this to refuse repositories holding unreviewed
	// Agent Changes, which a rebase would strand.
	Blocked map[string]string
	// Upstream overrides the ref to integrate from. Empty resolves the Base
	// Branch, preferring origin/<base>.
	Upstream string
}

// Rebase replays each selected Repo Worktree's branch onto its Base Branch.
// repoFilter limits it to one repository; empty means every repository in the
// Feature.
func Rebase(root string, cfg config.Config, name, repoFilter string, opts IntegrateOptions) (IntegrationResult, error) {
	return integrate(root, cfg, name, repoFilter, "rebase", opts)
}

// Merge takes each selected Repo Worktree's Base Branch into its branch. This is
// the alternative to Rebase for a branch that has already been pushed, where
// rewriting history would force everyone else to recover.
func Merge(root string, cfg config.Config, name, repoFilter string, opts IntegrateOptions) (IntegrationResult, error) {
	return integrate(root, cfg, name, repoFilter, "merge", opts)
}

func integrate(root string, cfg config.Config, name, repoFilter, operation string, opts IntegrateOptions) (IntegrationResult, error) {
	m, err := Load(root, name)
	if err != nil {
		return IntegrationResult{}, err
	}
	result := IntegrationResult{Feature: m.Name, Operation: operation}
	for _, r := range selectRepos(m, repoFilter) {
		result.Repositories = append(result.Repositories,
			integrateRepo(root, cfg, m, r, operation, opts))
	}
	return result, nil
}

// RepoIntegrateCheck is what an integration would do to one Repo Worktree, and
// why it would refuse. It is produced by the same function the integration
// itself consults, so a preflight dialog cannot disagree with the operation it
// is previewing.
type RepoIntegrateCheck struct {
	BaseBranch string
	// Upstream is the ref that would be integrated from. Empty when none
	// resolved, in which case Blocked says so.
	Upstream string
	// Behind is how many commits the upstream has that this branch does not.
	// Only filled when withCounts is asked for, since it costs a rev-list.
	Behind int
	// Unpushed is how many commits would need force-pushing after a rebase.
	Unpushed    int
	Dirty       bool
	Integration aggit.IntegrationState
	Blocked     bool
	Reason      string
}

// CheckIntegrateRepo decides whether one Repo Worktree may be integrated, and
// resolves what it would integrate from. opts.Blocked supplies refusals the
// caller already knows about (unreviewed Agent Changes, most often).
//
// It also fills in the counts a dialog needs. That costs two rev-lists, which
// is why the integration path calls the cheaper checkIntegrateRepo instead.
func CheckIntegrateRepo(worktreePath string, r RepoMeta, cfg config.Config, opts IntegrateOptions) RepoIntegrateCheck {
	check := checkIntegrateRepo(worktreePath, r, cfg, opts)
	if check.Upstream == "" {
		return check
	}
	if n, err := aggit.RevListCount(worktreePath, "HEAD.."+check.Upstream); err == nil {
		check.Behind = n
	}
	check.Unpushed = unpushedCount(worktreePath, r.Branch, check.BaseBranch)
	return check
}

// checkIntegrateRepo is the refusal policy itself: one place deciding what
// stops an integration, so the operation and any preview of it stay in step.
// Unlike CheckIntegrateRepo it runs no counting, and does not fetch.
func checkIntegrateRepo(worktreePath string, r RepoMeta, cfg config.Config, opts IntegrateOptions) RepoIntegrateCheck {
	base := r.BaseBranch
	if base == "" {
		base = cfg.Git.DefaultBaseBranch
	}
	check := RepoIntegrateCheck{BaseBranch: base}

	if state, err := aggit.IntegrationStateOf(worktreePath); err == nil {
		check.Integration = state
	}
	check.Dirty = aggit.HasChanges(worktreePath)

	// The order matters: it is the order the user has to deal with them in.
	switch {
	case check.Integration.InProgress():
		// git would refuse anyway, with a less useful message.
		check.Blocked = true
		check.Reason = fmt.Sprintf("a %s is already in progress; resolve or abort it first",
			check.Integration.Kind)
	case check.Dirty:
		check.Blocked = true
		check.Reason = "uncommitted changes; commit or stash first"
	case opts.Blocked[r.Name] != "":
		check.Blocked = true
		check.Reason = opts.Blocked[r.Name]
	}

	// Resolved even for a blocked repository, so a dialog can still say what the
	// integration would have targeted.
	upstream, err := resolveUpstream(worktreePath, base, opts.Upstream)
	if err != nil {
		if !check.Blocked {
			check.Blocked = true
			check.Reason = err.Error()
		}
		return check
	}
	check.Upstream = upstream
	return check
}

func integrateRepo(root string, cfg config.Config, m Metadata, r RepoMeta, operation string, opts IntegrateOptions) IntegrationRepoResult {
	p := filepath.Join(root, r.WorktreePath)

	_ = aggit.Fetch(p) // Best effort: integration falls back to local refs.

	check := checkIntegrateRepo(p, r, cfg, opts)
	rr := IntegrationRepoResult{
		Name: r.Name, Branch: r.Branch, BaseBranch: check.BaseBranch, Upstream: check.Upstream,
	}
	if check.Blocked {
		rr.Status = "skipped"
		rr.Detail = check.Reason
		return rr
	}
	upstream := check.Upstream

	before, _ := aggit.HeadSHA(p)
	runErr := aggit.RebaseOnto(p, upstream)
	if operation == "merge" {
		runErr = aggit.MergeOnto(p, upstream)
	}
	if runErr != nil {
		return integrationFailure(p, rr, operation, upstream, runErr, opts)
	}
	after, _ := aggit.HeadSHA(p)
	if before == after {
		rr.Status = "up-to-date"
		rr.Detail = fmt.Sprintf("already based on %s", upstream)
		return rr
	}
	if operation == "merge" {
		rr.Status = "merged"
		rr.Detail = fmt.Sprintf("merged %s", upstream)
		return rr
	}
	rr.Status = "rebased"
	rr.Detail = fmt.Sprintf("rebased onto %s", upstream)
	return rr
}

// integrationFailure distinguishes a conflict, which leaves a resolvable
// Interrupted Integration, from an operation that could not run at all. In
// either case git's own output is kept: it is routinely the only place the real
// reason is written down.
func integrationFailure(p string, rr IntegrationRepoResult, operation, upstream string, runErr error, opts IntegrateOptions) IntegrationRepoResult {
	rr.GitOutput = gitOutputOf(runErr)
	state, stateErr := aggit.IntegrationStateOf(p)
	if stateErr != nil || !state.InProgress() {
		rr.Status = "failed"
		rr.Detail = fmt.Sprintf("%s onto %s could not run", operation, upstream)
		return rr
	}
	if opts.AbortOnConflict {
		if operation == "merge" {
			_ = aggit.MergeAbort(p)
		} else {
			_ = aggit.RebaseAbort(p)
		}
		rr.Status = "failed"
		rr.Detail = fmt.Sprintf("%s onto %s conflicted; aborted as requested", operation, upstream)
		return rr
	}
	rr.Status = "conflicted"
	if paths, err := aggit.IntegrationConflicts(p); err == nil {
		rr.Conflicts = paths
	}
	rr.Detail = fmt.Sprintf("%s onto %s conflicted in %d file(s); resolve them, then continue",
		operation, upstream, len(rr.Conflicts))
	return rr
}

// IntegrationPushResult pairs an integration with the pushes that followed it,
// so a caller gets one value describing the whole "rebase then publish" action.
type IntegrationPushResult struct {
	Integration IntegrationResult `json:"integration"      yaml:"integration"`
	Pushes      []PushResult      `json:"pushes,omitempty" yaml:"pushes,omitempty"`
}

// PushFailed reports whether any follow-up push failed.
func (r IntegrationPushResult) PushFailed() bool {
	for _, p := range r.Pushes {
		if p.Failed() {
			return true
		}
	}
	return false
}

// PushIntegrated force-pushes exactly those repositories whose history the
// integration rewrote. This rule lives here, once, because both frontends need
// it and getting it wrong is how a half-finished rebase reaches origin:
//
//	rebased     rewritten, so a plain push cannot work — force with a lease
//	up-to-date  nothing new to send
//	skipped     never touched
//	conflicted  an Interrupted Integration is open; publishing it is the bug
//	failed      did not run
//
// See docs/adr/0003.
func PushIntegrated(root, name string, res IntegrationResult) ([]PushResult, error) {
	var out []PushResult
	for _, r := range res.Repositories {
		if r.Status != "rebased" {
			continue
		}
		pushed, err := Push(root, name, r.Name, PushOptions{Force: true})
		if err != nil {
			return out, err
		}
		out = append(out, pushed)
	}
	return out, nil
}

// ContinueIntegration resumes an Interrupted Integration once its conflicts have
// been resolved and staged. It reads which kind is open rather than being told,
// so it also finishes an integration a user started in a terminal.
func ContinueIntegration(root string, name, repoFilter string) (IntegrationResult, error) {
	return resolveIntegration(root, name, repoFilter, "continue")
}

// AbortIntegration discards an Interrupted Integration, restoring the Repo
// Worktree to where it was before the rebase or merge started.
func AbortIntegration(root string, name, repoFilter string) (IntegrationResult, error) {
	return resolveIntegration(root, name, repoFilter, "abort")
}

func resolveIntegration(root, name, repoFilter, operation string) (IntegrationResult, error) {
	m, err := Load(root, name)
	if err != nil {
		return IntegrationResult{}, err
	}
	result := IntegrationResult{Feature: m.Name, Operation: operation}
	for _, r := range selectRepos(m, repoFilter) {
		rr := IntegrationRepoResult{Name: r.Name, Branch: r.Branch, BaseBranch: r.BaseBranch}
		p := filepath.Join(root, r.WorktreePath)
		state, stateErr := aggit.IntegrationStateOf(p)
		if stateErr != nil || !state.InProgress() {
			rr.Status = "skipped"
			rr.Detail = "nothing in progress"
			result.Repositories = append(result.Repositories, rr)
			continue
		}
		var runErr error
		switch {
		case operation == "abort" && state.Kind == aggit.IntegrationMerge:
			runErr = aggit.MergeAbort(p)
		case operation == "abort":
			runErr = aggit.RebaseAbort(p)
		case state.Kind == aggit.IntegrationMerge:
			// Committing the resolved merge is what "continue" means for a merge;
			// git has no `merge --continue` on older versions, and committing the
			// staged result is equivalent.
			runErr = aggit.CommitMerge(p)
		default:
			runErr = aggit.RebaseContinue(p)
		}
		if runErr != nil {
			// Still conflicted, or conflicts were not staged. Report what remains
			// rather than the raw git error.
			after, _ := aggit.IntegrationStateWithConflicts(p)
			if after.InProgress() {
				rr.Status = "conflicted"
				rr.Conflicts = after.ConflictPaths
				rr.Detail = fmt.Sprintf("%d file(s) still unresolved", len(after.ConflictPaths))
			} else {
				rr.Status = "failed"
				rr.Detail = fmt.Sprintf("%s could not run", operation)
				rr.GitOutput = gitOutputOf(runErr)
			}
			result.Repositories = append(result.Repositories, rr)
			continue
		}
		if after, err := aggit.IntegrationStateWithConflicts(p); err == nil && after.InProgress() {
			// A multi-commit rebase stops again on the next conflicting commit.
			rr.Status = "conflicted"
			rr.Conflicts = after.ConflictPaths
			rr.Detail = fmt.Sprintf("advanced to %d/%d, conflicted again", after.Step, after.Total)
			result.Repositories = append(result.Repositories, rr)
			continue
		}
		rr.Status = operation + "d" // continued | aborted
		rr.Detail = "integration " + rr.Status
		result.Repositories = append(result.Repositories, rr)
	}
	return result, nil
}

// selectRepos returns the Feature's repositories, or just the named one.
func selectRepos(m Metadata, repoFilter string) []RepoMeta {
	if repoFilter == "" {
		return m.Repositories
	}
	for _, r := range m.Repositories {
		if r.Name == repoFilter {
			return []RepoMeta{r}
		}
	}
	return nil
}

// resolveUpstream picks the ref to integrate from, preferring the origin-side
// Base Branch so the result reflects what other people have pushed. An explicit
// override is used as given, which is how the graph page integrates from a ref
// the user clicked.
func resolveUpstream(worktreePath, base, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if base == "" {
		return "", fmt.Errorf("no base branch configured")
	}
	if aggit.RemoteBranchExists(worktreePath, base) {
		return "origin/" + base, nil
	}
	if aggit.LocalBranchExists(worktreePath, base) {
		return base, nil
	}
	return "", fmt.Errorf("base branch %q not found", base)
}
