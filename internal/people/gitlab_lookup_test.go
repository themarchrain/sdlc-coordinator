package people_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sdlc-coordinator/internal/people"
)

func TestGitLabLookupMarksUniqueExactUsernameCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v4/users" {
			http.NotFound(writer, request)
			return
		}
		if got := request.Header.Get("PRIVATE-TOKEN"); got != "temporary-token" {
			t.Fatalf("unexpected private token header %q", got)
		}
		if got := request.URL.Query().Get("search"); got != "alice" {
			t.Fatalf("unexpected search query %q", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `[{"id":7101,"username":"Alice","name":"Alice"},{"id":7102,"username":"alice-old","name":"旧账号"}]`)
	}))
	defer server.Close()

	result, err := people.NewGitLabLookupClient(server.Client()).Lookup(context.Background(), people.LookupRequest{
		Provider: "gitlab", BaseURL: server.URL, Token: "temporary-token", Email: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("lookup GitLab identity: %v", err)
	}
	if len(result.Candidates) != 2 || result.Candidates[0].ExternalUserID != "7101" || result.Candidates[0].Username != "Alice" {
		t.Fatalf("unexpected candidates: %#v", result.Candidates)
	}
	if result.ExactMatchIndex == nil || *result.ExactMatchIndex != 0 {
		t.Fatalf("expected exact candidate index 0, got %#v", result.ExactMatchIndex)
	}
}

func TestGitLabLookupDoesNotExposeTokenInAuthenticationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := people.NewGitLabLookupClient(server.Client()).Lookup(context.Background(), people.LookupRequest{
		Provider: "gitlab", BaseURL: server.URL, Token: "temporary-token", Username: "user",
	})
	if !errors.Is(err, people.ErrGitLabAuthFailed) {
		t.Fatalf("expected authentication error, got %v", err)
	}
	if err != nil && contains(err.Error(), "temporary-token") {
		t.Fatalf("lookup error leaked token: %v", err)
	}
}

func TestGitLabLookupMapsRequestTimeoutToUnavailable(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})}
	lookup := people.NewGitLabLookupClient(client, people.WithGitLabLookupTimeout(time.Millisecond))

	_, err := lookup.Lookup(context.Background(), people.LookupRequest{Provider: "gitlab", BaseURL: "https://gitlab.example.com", Token: "temporary-token", Username: "user"})
	if !errors.Is(err, people.ErrGitLabUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestGitLabLookupNeverForwardsTokenAcrossRedirects(t *testing.T) {
	forwardedToken := ""
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		forwardedToken = request.Header.Get("PRIVATE-TOKEN")
		_, _ = io.WriteString(writer, `[]`)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+"/api/v4/users", http.StatusFound)
	}))
	defer redirector.Close()

	_, err := people.NewGitLabLookupClient(redirector.Client()).Lookup(context.Background(), people.LookupRequest{
		Provider: "gitlab", BaseURL: redirector.URL, Token: "temporary-token", Username: "user",
	})
	if !errors.Is(err, people.ErrGitLabLookupFailed) {
		t.Fatalf("expected redirect to be rejected, got %v", err)
	}
	if forwardedToken != "" {
		t.Fatalf("redirect target received transient token %q", forwardedToken)
	}
}

func contains(value, part string) bool {
	return len(part) == 0 || (len(value) >= len(part) && stringContains(value, part))
}

func stringContains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
