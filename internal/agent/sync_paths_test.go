package agent

import (
	"strings"
	"testing"
)

func changeSet() map[string][]Change {
	return map[string][]Change{
		"api": {
			{Repo: "api", Path: "src/main.go", Type: Modified},
			{Repo: "api", Path: "src/util.go", Type: Modified},
			{Repo: "api", Path: "README.md", Type: Added},
		},
		"web": {
			{Repo: "web", Path: "src/main.go", Type: Modified},
		},
	}
}

func TestFilterChangesByPathKeepsOnlyTheRequestedFiles(t *testing.T) {
	got, err := filterChangesByPath(changeSet(), "api", []string{"src/util.go"})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("repos = %d, want only the filtered one: %v", len(got), got)
	}
	if len(got["api"]) != 1 || got["api"][0].Path != "src/util.go" {
		t.Errorf("changes = %v, want just src/util.go", got["api"])
	}
}

func TestFilterChangesByPathWithNoPathsIsTheWholeSet(t *testing.T) {
	all := changeSet()

	got, err := filterChangesByPath(all, "api", nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Errorf("repos = %d, want the untouched set", len(got))
	}
}

func TestFilterChangesByPathDoesNotReachIntoAnotherRepository(t *testing.T) {
	// Both repos have a src/main.go. Resolving one must not touch the other.
	got, err := filterChangesByPath(changeSet(), "api", []string{"src/main.go"})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := got["web"]; ok {
		t.Errorf("result includes web: %v", got)
	}
	if len(got["api"]) != 1 {
		t.Errorf("api changes = %v, want one", got["api"])
	}
}

func TestFilterChangesByPathRejectsAPathWithNoChange(t *testing.T) {
	// Silently syncing nothing would show as "resolved" while the agent copy
	// still differs.
	_, err := filterChangesByPath(changeSet(), "api", []string{"src/util.go", "src/gone.go"})

	if err == nil {
		t.Fatal("want an error naming the path that has no agent change")
	}
	if !strings.Contains(err.Error(), "src/gone.go") {
		t.Errorf("error %q does not name the missing path", err)
	}
}

func TestFilterChangesByPathRequiresARepository(t *testing.T) {
	// A bare filename exists in more than one repository; guessing would write
	// into a worktree the user was not looking at.
	_, err := filterChangesByPath(changeSet(), "", []string{"src/main.go"})

	if err == nil {
		t.Fatal("want an error when paths are given without a repository")
	}
}

func TestFilterChangesByPathAcceptsBackslashSeparators(t *testing.T) {
	// The desktop UI hands back whatever the platform produced; Change.Path is
	// always slash-separated.
	got, err := filterChangesByPath(changeSet(), "api", []string{`src\util.go`})
	if err != nil {
		t.Fatal(err)
	}

	if len(got["api"]) != 1 || got["api"][0].Path != "src/util.go" {
		t.Errorf("changes = %v, want src/util.go", got["api"])
	}
}
