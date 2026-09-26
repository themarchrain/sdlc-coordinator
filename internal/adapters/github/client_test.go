package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
)

func TestCreateIssueRequestPayload(t *testing.T) {
	var seenPath string
	var seenAuth string
	var seenAccept string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		seenAuth = r.Header.Get("Authorization")
		seenAccept = r.Header.Get("Accept")
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42,"html_url":"https://github.com/agent/framework/issues/42"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	issue, err := client.CreateIssue(context.Background(), "agent/framework", codehost.CreateIssueRequest{
		Title:       "upstream failure",
		Description: "created by coordinator",
		Labels:      []string{"developer-agent"},
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if seenPath != "/repos/agent/framework/issues" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenAuth != "Bearer test-token" {
		t.Fatalf("expected bearer auth header, got %q", seenAuth)
	}
	if !strings.Contains(seenAccept, "application/vnd.github+json") {
		t.Fatalf("expected github accept header, got %q", seenAccept)
	}
	if seenPayload["title"] != "upstream failure" || seenPayload["body"] != "created by coordinator" {
		t.Fatalf("unexpected payload: %#v", seenPayload)
	}
	labels, ok := seenPayload["labels"].([]any)
	if !ok || len(labels) != 1 || labels[0] != "developer-agent" {
		t.Fatalf("expected label names array, got %#v", seenPayload["labels"])
	}
	if issue.IID != 42 || issue.WebURL != "https://github.com/agent/framework/issues/42" {
		t.Fatalf("unexpected issue: %#v", issue)
	}
}

func TestCreateMergeRequestRequestPayload(t *testing.T) {
	var seenPath string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		if err := json.NewDecoder(r.Body).Decode(&seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":7,"html_url":"https://github.com/agent/framework/pull/7"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	mr, err := client.CreateMergeRequest(context.Background(), "agent/framework", codehost.CreateMergeRequestRequest{
		Title: "Fix widget", Description: "fixes #1", SourceBranch: "fix/widget", TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("create pull request: %v", err)
	}
	if seenPath != "/repos/agent/framework/pulls" {
		t.Fatalf("unexpected path %s", seenPath)
	}
	if seenPayload["head"] != "fix/widget" || seenPayload["base"] != "main" || seenPayload["title"] != "Fix widget" {
		t.Fatalf("unexpected payload: %#v", seenPayload)
	}
	if mr.IID != 7 {
		t.Fatalf("unexpected mr: %#v", mr)
	}
}

func TestNotesUseIssueCommentsEndpoint(t *testing.T) {
	var seenPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":9001,"body":"note body"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	note, err := client.CreateMergeRequestNote(context.Background(), "agent/framework", 7, codehost.CreateNoteRequest{Body: "review note"})
	if err != nil {
		t.Fatalf("create mr note: %v", err)
	}
	if _, err := client.CreateIssueNote(context.Background(), "agent/framework", 9, codehost.CreateNoteRequest{Body: "issue note"}); err != nil {
		t.Fatalf("create issue note: %v", err)
	}
	if note.ID != 9001 {
		t.Fatalf("unexpected note: %#v", note)
	}
	if len(seenPaths) != 2 || seenPaths[0] != "POST /repos/agent/framework/issues/7/comments" || seenPaths[1] != "POST /repos/agent/framework/issues/9/comments" {
		t.Fatalf("pull request comments must use the issues comments api, got %q", seenPaths)
	}
}

func TestSetAndAddLabelsUsePlainNames(t *testing.T) {
	var seen []struct {
		method  string
		path    string
		payload map[string]any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry := struct {
			method  string
			path    string
			payload map[string]any
		}{method: r.Method, path: r.URL.EscapedPath()}
		_ = json.NewDecoder(r.Body).Decode(&entry.payload)
		seen = append(seen, entry)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"name":"developer-agent"}]`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	if _, err := client.SetMergeRequestLabels(context.Background(), "agent/framework", 7, codehost.SetLabelsRequest{Labels: []string{"developer-agent"}}); err != nil {
		t.Fatalf("set labels: %v", err)
	}
	if _, err := client.AddMergeRequestLabels(context.Background(), "agent/framework", 7, codehost.SetLabelsRequest{Labels: []string{"rework"}}); err != nil {
		t.Fatalf("add labels: %v", err)
	}
	if seen[0].method != http.MethodPut || seen[0].path != "/repos/agent/framework/issues/7/labels" {
		t.Fatalf("expected replace via PUT, got %#v", seen[0])
	}
	if names, ok := seen[0].payload["labels"].([]any); !ok || len(names) != 1 || names[0] != "developer-agent" {
		t.Fatalf("expected plain label names, got %#v", seen[0].payload)
	}
	if seen[1].method != http.MethodPost || seen[1].path != "/repos/agent/framework/issues/7/labels" {
		t.Fatalf("expected append via POST, got %#v", seen[1])
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
			if !tc.ambiguous && StatusCode(err) != tc.status {
				t.Fatalf("expected status %d, got %d", tc.status, StatusCode(err))
			}
		})
	}
}

func TestGetMergeRequestMapsPullBranches(t *testing.T) {
	var seenMethod, seenPath, seenAPIVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.EscapedPath()
		seenAPIVersion = r.Header.Get("X-GitHub-Api-Version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"number": 17,
			"html_url": "https://github.com/octo-org/external/pull/17",
			"head": {"ref": "feature/external-pr", "sha": "cccccccccccccccccccccccccccccccccccccccc"},
			"base": {"ref": "main", "sha": "dddddddddddddddddddddddddddddddddddddddd"}
		}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	mergeRequest, err := client.GetMergeRequest(context.Background(), "octo-org/external", 17)
	if err != nil {
		t.Fatalf("get merge request: %v", err)
	}
	if seenMethod != http.MethodGet || seenPath != "/repos/octo-org/external/pulls/17" {
		t.Fatalf("unexpected request %s %s", seenMethod, seenPath)
	}
	if seenAPIVersion != "2022-11-28" {
		t.Fatalf("expected api version header, got %q", seenAPIVersion)
	}
	if mergeRequest.IID != 17 || mergeRequest.WebURL != "https://github.com/octo-org/external/pull/17" ||
		mergeRequest.SourceBranch != "feature/external-pr" || mergeRequest.TargetBranch != "main" ||
		mergeRequest.HeadSHA != "cccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("unexpected merge request: %#v", mergeRequest)
	}
}

// github.com 仓库在注册行里配网页域,REST 客户端构造时派生 API 域。
func TestNewClientDerivesAPIHost(t *testing.T) {
	client := NewClient("https://github.com", "test-token")
	if client.baseURL != "https://api.github.com" {
		t.Fatalf("baseURL = %q, want https://api.github.com", client.baseURL)
	}
}

func TestMergePullRequestSendsHeadAssertionAndReturnsCommit(t *testing.T) {
	var seenPath string
	var seenMethod string
	var seenPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.EscapedPath()
		_ = json.NewDecoder(r.Body).Decode(&seenPayload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"sha":"dddd0444d","merged":true,"message":"Pull Request successfully merged"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	result, err := client.MergePullRequest(context.Background(), "agent/framework", 7, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: "cccccccccccccccccccccccccccccccccccccccc",
		MergeMethod:     "squash",
		CommitTitle:     "feat: merge verification",
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if seenMethod != http.MethodPut || seenPath != "/repos/agent/framework/pulls/7/merge" {
		t.Fatalf("unexpected call: %s %s", seenMethod, seenPath)
	}
	if seenPayload["sha"] != "cccccccccccccccccccccccccccccccccccccccc" || seenPayload["merge_method"] != "squash" || seenPayload["commit_title"] != "feat: merge verification" {
		t.Fatalf("head assertion or merge options missing: %#v", seenPayload)
	}
	if !result.Merged || result.MergedCommitSHA != "dddd0444d" {
		t.Fatalf("unexpected merge result: %#v", result)
	}
}

func TestMergePullRequestHeadMismatchMapsToSentinel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"Head branch was modified out of the base branch"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	_, err := client.MergePullRequest(context.Background(), "agent/framework", 7, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: "cccccccccccccccccccccccccccccccccccccccc",
	})
	if !errors.Is(err, codehost.ErrMergeHeadMismatch) {
		t.Fatalf("head mismatch must map to the sentinel, got %v", err)
	}
}

func TestMergePullRequestProtectionRejectionMapsToSentinel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Required status checks have not passed"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	_, err := client.MergePullRequest(context.Background(), "agent/framework", 7, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: "cccccccccccccccccccccccccccccccccccccccc",
	})
	if !errors.Is(err, codehost.ErrMergeRejected) {
		t.Fatalf("protection rejection must map to the sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "Required status checks") {
		t.Fatalf("platform reason must be carried, got %v", err)
	}
}

// 读路径 404(PR 已删/仓库转移)与合并路径同属确定性拒绝:门禁据此回落
// 人工而不是无限重评。
func TestGetMergeRequestNotFoundMapsToRejectionSentinel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	_, err := client.GetMergeRequest(context.Background(), "agent/framework", 7)
	if !errors.Is(err, codehost.ErrMergeRejected) {
		t.Fatalf("read 404 must map to the rejection sentinel, got %v", err)
	}
}

func TestGetMergeRequestReturnsMergedState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"number":7,"merged":true,"merge_commit_sha":"merged444","head":{"ref":"feature","sha":"abc"}}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	mergeRequest, err := client.GetMergeRequest(context.Background(), "agent/framework", 7)
	if err != nil {
		t.Fatalf("get merge request: %v", err)
	}
	if !mergeRequest.Merged || mergeRequest.MergeCommitSHA != "merged444" {
		t.Fatalf("merged state must surface: %#v", mergeRequest)
	}
}

func TestMergePullRequest404MapsToRejectionSentinel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	_, err := client.MergePullRequest(context.Background(), "agent/framework", 7, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: "abc",
	})
	if !errors.Is(err, codehost.ErrMergeRejected) {
		t.Fatalf("merge 404 must map to the rejection sentinel, got %v", err)
	}
}

// 429/408 是本系统语义里唯一该重试的 4xx:mapMergeStatusError 不得把它
// 改写成确定性拒绝,否则限流窗口内的验证任务会被成批永久记为 fallback。
func TestMergePullRequest429StaysTransientRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "test-token", WithHTTPClient(server.Client()))
	_, err := client.MergePullRequest(context.Background(), "agent/framework", 7, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: "abc",
	})
	if !codehost.IsTransientRejection(err) {
		t.Fatalf("429 must stay a transient rejection, got %v", err)
	}
	if errors.Is(err, codehost.ErrMergeRejected) || errors.Is(err, codehost.ErrMergeHeadMismatch) {
		t.Fatalf("429 must not be rewritten to a deterministic sentinel, got %v", err)
	}
}
