package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
)

func TestGetIssueStateReportsClosed(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"closed"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token-1")
	state, err := client.GetIssueState(context.Background(), "group/demo", 7)
	if err != nil {
		t.Fatalf("get issue state: %v", err)
	}
	if !state.Closed {
		t.Fatal("closed issue must be reported closed")
	}
	if gotPath != "/api/v4/projects/group%2Fdemo/issues/7" {
		t.Fatalf("unexpected path %q", gotPath)
	}
}

func TestGetIssueStateReportsOpenAndPropagatesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/api/v4/projects/group%2Fdemo/issues/7" {
			_, _ = w.Write([]byte(`{"state":"opened"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token-1")
	state, err := client.GetIssueState(context.Background(), "group/demo", 7)
	if err != nil {
		t.Fatalf("get issue state: %v", err)
	}
	if state.Closed {
		t.Fatal("opened issue must not be reported closed")
	}
	if _, err := client.GetIssueState(context.Background(), "group/demo", 404); err == nil {
		t.Fatal("http error must propagate")
	}
}

var _ codehost.IssueStateReader = (*Client)(nil)
