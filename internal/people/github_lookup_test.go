package people

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubLookupReturnsExactCandidate(t *testing.T) {
	var seenAuth string
	var seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.EscapedPath()
		if seenPath != "/users/octocat" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":5823,"login":"octocat","name":"The Octocat"}`))
	}))
	t.Cleanup(server.Close)

	lookup := NewGitHubLookupClient(server.Client())
	result, err := lookup.Lookup(context.Background(), LookupRequest{
		Provider: "github", BaseURL: server.URL, Token: "repo-token", Username: "octocat",
	})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if seenAuth != "Bearer repo-token" {
		t.Fatalf("expected bearer auth, got %q", seenAuth)
	}
	if len(result.Candidates) != 1 || result.ExactMatchIndex == nil {
		t.Fatalf("expected exact candidate, got %#v", result)
	}
	candidate := result.Candidates[*result.ExactMatchIndex]
	if candidate.ExternalUserID != "5823" || candidate.Username != "octocat" || candidate.DisplayName != "The Octocat" {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
}

func TestGitHubLookupRejectsUnavailableServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	lookup := NewGitHubLookupClient(server.Client())
	if _, err := lookup.Lookup(context.Background(), LookupRequest{
		Provider: "github", BaseURL: server.URL, Token: "bad", Username: "octocat",
	}); err == nil {
		t.Fatal("expected lookup failure on non-200")
	}
}

// 404(查无此人)不是故障:返回空候选,批次继续、单条查询提示手工填写。
func TestGitHubLookupTreatsMissingUserAsNoCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	lookup := NewGitHubLookupClient(server.Client())
	result, err := lookup.Lookup(context.Background(), LookupRequest{
		Provider: "github", BaseURL: server.URL, Token: "repo-token", Email: "missing@example.com",
	})
	if err != nil {
		t.Fatalf("missing user must not fail the lookup: %v", err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", result)
	}
}

func TestGitHubLookupReportsAuthFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	lookup := NewGitHubLookupClient(server.Client())
	_, err := lookup.Lookup(context.Background(), LookupRequest{
		Provider: "github", BaseURL: server.URL, Token: "bad", Username: "octocat",
	})
	if !errors.Is(err, ErrGitHubAuthFailed) {
		t.Fatalf("expected auth failure sentinel, got %v", err)
	}
}
