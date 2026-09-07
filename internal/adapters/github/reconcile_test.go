package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
)

func pullPayloads(start int, count int, head, base, body string) []map[string]any {
	pulls := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		pulls = append(pulls, map[string]any{
			"number": start + i, "html_url": fmt.Sprintf("https://github.com/agent/framework/pull/%d", start+i),
			"body": body,
			"head": map[string]any{"ref": head, "sha": "sha-" + fmt.Sprint(start+i)},
			"base": map[string]any{"ref": base},
		})
	}
	return pulls
}

func TestReconcileWritebackFindsMergeRequestAcrossPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/agent/framework/pulls" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode(pullPayloads(7, 1, "feature/x", "main", "work <!-- m --> done"))
			return
		}
		_ = json.NewEncoder(w).Encode(pullPayloads(100, 100, "other", "main", "unrelated"))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	result, err := client.ReconcileWriteback(context.Background(), codehost.ReconcileRequest{
		Kind: "create_merge_request", ProjectID: "agent/framework",
		SourceBranch: "feature/x", TargetBranch: "main", OperationMarker: "<!-- m -->",
	})
	if err != nil {
		t.Fatalf("reconcile across pages: %v", err)
	}
	if result.State != codehost.ReconcileStateFound || result.ExternalID != "7" {
		t.Fatalf("expected found on page 2, got %#v", result)
	}
	if requests != 2 {
		t.Fatalf("expected exactly two pages scanned, got %d", requests)
	}
}

func TestReconcileWritebackFindsIssueAcrossPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"number":9,"html_url":"https://github.com/agent/framework/issues/9","body":"contains <!-- m --> marker"}]`))
			return
		}
		issues := make([]map[string]any, 0, 100)
		for i := 0; i < 100; i++ {
			issues = append(issues, map[string]any{"number": 100 + i, "html_url": "x", "body": "unrelated"})
		}
		_ = json.NewEncoder(w).Encode(issues)
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	result, err := client.ReconcileWriteback(context.Background(), codehost.ReconcileRequest{
		Kind: "create_issue", ProjectID: "agent/framework", OperationMarker: "<!-- m -->",
	})
	if err != nil {
		t.Fatalf("reconcile issue across pages: %v", err)
	}
	if result.State != codehost.ReconcileStateFound || result.ExternalID != "9" {
		t.Fatalf("expected issue found on page 2, got %#v", result)
	}
	if requests != 2 {
		t.Fatalf("expected exactly two pages scanned, got %d", requests)
	}
}

func TestReconcileWritebackFindsNoteAcrossPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/repos/agent/framework/issues/7/comments" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"id":9,"body":"contains <!-- m --> marker"}]`))
			return
		}
		notes := make([]map[string]any, 0, 100)
		for i := 0; i < 100; i++ {
			notes = append(notes, map[string]any{"id": 100 + i, "body": "unrelated"})
		}
		_ = json.NewEncoder(w).Encode(notes)
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	result, err := client.ReconcileWriteback(context.Background(), codehost.ReconcileRequest{
		Kind: "create_merge_request_note", ProjectID: "agent/framework", TargetIID: 7, OperationMarker: "<!-- m -->",
	})
	if err != nil {
		t.Fatalf("reconcile note across pages: %v", err)
	}
	if result.State != codehost.ReconcileStateFound || result.ExternalID != "9" {
		t.Fatalf("expected note found on page 2, got %#v", result)
	}
	if requests != 2 {
		t.Fatalf("expected exactly two pages scanned, got %d", requests)
	}
}

func TestReconcileWritebackMatchesLabels(t *testing.T) {
	cases := []struct {
		kind    string
		labels  []string
		matches bool
	}{
		// set 是精确匹配:远端当前标签集与请求集完全一致才算已生效。
		{kind: "set_issue_labels", labels: []string{"developer-agent", "review"}, matches: true},
		{kind: "set_issue_labels", labels: []string{"developer-agent"}, matches: false},
		{kind: "add_merge_request_labels", labels: []string{"review"}, matches: true},
		{kind: "add_merge_request_labels", labels: []string{"missing"}, matches: false},
	}
	for _, tc := range cases {
		t.Run(tc.kind+"/"+fmt.Sprint(tc.matches), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[{"name":"developer-agent"},{"name":"review"}]`))
			}))
			t.Cleanup(server.Close)

			client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
			result, err := client.ReconcileWriteback(context.Background(), codehost.ReconcileRequest{
				Kind: tc.kind, ProjectID: "agent/framework", TargetIID: 7, Labels: tc.labels,
			})
			if err != nil {
				t.Fatalf("reconcile labels: %v", err)
			}
			if tc.matches && result.State != codehost.ReconcileStateFound {
				t.Fatalf("expected found, got %#v", result)
			}
			if !tc.matches && result.State != codehost.ReconcileStateAbsent {
				t.Fatalf("expected absent, got %#v", result)
			}
		})
	}
}
