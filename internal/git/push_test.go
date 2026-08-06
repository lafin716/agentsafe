package git

import (
	"strings"
	"testing"
)

// A rebased branch can only reach origin through a force push, and the lease is
// the only thing standing between "my rewrite" and "somebody else's commits".
// These pin the argument construction, which is where the safety actually lives.

func TestPushWithLeaseArgsNamesTheExpectedRemoteSHA(t *testing.T) {
	args := pushWithLeaseArgs("feature/login", "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0")

	joined := strings.Join(args, " ")
	want := "--force-with-lease=feature/login:a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0"
	if !strings.Contains(joined, want) {
		t.Errorf("args = %v, want them to carry %q", args, want)
	}
	// The argument-less form reads the branch's upstream to decide what it is
	// leasing against, and configureWorktreeUpstream points a fresh feature
	// branch's upstream at the *base* branch until the first push — so what it
	// would compare against is not knowable here. See docs/adr/0003.
	for _, arg := range args {
		if arg == "--force-with-lease" {
			t.Error("args use the argument-less --force-with-lease; the expected SHA must be explicit")
		}
	}
}

func TestPushWithLeaseArgsNeverFallBackToPlainForce(t *testing.T) {
	for _, sha := range []string{"abc123", ""} {
		for _, arg := range pushWithLeaseArgs("feature/login", sha) {
			if arg == "--force" || arg == "-f" {
				t.Errorf("args for sha %q contain %q; a refused lease must never be retried with --force", sha, arg)
			}
		}
	}
}

func TestPushWithLeaseArgsWithoutARemoteSHAIsAPlainFirstPush(t *testing.T) {
	// origin has no such branch yet, so there is nothing to overwrite and
	// nothing to lease against.
	args := pushWithLeaseArgs("feature/login", "")

	joined := strings.Join(args, " ")
	if strings.Contains(joined, "force") {
		t.Errorf("args = %v, want a plain push when origin has no branch", args)
	}
	if !strings.Contains(joined, "-u origin feature/login") {
		t.Errorf("args = %v, want an upstream-setting push", args)
	}
}

func TestPushWithLeaseArgsSetsUpstream(t *testing.T) {
	// Without -u the branch keeps tracking whatever configureWorktreeUpstream
	// pointed it at, so the next plain push would target the base branch.
	args := pushWithLeaseArgs("feature/login", "abc123")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-u origin feature/login") {
		t.Errorf("args = %v, want -u origin <branch>", args)
	}
}
