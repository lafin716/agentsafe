package git

import (
	"strings"
	"testing"
)

// rec builds one --format record using the same separators LogFormat asks git
// for, so the fixtures below stay honest about the real wire format.
func rec(fields ...string) string {
	out := ""
	for i, f := range fields {
		if i > 0 {
			out += "\x00"
		}
		out += f
	}
	return out + "\x1e"
}

func TestParseLogReadsEveryField(t *testing.T) {
	raw := rec(
		"d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3",
		"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
		"Ju Park",
		"jupark@kicc.co.kr",
		"2026-08-03T14:12:05+09:00",
		"HEAD -> refs/heads/feat/login, refs/remotes/origin/feat/login",
		"add oauth guard",
	)

	commits := ParseLog(raw)

	if len(commits) != 1 {
		t.Fatalf("commit count = %d, want 1", len(commits))
	}
	c := commits[0]
	if c.SHA != "d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3" {
		t.Errorf("SHA = %q", c.SHA)
	}
	if len(c.Parents) != 1 || c.Parents[0] != "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0" {
		t.Errorf("Parents = %v", c.Parents)
	}
	if c.AuthorName != "Ju Park" || c.AuthorEmail != "jupark@kicc.co.kr" {
		t.Errorf("author = %q <%q>", c.AuthorName, c.AuthorEmail)
	}
	if c.AuthorDate != "2026-08-03T14:12:05+09:00" {
		t.Errorf("AuthorDate = %q", c.AuthorDate)
	}
	if c.Subject != "add oauth guard" {
		t.Errorf("Subject = %q", c.Subject)
	}
	if !c.IsHead {
		t.Error("IsHead = false, want true for a HEAD -> decoration")
	}
	want := []Ref{
		{Name: "feat/login", Kind: RefHead},
		{Name: "origin/feat/login", Kind: RefRemote},
	}
	if len(c.Refs) != len(want) {
		t.Fatalf("Refs = %v, want %v", c.Refs, want)
	}
	for i := range want {
		if c.Refs[i] != want[i] {
			t.Errorf("Refs[%d] = %v, want %v", i, c.Refs[i], want[i])
		}
	}
}

func TestParseLogClassifiesRefKinds(t *testing.T) {
	raw := rec("aaa", "", "n", "e", "d",
		"refs/tags/v1.2.0, refs/heads/main, refs/remotes/origin/main, refs/remotes/upstream/main",
		"init")

	commits := ParseLog(raw)
	if len(commits) != 1 {
		t.Fatalf("commit count = %d, want 1", len(commits))
	}
	got := map[string]RefKind{}
	for _, r := range commits[0].Refs {
		got[r.Name] = r.Kind
	}
	for name, kind := range map[string]RefKind{
		"v1.2.0":        RefTag,
		"main":          RefHead,
		"origin/main":   RefRemote,
		"upstream/main": RefRemote,
	} {
		if got[name] != kind {
			t.Errorf("%s kind = %q, want %q", name, got[name], kind)
		}
	}
	if commits[0].IsHead {
		t.Error("IsHead = true, want false when no HEAD decoration is present")
	}
}

func TestParseLogHandlesMergeAndRootCommits(t *testing.T) {
	raw := rec("merge1", "p1 p2", "n", "e", "d", "", "merge base") +
		rec("root1", "", "n", "e", "d", "", "initial commit")

	commits := ParseLog(raw)

	if len(commits) != 2 {
		t.Fatalf("commit count = %d, want 2", len(commits))
	}
	if len(commits[0].Parents) != 2 ||
		commits[0].Parents[0] != "p1" || commits[0].Parents[1] != "p2" {
		t.Errorf("merge parents = %v, want [p1 p2]", commits[0].Parents)
	}
	// A root commit must carry an empty (non-nil) parent slice so it serializes
	// to [] rather than null for the frontend lane layout.
	if commits[1].Parents == nil {
		t.Error("root commit Parents = nil, want empty slice")
	}
	if len(commits[1].Parents) != 0 {
		t.Errorf("root commit Parents = %v, want empty", commits[1].Parents)
	}
}

