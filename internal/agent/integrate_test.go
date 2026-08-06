package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
)

// The gate these cover is what stops reviewed work being thrown away: Diff
// compares the agent copy against the Repo Worktree file by file, so after a
// rebase every file the rebase touched looks like an Agent Change, and a later
// Sync would copy the pre-rebase agent copy back over the rebased worktree with
// no error anywhere.

// featureWithRepos writes a Feature whose repositories have worktree directories
// containing one file each. No git is involved, which keeps these fast.
func featureWithRepos(t *testing.T, root, featureName string, repos ...string) {
	t.Helper()
	metas := make([]feature.RepoMeta, 0, len(repos))
	for _, repoName := range repos {
		rel := filepath.Join("feature", featureName, repoName)
		writeIndexedFile(t, filepath.Join(root, rel, "file.txt"), "content")
		metas = append(metas, feature.RepoMeta{
			Name:         repoName,
			WorktreePath: filepath.ToSlash(rel),
		})
	}
	if err := feature.Save(root, feature.Metadata{
		Name: featureName, Key: featureName, Repositories: metas,
	}); err != nil {
		t.Fatal(err)
	}
}

func readinessFor(t *testing.T, r IntegrationReadiness, repo string) RepoIntegrationReadiness {
	t.Helper()
	for _, row := range r.Repositories {
		if row.Repo == repo {
			return row
		}
	}
	t.Fatalf("no readiness row for %q in %+v", repo, r.Repositories)
	return RepoIntegrationReadiness{}
}

