package gitea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
)

func TestCreateIssueRequestPayload(t *testing.T) {
	var seenPath string
	var seenToken string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		seenToken = r.Header.Get("Authorization")

		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42,"html_url":"http://gitea.local/agent/framework/issues/42"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	issue, err := client.CreateIssue(context.Background(), "agent/framework", codehost.CreateIssueRequest{
		Title:       "upstream failure",
		Description: "created by coordinator",
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	if seenPath != "/api/v1/repos/agent/framework/issues" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenToken != "token test-token" {
		t.Fatalf("expected token auth header, got %q", seenToken)
	}
	if seenPayload["title"] != "upstream failure" {
		t.Fatalf("unexpected title payload: %#v", seenPayload)
	}
	if seenPayload["body"] != "created by coordinator" {
		t.Fatalf("unexpected body payload: %#v", seenPayload)
	}
	if issue.IID != 42 {
		t.Fatalf("expected issue iid 42, got %d", issue.IID)
	}
	if issue.WebURL != "http://gitea.local/agent/framework/issues/42" {
		t.Fatalf("unexpected issue url %s", issue.WebURL)
	}
}

func TestCreateIssueClassifiesAmbiguousAndDeterministicFailures(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		ambiguous bool
		transient bool
	}{
		{name: "server error", status: http.StatusInternalServerError, body: `{"message":"temporary"}`, ambiguous: true},
		{name: "invalid success response", status: http.StatusCreated, body: `{`, ambiguous: true},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"message":"slow down"}`, transient: true},
		{name: "request timeout", status: http.StatusRequestTimeout, body: `{"message":"timeout"}`, transient: true},
		{name: "bad request", status: http.StatusBadRequest, body: `{"message":"invalid"}`, ambiguous: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)
			client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
			_, err := client.CreateIssue(context.Background(), "agent/framework", codehost.CreateIssueRequest{Title: "test"})
			if err == nil {
				t.Fatal("expected request error")
			}
			if got := codehost.IsAmbiguousError(err); got != tc.ambiguous {
				t.Fatalf("expected ambiguous=%v, got %v: %v", tc.ambiguous, got, err)
			}
			if got := codehost.IsTransientRejection(err); got != tc.transient {
				t.Fatalf("expected transient=%v, got %v: %v", tc.transient, got, err)
			}
		})
	}
}

func TestReconcileWritebackFindsGiteaRemoteEffects(t *testing.T) {
	marker := "<!-- sdlc-coordinator-operation:dedupe-1 -->"
	cases := []struct {
		name     string
		request  codehost.ReconcileRequest
		wantPath string
		response string
		wantID   string
	}{
		{
			name:     "pull request",
			request:  codehost.ReconcileRequest{Kind: "create_merge_request", ProjectID: "agent/framework", OperationMarker: marker, SourceBranch: "agent/task-1", TargetBranch: "main"},
			wantPath: "/api/v1/repos/agent/framework/pulls",
			response: `[{"number":7,"html_url":"http://gitea.local/agent/framework/pulls/7","body":"` + marker + `","head":{"ref":"agent/task-1"},"base":{"ref":"main"}}]`,
			wantID:   "7",
		},
		{
			name:     "issue",
			request:  codehost.ReconcileRequest{Kind: "create_issue", ProjectID: "agent/framework", OperationMarker: marker},
			wantPath: "/api/v1/repos/agent/framework/issues",
			response: `[{"number":42,"html_url":"http://gitea.local/agent/framework/issues/42","body":"evidence ` + marker + `"}]`,
			wantID:   "42",
		},
		{
			name:     "pull request comment",
			request:  codehost.ReconcileRequest{Kind: "create_merge_request_note", ProjectID: "agent/framework", TargetIID: 7, OperationMarker: marker},
			wantPath: "/api/v1/repos/agent/framework/issues/7/comments",
			response: `[{"id":202,"body":"review ` + marker + `"}]`,
			wantID:   "202",
		},
		{
			name:     "issue labels",
			request:  codehost.ReconcileRequest{Kind: "set_issue_labels", ProjectID: "agent/framework", TargetIID: 42, Labels: []string{"agent:generated", "agent:pending"}},
			wantPath: "/api/v1/repos/agent/framework/issues/42/labels",
			response: `[{"id":11,"name":"agent:pending"},{"id":12,"name":"agent:generated"}]`,
			wantID:   "42",
		},
		{
			name:     "additive pull request labels with unrelated labels preserved",
			request:  codehost.ReconcileRequest{Kind: "add_merge_request_labels", ProjectID: "agent/framework", TargetIID: 7, Labels: []string{"reviewer-agent"}},
			wantPath: "/api/v1/repos/agent/framework/issues/7/labels",
			response: `[{"id":10,"name":"team-owned"},{"id":11,"name":"reviewer-agent"}]`,
			wantID:   "7",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.EscapedPath() != tc.wantPath {
					t.Fatalf("unexpected reconcile request %s %s", r.Method, r.URL.EscapedPath())
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			t.Cleanup(server.Close)
			client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
			result, err := client.ReconcileWriteback(context.Background(), tc.request)
			if err != nil {
				t.Fatalf("reconcile writeback: %v", err)
			}
			if result.State != codehost.ReconcileStateFound || result.ExternalID != tc.wantID {
				t.Fatalf("unexpected reconcile result: %#v", result)
			}
		})
	}
}

func TestCreateIssueResolvesLabelNames(t *testing.T) {
	var seenPaths []string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.EscapedPath() {
		case "/api/v1/repos/agent/framework/labels":
			_, _ = w.Write([]byte(`[
				{"id":11,"name":"agent:generated"},
				{"id":12,"name":"agent:pending-review"}
			]`))
		case "/api/v1/repos/agent/framework/issues":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"number":42,"html_url":"http://gitea.local/agent/framework/issues/42"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.EscapedPath())
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	_, err := client.CreateIssue(context.Background(), "agent/framework", codehost.CreateIssueRequest{
		Title:       "upstream failure",
		Description: "created by coordinator",
		Labels:      []string{"agent:generated", "agent:pending-review"},
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	if len(seenPaths) != 2 || seenPaths[0] != "/api/v1/repos/agent/framework/labels" || seenPaths[1] != "/api/v1/repos/agent/framework/issues" {
		t.Fatalf("unexpected request paths %#v", seenPaths)
	}
	labels, ok := seenPayload["labels"].([]any)
	if !ok {
		t.Fatalf("expected labels payload, got %#v", seenPayload["labels"])
	}
	if len(labels) != 2 || labels[0].(float64) != 11 || labels[1].(float64) != 12 {
		t.Fatalf("unexpected labels payload %#v", labels)
	}
}

func TestCreateMergeRequestRequestPayload(t *testing.T) {
	var seenPath string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":7,"html_url":"http://gitea.local/agent/framework/pulls/7"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	mr, err := client.CreateMergeRequest(context.Background(), "agent/framework", codehost.CreateMergeRequestRequest{
		Title:        "fix framework validator",
		Description:  "created by coordinator",
		SourceBranch: "agent/fix-validator",
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("create pull request: %v", err)
	}

	if seenPath != "/api/v1/repos/agent/framework/pulls" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["title"] != "fix framework validator" {
		t.Fatalf("unexpected title payload: %#v", seenPayload)
	}
	if seenPayload["body"] != "created by coordinator" {
		t.Fatalf("unexpected body payload: %#v", seenPayload)
	}
	if seenPayload["head"] != "agent/fix-validator" {
		t.Fatalf("unexpected head payload: %#v", seenPayload)
	}
	if seenPayload["base"] != "main" {
		t.Fatalf("unexpected base payload: %#v", seenPayload)
	}
	if mr.IID != 7 {
		t.Fatalf("expected pull request iid 7, got %d", mr.IID)
	}
}

func TestCreateIssueNoteRequestPayload(t *testing.T) {
	var seenPath string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":101,"body":"created upstream fix PR"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	note, err := client.CreateIssueNote(context.Background(), "agent/framework", 42, codehost.CreateNoteRequest{
		Body: "created upstream fix PR",
	})
	if err != nil {
		t.Fatalf("create issue note: %v", err)
	}

	if seenPath != "/api/v1/repos/agent/framework/issues/42/comments" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["body"] != "created upstream fix PR" {
		t.Fatalf("unexpected body payload: %#v", seenPayload)
	}
	if note.ID != 101 {
		t.Fatalf("expected note id 101, got %d", note.ID)
	}
}

func TestCreateMergeRequestNoteUsesIssueCommentEndpoint(t *testing.T) {
	var seenPath string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":202,"body":"review requested"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	note, err := client.CreateMergeRequestNote(context.Background(), "agent/framework", 7, codehost.CreateNoteRequest{
		Body: "review requested",
	})
	if err != nil {
		t.Fatalf("create pull request note: %v", err)
	}

	if seenPath != "/api/v1/repos/agent/framework/issues/7/comments" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["body"] != "review requested" {
		t.Fatalf("unexpected body payload: %#v", seenPayload)
	}
	if note.ID != 202 {
		t.Fatalf("expected note id 202, got %d", note.ID)
	}
}

func TestSetIssueLabelsResolvesLabelNames(t *testing.T) {
	var seenPaths []string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.EscapedPath() {
		case "/api/v1/repos/agent/framework/labels":
			_, _ = w.Write([]byte(`[
				{"id":11,"name":"agent:generated"},
				{"id":12,"name":"agent:pending-review"}
			]`))
		case "/api/v1/repos/agent/framework/issues/42/labels":
			if r.Method != http.MethodPut {
				t.Fatalf("expected PUT, got %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			_, _ = w.Write([]byte(`[{"id":11,"name":"agent:generated"},{"id":12,"name":"agent:pending-review"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.EscapedPath())
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	issue, err := client.SetIssueLabels(context.Background(), "agent/framework", 42, codehost.SetLabelsRequest{
		Labels: []string{"agent:generated", "agent:pending-review"},
	})
	if err != nil {
		t.Fatalf("set issue labels: %v", err)
	}

	if len(seenPaths) != 2 || seenPaths[0] != "/api/v1/repos/agent/framework/labels" || seenPaths[1] != "/api/v1/repos/agent/framework/issues/42/labels" {
		t.Fatalf("unexpected request paths %#v", seenPaths)
	}
	labels, ok := seenPayload["labels"].([]any)
	if !ok {
		t.Fatalf("expected labels payload, got %#v", seenPayload["labels"])
	}
	if len(labels) != 2 || labels[0].(float64) != 11 || labels[1].(float64) != 12 {
		t.Fatalf("unexpected labels payload %#v", labels)
	}
	if issue.IID != 42 {
		t.Fatalf("expected issue iid 42, got %d", issue.IID)
	}
}

func TestSetMergeRequestLabelsUsesIssueLabelsEndpoint(t *testing.T) {
	var seenPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.EscapedPath() {
		case "/api/v1/repos/agent/framework/labels":
			_, _ = w.Write([]byte(`[{"id":11,"name":"agent:generated"}]`))
		case "/api/v1/repos/agent/framework/issues/7/labels":
			seenPath = r.URL.EscapedPath()
			_, _ = w.Write([]byte(`[{"id":11,"name":"agent:generated"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.EscapedPath())
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	mr, err := client.SetMergeRequestLabels(context.Background(), "agent/framework", 7, codehost.SetLabelsRequest{
		Labels: []string{"agent:generated"},
	})
	if err != nil {
		t.Fatalf("set pull request labels: %v", err)
	}

	if seenPath != "/api/v1/repos/agent/framework/issues/7/labels" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if mr.IID != 7 {
		t.Fatalf("expected mr iid 7, got %d", mr.IID)
	}
}

func TestAddMergeRequestLabelsUsesAdditiveIssueLabelsEndpoint(t *testing.T) {
	var seenMethod string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case "/api/v1/repos/agent/framework/labels":
			_, _ = w.Write([]byte(`[{"id":11,"name":"reviewer-agent"}]`))
		case "/api/v1/repos/agent/framework/issues/7/labels":
			seenMethod = r.Method
			if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			_, _ = w.Write([]byte(`[{"id":11,"name":"reviewer-agent"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.EscapedPath())
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	mr, err := client.AddMergeRequestLabels(context.Background(), "agent/framework", 7, codehost.SetLabelsRequest{
		Labels: []string{"reviewer-agent"},
	})
	if err != nil {
		t.Fatalf("add pull request labels: %v", err)
	}
	if seenMethod != http.MethodPost {
		t.Fatalf("expected additive POST, got %s", seenMethod)
	}
	labels, ok := seenPayload["labels"].([]any)
	if !ok || len(labels) != 1 || labels[0].(float64) != 11 {
		t.Fatalf("unexpected labels payload %#v", seenPayload["labels"])
	}
	if mr.IID != 7 {
		t.Fatalf("expected mr iid 7, got %d", mr.IID)
	}
}

func TestSetIssueLabelsRejectsUnknownLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":11,"name":"agent:generated"}]`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	_, err := client.SetIssueLabels(context.Background(), "agent/framework", 42, codehost.SetLabelsRequest{
		Labels: []string{"missing-label"},
	})
	if err == nil {
		t.Fatal("expected unknown label error")
	}
	if got := err.Error(); got != `gitea label "missing-label" not found in agent/framework` {
		t.Fatalf("unexpected error %q", got)
	}
}

// Gitea 列表接口按页返回(服务器把 limit 截到自身上限);标签解析必须翻页,
// 否则大标签库下会误报 label not found。
func TestSetMergeRequestLabelsResolvesLabelsAcrossPages(t *testing.T) {
	var seenPages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/repos/agent/framework/issues/9/labels":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":77,"name":"review-needed"}]`))
		default:
			seenPages = append(seenPages, r.URL.Query().Get("page"))
			w.WriteHeader(http.StatusOK)
			if r.URL.Query().Get("page") == "2" {
				_, _ = w.Write([]byte(`[{"id":77,"name":"review-needed"}]`))
				return
			}
			labels := make([]map[string]any, 0, 50)
			for i := 0; i < 50; i++ {
				labels = append(labels, map[string]any{"id": 100 + i, "name": fmt.Sprintf("bulk-%d", i)})
			}
			_ = json.NewEncoder(w).Encode(labels)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	_, err := client.SetMergeRequestLabels(context.Background(), "agent/framework", 9, codehost.SetLabelsRequest{Labels: []string{"review-needed"}})
	if err != nil {
		t.Fatalf("set labels across pages: %v", err)
	}
	if len(seenPages) < 2 {
		t.Fatalf("expected label listing to paginate, got pages %q", seenPages)
	}
}

// reconcile 只扫第一页会漏掉老仓库里的 MR/评论;翻页直到短页。
func TestReconcileWritebackFindsMergeRequestAcrossPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/agent/framework/pulls" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"number":7,"html_url":"http://gitea.local/agent/framework/pulls/7","body":"<!-- m -->","head":{"ref":"feature/x"},"base":{"ref":"main"}}]`))
			return
		}
		pulls := make([]map[string]any, 0, 50)
		for i := 0; i < 50; i++ {
			pulls = append(pulls, map[string]any{
				"number": 100 + i, "html_url": "http://gitea.local/agent/framework/pulls/x",
				"body": "unrelated", "head": map[string]any{"ref": "other"}, "base": map[string]any{"ref": "main"},
			})
		}
		_ = json.NewEncoder(w).Encode(pulls)
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

func TestReconcileWritebackFindsNoteAcrossPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"id":9,"body":"contains <!-- m --> marker"}]`))
			return
		}
		notes := make([]map[string]any, 0, 50)
		for i := 0; i < 50; i++ {
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

func TestGetMergeRequestMapsPullBranches(t *testing.T) {
	var seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"number": 23,
			"html_url": "https://gitea.example/octo-org/external/pulls/23",
			"head": {"name": "legacy-head", "ref": "feature/external-pr", "sha": "cccccccccccccccccccccccccccccccccccccccc"},
			"base": {"name": "main"},
			"merged": true,
			"merge_commit_sha": "merged123"
		}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	mergeRequest, err := client.GetMergeRequest(context.Background(), "octo-org/external", 23)
	if err != nil {
		t.Fatalf("get merge request: %v", err)
	}
	if seenPath != "/api/v1/repos/octo-org/external/pulls/23" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if mergeRequest.IID != 23 || mergeRequest.WebURL != "https://gitea.example/octo-org/external/pulls/23" ||
		mergeRequest.SourceBranch != "feature/external-pr" || mergeRequest.TargetBranch != "main" ||
		mergeRequest.HeadSHA != "cccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("unexpected merge request: %#v", mergeRequest)
	}
	if !mergeRequest.Merged || mergeRequest.MergeCommitSHA != "merged123" {
		t.Fatalf("expected merge truth merged=true sha=merged123, got %#v", mergeRequest)
	}
}

func TestClientMergePullRequestSendsHeadAssertionAndReadsBackTruth(t *testing.T) {
	var seenPath string
	var seenPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			seenPath = r.URL.EscapedPath()
			if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
				t.Errorf("decode merge payload: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			// Gitea 合并成功响应是空 200。
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(`{
			"number": 12,
			"html_url": "http://gitea.local/group/project/pulls/12",
			"head": {"ref": "feature/x", "sha": "abc123"},
			"base": {"ref": "main"},
			"merged": true,
			"merge_commit_sha": "merged999"
		}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	result, err := client.MergePullRequest(context.Background(), "group/project", 12, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: "abc123",
		MergeMethod:     "squash",
		CommitTitle:     "squash title",
		CommitMessage:   "squash body",
	})
	if err != nil {
		t.Fatalf("merge pull request: %v", err)
	}
	if seenPath != "/api/v1/repos/group/project/pulls/12/merge" {
		t.Fatalf("unexpected merge path %s", seenPath)
	}
	if seenPayload["do"] != "squash" {
		t.Fatalf("unexpected merge method: %#v", seenPayload)
	}
	if seenPayload["head_commit_id"] != "abc123" {
		t.Fatalf("unexpected head assertion: %#v", seenPayload)
	}
	if seenPayload["delete_branch_after_merge"] != false {
		t.Fatalf("delete_branch_after_merge must be explicit false: %#v", seenPayload)
	}
	if seenPayload["merge_title_field"] != "squash title" || seenPayload["merge_message_field"] != "squash body" {
		t.Fatalf("unexpected merge commit fields: %#v", seenPayload)
	}
	if !result.Merged || result.MergedCommitSHA != "merged999" {
		t.Fatalf("expected merged result from readback, got %#v", result)
	}
}

