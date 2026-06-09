package forge

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		url  string
		want Kind
	}{
		{"https://github.com/acme/backend.git", GitHub},
		{"git@github.com:acme/backend.git", GitHub},
		{"https://gitlab.example.com/group/sub/repo.git", GitLab},
		{"git@gitlab.com:acme/repo.git", GitLab},
		{"https://bitbucket.org/acme/repo.git", Unknown},
	}
	for _, c := range cases {
		if got := Detect(c.url); got != c.want {
			t.Errorf("Detect(%q)=%q want %q", c.url, got, c.want)
		}
	}
}

func TestParseHostOwnerRepo(t *testing.T) {
	cases := []struct {
		url, host, owner, repo string
	}{
		{"https://github.com/acme/backend.git", "github.com", "acme", "backend"},
		{"git@github.com:acme/backend.git", "github.com", "acme", "backend"},
		{"https://gitlab.example.com/group/sub/repo.git", "gitlab.example.com", "group/sub", "repo"},
		{"git@gitlab.com:group/sub/repo.git", "gitlab.com", "group/sub", "repo"},
		{"https://github.com/acme/backend", "github.com", "acme", "backend"},
	}
	for _, c := range cases {
		host, owner, repo, err := ParseHostOwnerRepo(c.url)
		if err != nil {
			t.Errorf("ParseHostOwnerRepo(%q) error: %v", c.url, err)
			continue
		}
		if host != c.host || owner != c.owner || repo != c.repo {
			t.Errorf("ParseHostOwnerRepo(%q)=(%q,%q,%q) want (%q,%q,%q)",
				c.url, host, owner, repo, c.host, c.owner, c.repo)
		}
	}
}

func TestNewRequestURL(t *testing.T) {
	gh, err := NewRequestURL("https://github.com/acme/backend.git", "feature/x", "main", "My PR")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://github.com/acme/backend/compare/main...feature%2Fx?expand=1&title=My+PR"
	if gh != want {
		t.Errorf("github url=\n%q\nwant\n%q", gh, want)
	}

	gl, err := NewRequestURL("git@gitlab.example.com:group/sub/repo.git", "feature/x", "develop", "My MR")
	if err != nil {
		t.Fatal(err)
	}
	const glPrefix = "https://gitlab.example.com/group/sub/repo/-/merge_requests/new?"
	if len(gl) < len(glPrefix) || gl[:len(glPrefix)] != glPrefix {
		t.Errorf("gitlab url=%q want prefix %q", gl, glPrefix)
	}
}

func TestNewRequestURLUnknown(t *testing.T) {
	if _, err := NewRequestURL("https://bitbucket.org/a/b.git", "s", "t", "x"); err == nil {
		t.Error("expected error for unknown provider")
	}
}
