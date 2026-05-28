package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Result struct {
	Command string
	Dir     string
	Stdout  string
	Stderr  string
}

type Error struct {
	Result Result
	Err    error
}

func (e *Error) Error() string {
	return fmt.Sprintf("git command failed: %s\nReason: %v\nstdout: %s\nstderr: %s", e.Result.Command, e.Err, e.Result.Stdout, e.Result.Stderr)
}

func Run(dir string, args ...string) (Result, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, er bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &er
	err := cmd.Run()
	res := Result{Command: "git " + strings.Join(args, " "), Dir: dir, Stdout: out.String(), Stderr: er.String()}
	if err != nil {
		return res, &Error{Result: res, Err: err}
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
func FetchAll(repoPath string) error {
	_, err := Run(repoPath, "fetch", "--all", "--prune")
	return err
}
func Checkout(repoPath, branch string) error { _, err := Run(repoPath, "checkout", branch); return err }
func Pull(repoPath, remote, branch string) error {
	_, err := Run(repoPath, "pull", remote, branch)
	return err
}
func LocalBranchExists(repoPath, branch string) bool {
	_, err := Run(repoPath, "rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}
func RemoteBranchExists(repoPath, branch string) bool {
	_, err := Run(repoPath, "ls-remote", "--exit-code", "--heads", "origin", branch)
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
func AddWorktree(repoPath, dest, branch, start string, create bool) error {
	if create {
		_, err := Run(repoPath, "worktree", "add", dest, "-b", branch, start)
		return err
	}
	_, err := Run(repoPath, "worktree", "add", dest, branch)
	return err
}
