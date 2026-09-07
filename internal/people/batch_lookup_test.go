package people_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"sdlc-coordinator/internal/people"
)

func TestBatchLookupOnlyQueriesIncompleteIdentitiesForRequestedGitLab(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	draft, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "待补全", Email: "draft@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}},
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	_, err = service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "已完整", Email: "complete@example.com", Enabled: true,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ExternalUserID: "8", Username: "complete"}},
	})
	if err != nil {
		t.Fatalf("create complete person: %v", err)
	}
	_, err = service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "其他平台", Email: "other@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://other.example.com"}},
	})
	if err != nil {
		t.Fatalf("create other draft: %v", err)
	}

	exact := 0
	lookup := &batchRecordingLookup{results: map[string]people.LookupResult{
		"draft@example.com": {Candidates: []people.LookupCandidate{{ExternalUserID: "7", Username: "draft", DisplayName: "待补全"}}, ExactMatchIndex: &exact},
	}}
	result, err := people.NewBatchLookupService(service, lookup).Lookup(ctx, people.BatchLookupRequest{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", Token: "temporary-token",
	})
	if err != nil {
		t.Fatalf("batch lookup: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].PersonID != draft.PersonID || result.Items[0].IdentityID != draft.Identities[0].IdentityID {
		t.Fatalf("unexpected batch result: %#v", result)
	}
	if len(lookup.requests) != 1 || lookup.requests[0].Email != "draft@example.com" || lookup.requests[0].Token != "temporary-token" || lookup.requests[0].Username != "" {
		t.Fatalf("unexpected lookup requests: %#v", lookup.requests)
	}
}

func TestBatchLookupKeepsOrdinaryLookupFailureOnItsItem(t *testing.T) {
	service := newTestService(t)
	_, err := service.Create(context.Background(), people.CreatePersonRequest{
		DisplayName: "待补全", Email: "draft@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}},
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	lookup := &batchRecordingLookup{errors: map[string]error{"draft@example.com": people.ErrGitLabLookupFailed}}

	result, err := people.NewBatchLookupService(service, lookup).Lookup(context.Background(), people.BatchLookupRequest{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", Token: "temporary-token",
	})
	if err != nil {
		t.Fatalf("batch lookup: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ErrorCode != "lookup_failed" {
		t.Fatalf("unexpected item failure: %#v", result)
	}
}

func TestBatchLookupReturnsAuthenticationFailureForWholeBatch(t *testing.T) {
	service := newTestService(t)
	_, err := service.Create(context.Background(), people.CreatePersonRequest{
		DisplayName: "待补全", Email: "draft@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}},
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	lookup := &batchRecordingLookup{errors: map[string]error{"draft@example.com": people.ErrGitLabAuthFailed}}

	_, err = people.NewBatchLookupService(service, lookup).Lookup(context.Background(), people.BatchLookupRequest{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", Token: "temporary-token",
	})
	if !errors.Is(err, people.ErrGitLabAuthFailed) {
		t.Fatalf("expected authentication failure, got %v", err)
	}
}

type batchRecordingLookup struct {
	mu       sync.Mutex
	requests []people.LookupRequest
	results  map[string]people.LookupResult
	errors   map[string]error
}

func (lookup *batchRecordingLookup) Lookup(_ context.Context, request people.LookupRequest) (people.LookupResult, error) {
	lookup.mu.Lock()
	lookup.requests = append(lookup.requests, request)
	lookup.mu.Unlock()
	if err := lookup.errors[request.Email]; err != nil {
		return people.LookupResult{}, err
	}
	return lookup.results[request.Email], nil
}

func TestBatchLookupSupportsGiteaAndGithubProviders(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	giteaDraft, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "Gitea 待补全", Email: "gitea-user@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "gitea", BaseURL: "https://gitea.example.com"}},
	})
	if err != nil {
		t.Fatalf("create gitea draft: %v", err)
	}
	githubDraft, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "GitHub 待补全", Email: "octocat@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "github", BaseURL: "https://github.example.com"}},
	})
	if err != nil {
		t.Fatalf("create github draft: %v", err)
	}

	exact := 0
	lookup := &batchRecordingLookup{results: map[string]people.LookupResult{
		"gitea-user@example.com": {Candidates: []people.LookupCandidate{{ExternalUserID: "71", Username: "gitea-user", DisplayName: "Gitea 待补全"}}, ExactMatchIndex: &exact},
		"octocat@example.com":    {Candidates: []people.LookupCandidate{{ExternalUserID: "583231", Username: "octocat", DisplayName: "GitHub 待补全"}}, ExactMatchIndex: &exact},
	}}
	result, err := people.NewBatchLookupService(service, lookup).Lookup(ctx, people.BatchLookupRequest{
		Provider: "gitea", BaseURL: "https://gitea.example.com", Token: "gitea-token",
	})
	if err != nil {
		t.Fatalf("gitea batch lookup: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].PersonID != giteaDraft.PersonID {
		t.Fatalf("unexpected gitea batch result: %#v", result)
	}

	result, err = people.NewBatchLookupService(service, lookup).Lookup(ctx, people.BatchLookupRequest{
		Provider: "github", BaseURL: "https://github.example.com", Token: "github-token",
	})
	if err != nil {
		t.Fatalf("github batch lookup: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].PersonID != githubDraft.PersonID {
		t.Fatalf("unexpected github batch result: %#v", result)
	}

	if _, err := people.NewBatchLookupService(service, lookup).Lookup(ctx, people.BatchLookupRequest{
		Provider: "bitbucket", BaseURL: "https://bitbucket.example.com", Token: "token",
	}); err == nil {
		t.Fatal("expected unsupported provider to be rejected")
	}
}

// P1 回归:批量补全中"邮箱前缀猜用户名"未命中(平台 404)只影响单条,
// 不得整批终止——命中者照常拿到候选。
func TestBatchLookupKeepsGoingWhenGiteaUserMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/api/v1/users/hit" {
			_, _ = w.Write([]byte(`{"id":71,"login":"hit","full_name":"Hit User"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	service := newTestService(t)
	ctx := context.Background()
	hit, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "命中", Email: "hit@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "gitea", BaseURL: server.URL}},
	})
	if err != nil {
		t.Fatalf("create hit draft: %v", err)
	}
	missing, err := service.Create(ctx, people.CreatePersonRequest{
		DisplayName: "未命中", Email: "missing-user@example.com", Enabled: false,
		Identities: []people.IdentityInput{{Provider: "gitea", BaseURL: server.URL}},
	})
	if err != nil {
		t.Fatalf("create missing draft: %v", err)
	}

	result, err := people.NewBatchLookupService(service, people.NewGiteaLookupClient(server.Client())).Lookup(ctx, people.BatchLookupRequest{
		Provider: "gitea", BaseURL: server.URL, Token: "gitea-token",
	})
	if err != nil {
		t.Fatalf("batch lookup must not fail on missing users: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected both drafts in batch result, got %#v", result)
	}
	byPerson := map[string]people.BatchLookupItem{}
	for _, item := range result.Items {
		byPerson[item.PersonID] = item
	}
	if len(byPerson[hit.PersonID].Candidates) != 1 || byPerson[hit.PersonID].ErrorCode != "" {
		t.Fatalf("expected hit draft to carry candidate, got %#v", byPerson[hit.PersonID])
	}
	if len(byPerson[missing.PersonID].Candidates) != 0 || byPerson[missing.PersonID].ErrorCode != "" {
		t.Fatalf("expected missing draft to be a clean no-candidate row, got %#v", byPerson[missing.PersonID])
	}
}
