package people

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGiteaLookupReturnsExactCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v1/users/octocat" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "token repo-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":71,"login":"octocat","full_name":"The Octocat"}`))
	}))
	t.Cleanup(server.Close)

	lookup := NewGiteaLookupClient(server.Client())
	result, err := lookup.Lookup(context.Background(), LookupRequest{
		Provider: "gitea", BaseURL: server.URL, Token: "repo-token", Username: "octocat",
	})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(result.Candidates) != 1 || result.ExactMatchIndex == nil {
		t.Fatalf("expected exact candidate, got %#v", result)
	}
	candidate := result.Candidates[*result.ExactMatchIndex]
	if candidate.ExternalUserID != "71" || candidate.Username != "octocat" || candidate.DisplayName != "The Octocat" {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
}

// 404(查无此人)不是故障:邮箱前缀猜用户名未命中是常态,应返回空候选而非错误,
// 让批量补全继续、单条查询提示手工填写。
func TestGiteaLookupTreatsMissingUserAsNoCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	lookup := NewGiteaLookupClient(server.Client())
	result, err := lookup.Lookup(context.Background(), LookupRequest{
		Provider: "gitea", BaseURL: server.URL, Token: "repo-token", Email: "missing@example.com",
	})
	if err != nil {
		t.Fatalf("missing user must not fail the lookup: %v", err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", result)
	}
}

func TestGiteaLookupReportsAuthFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	lookup := NewGiteaLookupClient(server.Client())
	_, err := lookup.Lookup(context.Background(), LookupRequest{
		Provider: "gitea", BaseURL: server.URL, Token: "bad", Email: "user@example.com",
	})
	if !errors.Is(err, ErrGiteaAuthFailed) {
		t.Fatalf("expected auth failure sentinel, got %v", err)
	}
}

func TestGiteaLookupReportsUnavailableServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	lookup := NewGiteaLookupClient(server.Client())
	if _, err := lookup.Lookup(context.Background(), LookupRequest{
		Provider: "gitea", BaseURL: server.URL, Token: "token", Email: "user@example.com",
	}); !errors.Is(err, ErrGiteaUnavailable) {
		t.Fatalf("expected unavailable sentinel, got %v", err)
	}
}