func TestCheckIntegrationAllowsARepositoryWithNoAgentWorkspace(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	featureWithRepos(t, root, "demo", "repo")

	readiness, err := CheckIntegration(root, cfg, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	row := readinessFor(t, readiness, "repo")
	if row.AgentPrepared {
		t.Error("AgentPrepared = true, want false when nothing was prepared")
	}
	if row.Blocked {
		t.Errorf("Blocked = true (%q); there is no agent copy to strand", row.Reason)
	}
	if row.StaleAfter {
		t.Error("StaleAfter = true, want false when nothing was prepared")
	}
}

func TestCheckIntegrationAllowsAPreparedRepositoryWithNoAgentChanges(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	featureWithRepos(t, root, "demo", "repo")
	if err := Init(root, cfg, "demo", PrepareOptions{}); err != nil {
		t.Fatal(err)
	}

	readiness, err := CheckIntegration(root, cfg, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	row := readinessFor(t, readiness, "repo")
	if !row.AgentPrepared {
		t.Fatal("AgentPrepared = false, want true after Init")
	}
	if row.Blocked {
		t.Errorf("Blocked = true (%q), want allowed with no agent changes", row.Reason)
	}
	if row.AgentChanges != 0 {
		t.Errorf("AgentChanges = %d, want 0", row.AgentChanges)
	}
	// The agent copy will not match the integrated worktree afterwards, and the
	// user has to be told to prepare it again.
	if !row.StaleAfter {
		t.Error("StaleAfter = false, want true for a prepared repository")
	}
}

func TestCheckIntegrationBlocksAPreparedRepositoryHoldingAgentChanges(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	featureWithRepos(t, root, "demo", "repo")
	if err := Init(root, cfg, "demo", PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
	// An unreviewed edit in the agent copy.
	writeIndexedFile(t,
		filepath.Join(config.AgentPath(root, "demo", "repo"), "file.txt"), "edited")

	readiness, err := CheckIntegration(root, cfg, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	row := readinessFor(t, readiness, "repo")
	if !row.Blocked {
		t.Fatal("Blocked = false, want the repository refused")
	}
	if row.AgentChanges != 1 {
		t.Errorf("AgentChanges = %d, want 1", row.AgentChanges)
	}
	// The reason has to say what to do about it, not just that it was refused.
	if !strings.Contains(row.Reason, "review") {
		t.Errorf("Reason = %q, want it to tell the user to review the changes", row.Reason)
	}
}

func TestCheckIntegrationJudgesEachRepositorySeparately(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	featureWithRepos(t, root, "demo", "clean", "dirty")
	if err := Init(root, cfg, "demo", PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
	writeIndexedFile(t,
		filepath.Join(config.AgentPath(root, "demo", "dirty"), "file.txt"), "edited")

	readiness, err := CheckIntegration(root, cfg, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if readinessFor(t, readiness, "clean").Blocked {
		t.Error("clean repository was refused")
	}
	if !readinessFor(t, readiness, "dirty").Blocked {
		t.Error("dirty repository was allowed")
	}

	blockers := readiness.Blockers()
	if len(blockers) != 1 || blockers["dirty"] == "" {
		t.Errorf("Blockers = %v, want just dirty", blockers)
	}
}

func TestCheckIntegrationHonoursTheRepositoryFilter(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	featureWithRepos(t, root, "demo", "clean", "dirty")
	if err := Init(root, cfg, "demo", PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
	writeIndexedFile(t,
		filepath.Join(config.AgentPath(root, "demo", "dirty"), "file.txt"), "edited")

	// The commit graph checks one repository at a time, because a graph shows one
	// repository and that is what changes by default.
	readiness, err := CheckIntegration(root, cfg, "demo", "clean")
	if err != nil {
		t.Fatal(err)
	}
	if len(readiness.Repositories) != 1 {
		t.Fatalf("Repositories = %+v, want only the filtered one", readiness.Repositories)
	}
	if readiness.Repositories[0].Repo != "clean" {
		t.Errorf("Repositories[0] = %q, want clean", readiness.Repositories[0].Repo)
	}
	if len(readiness.Blockers()) != 0 {
		t.Errorf("Blockers = %v, want none when the dirty repo is filtered out", readiness.Blockers())
	}
}

func TestWithAgentBlockersKeepsTheCallersOwnReasons(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	featureWithRepos(t, root, "demo", "clean", "dirty")
	if err := Init(root, cfg, "demo", PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
	writeIndexedFile(t,
		filepath.Join(config.AgentPath(root, "demo", "dirty"), "file.txt"), "edited")

	opts, err := withAgentBlockers(root, cfg, "demo", "", feature.IntegrateOptions{
		Blocked: map[string]string{"clean": "caller said no"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Blocked["clean"] != "caller said no" {
		t.Errorf("caller reason for clean = %q, want it preserved", opts.Blocked["clean"])
	}
	if opts.Blocked["dirty"] == "" {
		t.Error("dirty was not blocked by the agent check")
	}
}

func TestWithAgentBlockersDoesNotMutateTheCallersMap(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	featureWithRepos(t, root, "demo", "dirty")
	if err := Init(root, cfg, "demo", PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
	writeIndexedFile(t,
		filepath.Join(config.AgentPath(root, "demo", "dirty"), "file.txt"), "edited")

	callerMap := map[string]string{}
	if _, err := withAgentBlockers(root, cfg, "demo", "", feature.IntegrateOptions{
		Blocked: callerMap,
	}); err != nil {
		t.Fatal(err)
	}
	if len(callerMap) != 0 {
		t.Errorf("caller's map was written to: %v", callerMap)
	}
}

func TestCheckIntegrationFailsForAnUnknownFeature(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")

	if _, err := CheckIntegration(root, cfg, "nope", ""); err == nil {
		t.Error("want an error for a feature that does not exist")
	}
}

// The block runs the other way round from CheckIntegration: an open Interrupted
// Integration leaves the working tree matching no commit, so preparing from it
// or syncing into it would mix reviewed work with an unfinished conflict
// (docs/adr/0002).

// openRebase fakes a conflicted rebase by writing the admin files git leaves
// behind, which is what IntegrationStateOf reads. No git subprocess needed.
func openRebase(t *testing.T, root, featureName, repoName string) {
	t.Helper()
	gitDir := filepath.Join(root, "feature", featureName, repoName, ".git", "rebase-merge")
	writeIndexedFile(t, filepath.Join(gitDir, "head-name"), "refs/heads/feature/demo\n")
	writeIndexedFile(t, filepath.Join(gitDir, "msgnum"), "1\n")
	writeIndexedFile(t, filepath.Join(gitDir, "end"), "2\n")
}

func TestGuardIntegrationInProgressAllowsACleanWorktree(t *testing.T) {
	root := t.TempDir()
	featureWithRepos(t, root, "demo", "api")

	if err := GuardIntegrationInProgress(root, "demo", ""); err != nil {
		t.Errorf("GuardIntegrationInProgress = %v, want nil", err)
	}
}

func TestGuardIntegrationInProgressRefusesAnOpenIntegration(t *testing.T) {
	root := t.TempDir()
	featureWithRepos(t, root, "demo", "api")
	openRebase(t, root, "demo", "api")

	err := GuardIntegrationInProgress(root, "demo", "")
	if err == nil {
		t.Fatal("want an error naming the repository and the operation")
	}
	if !strings.Contains(err.Error(), "api") || !strings.Contains(err.Error(), "rebase") {
		t.Errorf("error = %q, want it to name the repository and the kind", err)
	}
}

func TestGuardIntegrationInProgressIsScopedToTheNamedRepository(t *testing.T) {
	// One repository being mid-rebase must not stop work on the others.
	root := t.TempDir()
	featureWithRepos(t, root, "demo", "api", "web")
	openRebase(t, root, "demo", "api")

	if err := GuardIntegrationInProgress(root, "demo", "web"); err != nil {
		t.Errorf("web should be allowed, got %v", err)
	}
	if err := GuardIntegrationInProgress(root, "demo", "api"); err == nil {
		t.Error("api should be refused")
	}
	// Unfiltered means "every repository", so one open integration blocks it.
	if err := GuardIntegrationInProgress(root, "demo", ""); err == nil {
		t.Error("an unfiltered guard should be refused while any repository is mid-rebase")
	}
}
