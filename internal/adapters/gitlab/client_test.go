package gitlab

import (
	"context"
	"encoding/json"
	"errors"
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
		seenToken = r.Header.Get("PRIVATE-TOKEN")

		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"iid":42,"web_url":"https://gitlab.example/group/test-repo/-/issues/42"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	issue, err := client.CreateIssue(context.Background(), "group/test-repo", codehost.CreateIssueRequest{
		Title:       "upstream failure",
		Description: "coordinator discovered an upstream dependency issue",
		Labels:      []string{"agent:pending", "repo:framework"},
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	if seenPath != "/api/v4/projects/group%2Ftest-repo/issues" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenToken != "test-token" {
		t.Fatalf("expected private token header, got %q", seenToken)
	}
	if seenPayload["title"] != "upstream failure" {
		t.Fatalf("unexpected title payload: %#v", seenPayload)
	}
	if seenPayload["labels"] != "agent:pending,repo:framework" {
		t.Fatalf("unexpected labels payload: %#v", seenPayload["labels"])
	}
	if issue.IID != 42 {
		t.Fatalf("expected issue iid 42, got %d", issue.IID)
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
			_, err := client.CreateIssue(context.Background(), "group/test-repo", codehost.CreateIssueRequest{Title: "test"})
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

func TestClientReturnsTypedStatusErrorForClientFailures(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		wantTyped bool
		ambiguous bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantTyped: true},
		{name: "forbidden", status: http.StatusForbidden, wantTyped: true},
		{name: "not found", status: http.StatusNotFound, wantTyped: true},
		{name: "server error", status: http.StatusInternalServerError, wantTyped: false, ambiguous: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"message":"denied"}`))
			}))
			t.Cleanup(server.Close)
			client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
			_, err := client.CreateIssue(context.Background(), "group/test-repo", codehost.CreateIssueRequest{Title: "test"})
			if err == nil {
				t.Fatal("expected request error")
			}
			var statusErr *StatusError
			gotTyped := errors.As(err, &statusErr)
			if gotTyped != tc.wantTyped {
				t.Fatalf("typed=%v want %v err=%v", gotTyped, tc.wantTyped, err)
			}
			if tc.wantTyped && statusErr.StatusCode != tc.status {
				t.Fatalf("status=%d want %d", statusErr.StatusCode, tc.status)
			}
			if codehost.IsAmbiguousError(err) != tc.ambiguous {
				t.Fatalf("ambiguous=%v want %v: %v", codehost.IsAmbiguousError(err), tc.ambiguous, err)
			}
		})
	}
}

func TestReconcileWritebackFindsGitLabRemoteEffects(t *testing.T) {
	marker := "<!-- sdlc-coordinator-operation:dedupe-1 -->"
	cases := []struct {
		name     string
		request  codehost.ReconcileRequest
		wantPath string
		response string
		wantID   string
	}{
		{
			name:     "merge request",
			request:  codehost.ReconcileRequest{Kind: "create_merge_request", ProjectID: "group/framework", OperationMarker: marker, SourceBranch: "agent/task-1", TargetBranch: "main"},
			wantPath: "/api/v4/projects/group%2Fframework/merge_requests",
			response: `[{"iid":7,"web_url":"https://gitlab.example/mr/7","description":"` + marker + `","source_branch":"agent/task-1","target_branch":"main"}]`,
			wantID:   "7",
		},
		{
			name:     "issue",
			request:  codehost.ReconcileRequest{Kind: "create_issue", ProjectID: "group/framework", OperationMarker: marker},
			wantPath: "/api/v4/projects/group%2Fframework/issues",
			response: `[{"iid":42,"web_url":"https://gitlab.example/issues/42","description":"evidence ` + marker + `"}]`,
			wantID:   "42",
		},
		{
			name:     "merge request note",
			request:  codehost.ReconcileRequest{Kind: "create_merge_request_note", ProjectID: "group/framework", TargetIID: 7, OperationMarker: marker},
			wantPath: "/api/v4/projects/group%2Fframework/merge_requests/7/notes",
			response: `[{"id":202,"body":"review ` + marker + `"}]`,
			wantID:   "202",
		},
		{
			name:     "issue labels",
			request:  codehost.ReconcileRequest{Kind: "set_issue_labels", ProjectID: "group/framework", TargetIID: 42, Labels: []string{"agent:generated", "agent:pending"}},
			wantPath: "/api/v4/projects/group%2Fframework/issues/42",
			response: `{"iid":42,"web_url":"https://gitlab.example/issues/42","labels":["agent:pending","agent:generated"]}`,
			wantID:   "42",
		},
		{
			name:     "additive merge request labels with unrelated labels preserved",
			request:  codehost.ReconcileRequest{Kind: "add_merge_request_labels", ProjectID: "group/framework", TargetIID: 7, Labels: []string{"reviewer-agent"}},
			wantPath: "/api/v4/projects/group%2Fframework/merge_requests/7",
			response: `{"iid":7,"web_url":"https://gitlab.example/mr/7","labels":["team-owned","reviewer-agent"]}`,
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
		_, _ = w.Write([]byte(`{"iid":7,"web_url":"https://gitlab.example/group/framework/-/merge_requests/7"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	mr, err := client.CreateMergeRequest(context.Background(), "group/framework", CreateMergeRequestRequest{
		Title:              "fix framework validator",
		Description:        "created by coordinator",
		SourceBranch:       "agent/fix-validator",
		TargetBranch:       "main",
		RemoveSourceBranch: true,
		Labels:             []string{"agent:generated", "agent:pending-review"},
	})
	if err != nil {
		t.Fatalf("create merge request: %v", err)
	}

	if seenPath != "/api/v4/projects/group%2Fframework/merge_requests" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["title"] != "fix framework validator" {
		t.Fatalf("unexpected title payload: %#v", seenPayload)
	}
	if seenPayload["source_branch"] != "agent/fix-validator" {
		t.Fatalf("unexpected source branch payload: %#v", seenPayload)
	}
	if seenPayload["target_branch"] != "main" {
		t.Fatalf("unexpected target branch payload: %#v", seenPayload)
	}
	if seenPayload["remove_source_branch"] != true {
		t.Fatalf("unexpected remove source branch payload: %#v", seenPayload)
	}
	if seenPayload["labels"] != "agent:generated,agent:pending-review" {
		t.Fatalf("unexpected labels payload: %#v", seenPayload["labels"])
	}
	if mr.IID != 7 {
		t.Fatalf("expected mr iid 7, got %d", mr.IID)
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
		_, _ = w.Write([]byte(`{"id":101,"body":"created upstream fix MR"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	note, err := client.CreateIssueNote(context.Background(), "group/test-automates", 42, CreateNoteRequest{
		Body: "created upstream fix MR",
	})
	if err != nil {
		t.Fatalf("create issue note: %v", err)
	}

	if seenPath != "/api/v4/projects/group%2Ftest-automates/issues/42/notes" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["body"] != "created upstream fix MR" {
		t.Fatalf("unexpected body payload: %#v", seenPayload)
	}
	if note.ID != 101 {
		t.Fatalf("expected note id 101, got %d", note.ID)
	}
}

func TestCreateMergeRequestNoteRequestPayload(t *testing.T) {
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

	note, err := client.CreateMergeRequestNote(context.Background(), "group/framework", 7, CreateNoteRequest{
		Body: "review requested",
	})
	if err != nil {
		t.Fatalf("create merge request note: %v", err)
	}

	if seenPath != "/api/v4/projects/group%2Fframework/merge_requests/7/notes" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["body"] != "review requested" {
		t.Fatalf("unexpected body payload: %#v", seenPayload)
	}
	if note.ID != 202 {
		t.Fatalf("expected note id 202, got %d", note.ID)
	}
}

func TestSetIssueLabelsRequestPayload(t *testing.T) {
	var seenPath string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"iid":42,"web_url":"https://gitlab.example/group/test-automates/-/issues/42"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	issue, err := client.SetIssueLabels(context.Background(), "group/test-automates", 42, SetLabelsRequest{
		Labels: []string{"agent:waiting-upstream", "agent:generated"},
	})
	if err != nil {
		t.Fatalf("set issue labels: %v", err)
	}

	if seenPath != "/api/v4/projects/group%2Ftest-automates/issues/42" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["labels"] != "agent:waiting-upstream,agent:generated" {
		t.Fatalf("unexpected labels payload: %#v", seenPayload["labels"])
	}
	if issue.IID != 42 {
		t.Fatalf("expected issue iid 42, got %d", issue.IID)
	}
}

func TestSetMergeRequestLabelsRequestPayload(t *testing.T) {
	var seenPath string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"iid":7,"web_url":"https://gitlab.example/group/framework/-/merge_requests/7"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))

	mr, err := client.SetMergeRequestLabels(context.Background(), "group/framework", 7, SetLabelsRequest{
		Labels: []string{"agent:pending-review", "agent:generated"},
	})
	if err != nil {
		t.Fatalf("set merge request labels: %v", err)
	}

	if seenPath != "/api/v4/projects/group%2Fframework/merge_requests/7" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["labels"] != "agent:pending-review,agent:generated" {
		t.Fatalf("unexpected labels payload: %#v", seenPayload["labels"])
	}
	if mr.IID != 7 {
		t.Fatalf("expected mr iid 7, got %d", mr.IID)
	}
}

func TestAddMergeRequestLabelsUsesAdditiveGitLabPayload(t *testing.T) {
	var seenPath string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"iid":7,"web_url":"https://gitlab.example/group/framework/-/merge_requests/7"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	mr, err := client.AddMergeRequestLabels(context.Background(), "group/framework", 7, SetLabelsRequest{
		Labels: []string{"reviewer-agent"},
	})
	if err != nil {
		t.Fatalf("add merge request labels: %v", err)
	}

	if seenPath != "/api/v4/projects/group%2Fframework/merge_requests/7" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["add_labels"] != "reviewer-agent" {
		t.Fatalf("unexpected add_labels payload: %#v", seenPayload)
	}
	if mr.IID != 7 {
		t.Fatalf("expected mr iid 7, got %d", mr.IID)
	}
}

func TestGetProjectUsesPrivateTokenAndPathEscape(t *testing.T) {
	var seenPath string
	var seenToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		seenToken = r.Header.Get("PRIVATE-TOKEN")
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":24207,"path_with_namespace":"group/coordinator","default_branch":"master"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "repo-token", WithHTTPClient(server.Client()))
	project, err := client.GetProject(context.Background(), "group/coordinator")
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if seenPath != "/api/v4/projects/group%2Fcoordinator" {
		t.Fatalf("path=%s", seenPath)
	}
	if seenToken != "repo-token" {
		t.Fatalf("token=%q", seenToken)
	}
	if project.ID != 24207 || project.PathWithNamespace != "group/coordinator" || project.DefaultBranch != "master" {
		t.Fatalf("project=%+v", project)
	}
}

func TestGetMergeRequestDecodesBranches(t *testing.T) {
	var seenPath string
	var seenToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		seenToken = r.Header.Get("PRIVATE-TOKEN")
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"iid": 21,
			"web_url": "https://gitlab.example/demo-group/demo-project/-/merge_requests/21",
			"source_branch": "feature/external",
			"target_branch": "main",
			"sha": "cccccccccccccccccccccccccccccccccccccccc"
		}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	mergeRequest, err := client.GetMergeRequest(context.Background(), "demo-group/demo-project", 21)
	if err != nil {
		t.Fatalf("get merge request: %v", err)
	}
	if seenPath != "/api/v4/projects/demo-group%2Fdemo-project/merge_requests/21" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenToken != "test-token" {
		t.Fatalf("expected private token header, got %q", seenToken)
	}
	if mergeRequest.IID != 21 || mergeRequest.SourceBranch != "feature/external" || mergeRequest.TargetBranch != "main" ||
		mergeRequest.HeadSHA != "cccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("unexpected merge request: %#v", mergeRequest)
	}
}
