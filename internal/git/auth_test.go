package git

import (
	"strings"
	"testing"
)

func TestIsAuthenticationError(t *testing.T) {
	cases := []string{
		"fatal: Authentication failed for 'https://example.com/repo.git/'",
		"fatal: could not read Username for 'https://example.com': terminal prompts disabled",
		"The requested URL returned error: 401",
		"The requested URL returned error: 403",
	}
	for _, message := range cases {
		if !IsAuthenticationError(&fakeError{message: message}) {
			t.Errorf("expected authentication error for %q", message)
		}
	}
	if IsAuthenticationError(&fakeError{message: "fatal: repository not found"}) {
		t.Fatal("repository-not-found must not be classified as authentication")
	}
}

func TestRunWithHTTPAuthDoesNotExposeSecret(t *testing.T) {
	const secret = "top-secret-token"
	result, err := RunWithHTTPAuth(t.TempDir(), "https://example.com/repo.git", "user", secret, "not-a-real-command")
	if err == nil {
		t.Fatal("expected git failure")
	}
	combined := result.Command + result.Stdout + result.Stderr + err.Error()
	if strings.Contains(combined, secret) {
		t.Fatal("secret was exposed in Git result or error")
	}
}

type fakeError struct{ message string }

func (e *fakeError) Error() string { return e.message }
