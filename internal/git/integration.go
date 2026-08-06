package git

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Interrupted Integration: a rebase or merge that stopped on conflict, leaving
// a Repo Worktree in a partial state. Rebase and merge deliberately leave this
// state behind rather than aborting (docs/adr/0002), so every screen that shows
// a worktree has to be able to name it — mid-rebase the HEAD is detached and
// `branch --show-current` answers with an empty string, which would otherwise
// read as "a Feature with no branch".
//
// Detection is filesystem-only so it is cheap enough to fold into the
// per-repository status that the Features screens load on every render.
// Listing the conflicted paths does cost a subprocess, so it lives in
// IntegrationConflicts and is called only once a state is known to be open.

type IntegrationKind string

const (
	IntegrationNone   IntegrationKind = ""
	IntegrationRebase IntegrationKind = "rebase"
	IntegrationMerge  IntegrationKind = "merge"
)

type IntegrationState struct {
	Kind IntegrationKind `json:"kind,omitempty" yaml:"kind,omitempty"`
	// Branch is the branch being replayed (rebase) or merged into (merge).
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`
	// Onto is the commit a rebase is replaying onto, or the commit being merged
	// in. Always a raw SHA: git records no name for it, and resolving one would
	// cost a subprocess on a hot path.
	Onto string `json:"onto,omitempty" yaml:"onto,omitempty"`
	// Step and Total describe rebase progress ("2 of 3"). Both zero for a merge.
	Step  int `json:"step,omitempty"  yaml:"step,omitempty"`
	Total int `json:"total,omitempty" yaml:"total,omitempty"`
	// Summary is the merge message subject, when git recorded one.
	Summary       string   `json:"summary,omitempty" yaml:"summary,omitempty"`
	ConflictPaths []string `json:"conflictPaths"     yaml:"conflictPaths"`
}

func (s IntegrationState) InProgress() bool { return s.Kind != IntegrationNone }

// resolveGitDir finds the git admin directory for a work tree. For a Main Clone
// that is <path>/.git; for a Repo Worktree, .git is a file holding
// "gitdir: <path to the Main Clone's worktrees/<name>>".
func resolveGitDir(path string) (string, error) {
	dot := filepath.Join(path, ".git")
	info, err := os.Stat(dot)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return dot, nil
	}
	raw, err := os.ReadFile(dot)
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), "gitdir:"))
	if target == "" {
		return "", os.ErrNotExist
	}
	target = filepath.FromSlash(target)
	if !filepath.IsAbs(target) {
		target = filepath.Join(path, target)
	}
	return filepath.Clean(target), nil
}

// IntegrationStateOf reports whether worktreePath is mid-rebase or mid-merge. A
// path that is not a repository is reported as "none" rather than an error, so
// callers can ask about any workspace directory.
func IntegrationStateOf(worktreePath string) (IntegrationState, error) {
	state := IntegrationState{ConflictPaths: []string{}}
	gitDir, err := resolveGitDir(worktreePath)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	// Rebase is checked first: a conflicted rebase also leaves MERGE_MSG (and,
	// with some backends, MERGE_HEAD) behind while auto-merging.
	for _, backend := range []struct {
		dir, step, total string
	}{
		{"rebase-merge", "msgnum", "end"},
		{"rebase-apply", "next", "last"},
	} {
		dir := filepath.Join(gitDir, backend.dir)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		state.Kind = IntegrationRebase
		state.Branch = NormalizeBranchName(readTrimmed(filepath.Join(dir, "head-name")))
		state.Onto = readTrimmed(filepath.Join(dir, "onto"))
		state.Step = readInt(filepath.Join(dir, backend.step))
		state.Total = readInt(filepath.Join(dir, backend.total))
		return state, nil
	}
	if mergeHead := readTrimmed(filepath.Join(gitDir, "MERGE_HEAD")); mergeHead != "" {
		state.Kind = IntegrationMerge
		state.Onto = mergeHead
		state.Branch = headBranch(gitDir)
		state.Summary = firstLine(readTrimmed(filepath.Join(gitDir, "MERGE_MSG")))
		return state, nil
	}
	return state, nil
}

// IntegrationConflicts lists the unmerged paths of an open integration. Costs
// one git subprocess, so callers should only reach for it once
// IntegrationStateOf reports a state in progress.
func IntegrationConflicts(worktreePath string) ([]string, error) {
	res, err := Run(worktreePath, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for _, p := range strings.Split(strings.Trim(res.Stdout, "\x00"), "\x00") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// IntegrationStateWithConflicts reads the state and, when one is open, fills in
// the conflicted paths.
func IntegrationStateWithConflicts(worktreePath string) (IntegrationState, error) {
	state, err := IntegrationStateOf(worktreePath)
	if err != nil || !state.InProgress() {
		return state, err
	}
	if paths, err := IntegrationConflicts(worktreePath); err == nil {
		state.ConflictPaths = paths
	}
	return state, nil
}

// headBranch reads the branch a symbolic HEAD points at, or "" when detached.
func headBranch(gitDir string) string {
	head := readTrimmed(filepath.Join(gitDir, "HEAD"))
	if rest, ok := strings.CutPrefix(head, "ref: "); ok {
		return NormalizeBranchName(rest)
	}
	return ""
}

func readTrimmed(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func readInt(path string) int {
	n, err := strconv.Atoi(readTrimmed(path))
	if err != nil {
		return 0
	}
	return n
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	return strings.TrimSpace(line)
}

// MergeOnto merges upstream into the branch checked out at worktreePath. This is
// the only merge direction agentsafe performs: a Base Branch into a Feature's
// branch, never the reverse (docs/adr/0001). --no-edit keeps git from opening an
// editor, which would hang a non-interactive run.
func MergeOnto(worktreePath, upstream string) error {
	_, err := Run(worktreePath, "merge", "--no-edit", upstream)
	return err
}

func MergeAbort(worktreePath string) error {
	_, err := Run(worktreePath, "merge", "--abort")
	return err
}

// CommitMerge concludes a merge whose conflicts have been resolved and staged.
// This is what "continue" means for a merge: `git merge --continue` only exists
// on newer git, and committing the staged result is what it does.
func CommitMerge(worktreePath string) error {
	_, err := run(worktreePath, []string{"GIT_EDITOR=true"}, "commit", "--no-edit")
	return err
}

// RebaseContinue resumes an interrupted rebase. GIT_EDITOR=true stands in for an
// editor so git accepts the existing commit message instead of waiting for one.
func RebaseContinue(worktreePath string) error {
	_, err := run(worktreePath, []string{"GIT_EDITOR=true"}, "rebase", "--continue")
	return err
}

// RemoteBranchSHA reads what origin/<branch> currently points at, which is the
// expected value a lease is built from. Fails when the ref does not exist.
func RemoteBranchSHA(path, branch string) (string, error) {
	return Output(path, "rev-parse", "--verify", "refs/remotes/origin/"+NormalizeBranchName(branch))
}

// PushWithLease updates origin/<branch> to the local branch, refusing the push
// if origin/<branch> is not still at expectedSHA. A Feature branch that was
// rebased cannot reach origin any other way, and the lease is what stops the
// rewrite from discarding commits someone else pushed in the meantime.
//
// The expected SHA is always explicit. See docs/adr/0003 for why the
// argument-less --force-with-lease is not used here. A refused lease is never
// retried with --force.
func PushWithLease(path, branch, expectedSHA string) error {
	_, err := Run(path, pushWithLeaseArgs(branch, expectedSHA)...)
	return err
}

// pushWithLeaseArgs builds the push arguments. Split out so the safety-critical
// part is a pure function the tests can pin without running git.
func pushWithLeaseArgs(branch, expectedSHA string) []string {
	if expectedSHA == "" {
		// Nothing on origin to overwrite, so there is nothing to lease against.
		return []string{"push", "-u", "origin", branch}
	}
	return []string{
		"push", "--force-with-lease=" + branch + ":" + expectedSHA,
		"-u", "origin", branch,
	}
}