func TestParseLogKeepsSubjectsContainingSeparatorLookalikes(t *testing.T) {
	raw := rec("aaa", "", "n", "e", "d", "", "fix: handle a, b and c -> d")

	commits := ParseLog(raw)

	if len(commits) != 1 {
		t.Fatalf("commit count = %d, want 1", len(commits))
	}
	if commits[0].Subject != "fix: handle a, b and c -> d" {
		t.Errorf("Subject = %q", commits[0].Subject)
	}
}

func TestParseLogIgnoresBlankAndMalformedRecords(t *testing.T) {
	// Trailing newline after the final record separator, plus a short record
	// that git would never emit but a truncated read might.
	raw := rec("aaa", "", "n", "e", "d", "", "ok") + "\n" +
		"short\x00record\x1e" + "\n"

	commits := ParseLog(raw)

	if len(commits) != 1 {
		t.Fatalf("commit count = %d, want 1 (malformed record dropped)", len(commits))
	}
	if commits[0].SHA != "aaa" {
		t.Errorf("SHA = %q", commits[0].SHA)
	}
}

func TestParseLogStripsCarriageReturnsBetweenRecords(t *testing.T) {
	raw := rec("aaa", "", "n", "e", "d", "", "one") + "\r\n" +
		rec("bbb", "aaa", "n", "e", "d", "", "two")

	commits := ParseLog(raw)

	if len(commits) != 2 {
		t.Fatalf("commit count = %d, want 2", len(commits))
	}
	if commits[1].SHA != "bbb" {
		t.Errorf("commits[1].SHA = %q, want bbb", commits[1].SHA)
	}
}

func TestParseRefListPairsNamesWithTips(t *testing.T) {
	raw := "refs/heads/main\x00a1b2c3\n" +
		"refs/remotes/origin/main\x00a1b2c3\n" +
		"refs/remotes/origin/HEAD\x00a1b2c3\n" +
		"refs/tags/v1.0\x00ffee00\n"

	refs := ParseRefList(raw)

	if len(refs) != 3 {
		t.Fatalf("ref count = %d, want 3 (origin/HEAD dropped), got %v", len(refs), refs)
	}
	byName := map[string]RefTip{}
	for _, r := range refs {
		byName[r.Name] = r
	}
	if got := byName["main"]; got.Kind != RefHead || got.SHA != "a1b2c3" {
		t.Errorf("main = %+v", got)
	}
	if got := byName["origin/main"]; got.Kind != RefRemote || got.SHA != "a1b2c3" {
		t.Errorf("origin/main = %+v", got)
	}
	if got := byName["v1.0"]; got.Kind != RefTag || got.SHA != "ffee00" {
		t.Errorf("v1.0 = %+v", got)
	}
}

// Windows rejects an argv entry containing a NUL byte outright —
// UTF16PtrFromString answers EINVAL and the exec fails with "invalid argument"
// before git ever runs. The separators therefore have to be asked for with git's
// %xNN escapes, which put the bytes in the output instead of the argument.
func TestLogFormatArgumentsCarryNoNulByte(t *testing.T) {
	for _, arg := range []string{logFormat, refListFormat} {
		if strings.ContainsRune(arg, 0) {
			t.Errorf("format argument %q contains a NUL byte; use git's %%xNN escape "+
				"so the byte appears in the output rather than in argv", arg)
		}
	}
}

func TestLogRefArgsUsesManagedRefsUnlessAllRequested(t *testing.T) {
	managed := []string{"main", "origin/main", "feat/login"}

	got := LogRefArgs(managed, false)
	want := []string{"main", "origin/main", "feat/login"}
	if len(got) != len(want) {
		t.Fatalf("managed args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("managed args = %v, want %v", got, want)
		}
	}

	all := LogRefArgs(managed, true)
	if len(all) != 1 || all[0] != "--all" {
		t.Errorf("all args = %v, want [--all]", all)
	}
}