func TestClientMergePullRequestDefaultsToGiteaMerge(t *testing.T) {
	var seenPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
				t.Errorf("decode merge payload: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(`{"number":12,"merged":true,"merge_commit_sha":"merged777"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	if _, err := client.MergePullRequest(context.Background(), "group/project", 12, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: "abc123",
	}); err != nil {
		t.Fatalf("merge pull request with default method: %v", err)
	}
	if seenPayload["do"] != "merge" {
		t.Fatalf("expected gitea required default do=merge, got %#v", seenPayload)
	}
	if _, ok := seenPayload["force_merge"]; ok {
		t.Fatalf("force_merge must never be sent, got %#v", seenPayload)
	}
	if _, ok := seenPayload["merge_when_checks_succeed"]; ok {
		t.Fatalf("merge_when_checks_succeed must never be sent, got %#v", seenPayload)
	}
}

func TestClientMergeErrorClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		headMismatch bool
		rejected     bool
	}{
		{name: "conflict is head mismatch", status: http.StatusConflict, headMismatch: true},
		{name: "forbidden is platform rejection", status: http.StatusForbidden, rejected: true},
		{name: "not found is platform rejection", status: http.StatusNotFound, rejected: true},
		{name: "method not allowed is platform rejection", status: http.StatusMethodNotAllowed, rejected: true},
		{name: "unprocessable is platform rejection", status: http.StatusUnprocessableEntity, rejected: true},
		{name: "locked is platform rejection", status: http.StatusLocked, rejected: true},
		{name: "request timeout stays transient", status: http.StatusRequestTimeout},
		{name: "rate limited stays transient", status: http.StatusTooManyRequests},
		{name: "server error stays ambiguous", status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)
			client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
			_, err := client.MergePullRequest(context.Background(), "agent/framework", 12, codehost.MergePullRequestRequest{
				ExpectedHeadSHA: "abc123",
			})
			if err == nil {
				t.Fatal("expected merge error")
			}
			if got := errors.Is(err, codehost.ErrMergeHeadMismatch); got != tc.headMismatch {
				t.Fatalf("expected headMismatch=%v, got %v: %v", tc.headMismatch, got, err)
			}
			if got := errors.Is(err, codehost.ErrMergeRejected); got != tc.rejected {
				t.Fatalf("expected rejected=%v, got %v: %v", tc.rejected, got, err)
			}
			if (tc.status == http.StatusRequestTimeout || tc.status == http.StatusTooManyRequests) && !codehost.IsTransientRejection(err) {
				t.Fatalf("expected transient rejection, got %v", err)
			}
			if tc.status >= http.StatusInternalServerError && !codehost.IsAmbiguousError(err) {
				t.Fatalf("expected ambiguous error, got %v", err)
			}
		})
	}
}

func TestClientGetMergeRequestErrorClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		headMismatch bool
		rejected     bool
	}{
		{name: "conflict is head mismatch", status: http.StatusConflict, headMismatch: true},
		{name: "not found is platform rejection", status: http.StatusNotFound, rejected: true},
		{name: "forbidden is platform rejection", status: http.StatusForbidden, rejected: true},
		{name: "rate limited stays transient", status: http.StatusTooManyRequests},
		{name: "server error stays ambiguous", status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)
			client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
			_, err := client.GetMergeRequest(context.Background(), "agent/framework", 12)
			if err == nil {
				t.Fatal("expected read error")
			}
			if got := errors.Is(err, codehost.ErrMergeHeadMismatch); got != tc.headMismatch {
				t.Fatalf("expected headMismatch=%v, got %v: %v", tc.headMismatch, got, err)
			}
			if got := errors.Is(err, codehost.ErrMergeRejected); got != tc.rejected {
				t.Fatalf("expected rejected=%v, got %v: %v", tc.rejected, got, err)
			}
			if tc.status == http.StatusTooManyRequests && !codehost.IsTransientRejection(err) {
				t.Fatalf("expected transient rejection, got %v", err)
			}
			if tc.status >= http.StatusInternalServerError && !codehost.IsAmbiguousError(err) {
				t.Fatalf("expected ambiguous error, got %v", err)
			}
		})
	}
}

func TestClientMergePullRequestInvalidRepoDoesNotReachNetwork(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	_, err := client.MergePullRequest(context.Background(), "not-a-repo", 12, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: "abc123",
	})
	if err == nil {
		t.Fatal("expected invalid repo error")
	}
	if requests != 0 {
		t.Fatalf("invalid repo path must not reach the network, got %d requests", requests)
	}
}
