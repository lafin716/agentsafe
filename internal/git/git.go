package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	Command string
	Dir     string
	Stdout  string
	Stderr  string
}

type Error struct {
	Result  Result
	Err     error
	Timeout bool
}

func (e *Error) Error() string {
	if e.Timeout {
		return fmt.Sprintf("git command timed out: %s\nReason: command exceeded timeout\nstdout: %s\nstderr: %s\nSuggestion: check Git authentication/network, then retry. agentsafe runs Git non-interactively to avoid hanging.", e.Result.Command, e.Result.Stdout, e.Result.Stderr)
	}
	return fmt.Sprintf("git command failed: %s\nReason: %v\nstdout: %s\nstderr: %s", e.Result.Command, e.Err, e.Result.Stdout, e.Result.Stderr)
}

func timeout() time.Duration {
	seconds := 120
	if raw := os.Getenv("AGENTSAFE_GIT_TIMEOUT_SECONDS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			seconds = n
		}
	}
	return time.Duration(seconds) * time.Second
}

func Run(dir string, args ...string) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	)
	hideWindow(cmd) // suppress the per-process console window on Windows GUI apps
	var out, er bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &er
	err := cmd.Run()
	res := Result{Command: "git " + strings.Join(args, " "), Dir: dir, Stdout: out.String(), Stderr: er.String()}
	if err != nil {
		return res, &Error{Result: res, Err: err, Timeout: ctx.Err() == context.DeadlineExceeded}
	}
	return res, nil
}

func Output(dir string, args ...string) (string, error) {
	r, err := Run(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Stdout), nil
}

func Fetch(repoPath string) error { _, err := Run(repoPath, "fetch", "origin"); return err }

// FetchBranch fetches just one branch from origin (updating FETCH_HEAD and the
// matching origin/<branch> ref). Cheaper than a full fetch when only the base
// branch is needed.
func FetchBranch(repoPath, branch string) error {
	_, err := Run(repoPath, "fetch", "origin", branch)
	return err
}
func FetchAll(repoPath string) error {
	_, err := Run(repoPath, "fetch", "--all", "--prune")
	return err
}
func Checkout(repoPath, branch string) error { _, err := Run(repoPath, "checkout", branch); return err }
func Pull(repoPath, remote, branch string) error {
	_, err := Run(repoPath, "pull", "--ff-only", remote, branch)
	return err
}
func LocalBranchExists(repoPath, branch string) bool {
	_, err := Run(repoPath, "rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}
func RemoteBranchExists(repoPath, branch string) bool {
	_, err := Run(repoPath, "rev-parse", "--verify", "refs/remotes/origin/"+branch)
	return err == nil
}
func StatusShort(path string) (string, error)   { return Output(path, "status", "--short") }
func CurrentBranch(path string) (string, error) { return Output(path, "branch", "--show-current") }
func HasChanges(path string) bool               { s, err := StatusShort(path); return err == nil && s != "" }
func CommitAll(path, message string) error {
	if _, err := Run(path, "add", "."); err != nil {
		return err
	}
	_, err := Run(path, "commit", "-m", message)
	return err
}
func Push(path, branch string) error { _, err := Run(path, "push", "-u", "origin", branch); return err }
func DeleteLocalBranch(repoPath, branch string) error {
	_, err := Run(repoPath, "branch", "-D", branch)
	return err
}
func RebaseOnto(worktreePath, upstream string) error {
	_, err := Run(worktreePath, "rebase", upstream)
	return err
}
func RebaseAbort(worktreePath string) error {
	_, err := Run(worktreePath, "rebase", "--abort")
	return err
}
func HeadSHA(path string) (string, error) { return Output(path, "rev-parse", "HEAD") }
func AddWorktree(repoPath, dest, branch, start string, create bool) error {
	if create {
		_, err := Run(repoPath, "worktree", "add", dest, "-b", branch, start)
		return err
	}
	_, err := Run(repoPath, "worktree", "add", dest, branch)
	return err
}
