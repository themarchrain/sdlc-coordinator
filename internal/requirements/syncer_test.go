package requirements

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
)

type fakeIssueClient struct {
	reconcileResult codehost.ReconcileResult
	reconcileErr    error
	createIssue     *codehost.Issue
	createErr       error
	states          map[int]codehost.IssueState
	stateErr        error
	created         []codehost.CreateIssueRequest
	createCalls     int
}

func (f *fakeIssueClient) CreateIssue(_ context.Context, _ string, request codehost.CreateIssueRequest) (*codehost.Issue, error) {
	f.createCalls++
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, request)
	return f.createIssue, nil
}

func (f *fakeIssueClient) ReconcileWriteback(_ context.Context, _ codehost.ReconcileRequest) (codehost.ReconcileResult, error) {
	return f.reconcileResult, f.reconcileErr
}

func (f *fakeIssueClient) GetIssueState(_ context.Context, _ string, iid int) (codehost.IssueState, error) {
	if f.stateErr != nil {
		return codehost.IssueState{}, f.stateErr
	}
	return f.states[iid], nil
}

func newSyncTestService(t *testing.T, client *fakeIssueClient, repoStatus registeredrepo.RepositoryStatus) (*Service, *Store) {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	requirementStore := NewStore(db)
	if err := requirementStore.Migrate(); err != nil {
		t.Fatalf("migrate requirements: %v", err)
	}
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate repos: %v", err)
	}
	if err := repoStore.Create(context.Background(), &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-1", DisplayName: "Demo", PathWithNamespace: "group/demo", Token: "tok-1", Status: repoStatus,
	}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	service := NewService(requirementStore, repoStore, func(*registeredrepo.RegisteredRepository) ClientFactory { return client })
	return service, requirementStore
}

func containsLabel(labels []string, expected string) bool {
	for _, label := range labels {
		if strings.EqualFold(label, expected) {
			return true
		}
	}
	return false
}

func TestSyncOnceCreatesIssueWithSignatureAndLabel(t *testing.T) {
	client := &fakeIssueClient{createIssue: &codehost.Issue{IID: 7, WebURL: "http://codehost/group/demo/issues/7"}}
	service, requirementStore := newSyncTestService(t, client, registeredrepo.StatusEnabled)
	ctx := context.Background()
	item := &Requirement{Title: "重构登录", Description: "用新设计", RepositoryID: "repo-1", Labels: EncodeLabels([]string{"frontend"})}
	if err := requirementStore.Create(ctx, item); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := service.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("expected 1 created, got %+v", result)
	}
	got, _ := requirementStore.Get(ctx, item.RequirementID)
	if got.Status != StatusPublished || got.IssueIID != 7 || got.IssueURL == "" || got.PublishedAt == nil {
		t.Fatalf("requirement must be published: %+v", got)
	}
	if len(client.created) != 1 {
		t.Fatalf("expected one create call, got %d", len(client.created))
	}
	request := client.created[0]
	if !containsLabel(request.Labels, DeveloperAgentLabel) || !containsLabel(request.Labels, "frontend") {
		t.Fatalf("labels must contain developer-agent and user labels, got %v", request.Labels)
	}
	if !strings.Contains(request.Description, item.SourceKey()) ||
		!strings.Contains(request.Description, "req-source-key: "+item.SourceKey()) ||
		!strings.Contains(request.Description, "用新设计") {
		t.Fatalf("description must keep body and embed signature: %q", request.Description)
	}
}

func TestSyncOnceRecoversOrphanIssueWithoutCreating(t *testing.T) {
	client := &fakeIssueClient{reconcileResult: codehost.ReconcileResult{State: codehost.ReconcileStateFound, ExternalID: "9", ExternalURL: "http://codehost/group/demo/issues/9"}}
	service, requirementStore := newSyncTestService(t, client, registeredrepo.StatusEnabled)
	ctx := context.Background()
	item := &Requirement{Title: "孤儿恢复", RepositoryID: "repo-1", Status: StatusQueued}
	if err := requirementStore.Create(ctx, item); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := service.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Recovered != 1 || client.createCalls != 0 {
		t.Fatalf("must recover without create: %+v calls=%d", result, client.createCalls)
	}
	got, _ := requirementStore.Get(ctx, item.RequirementID)
	if got.Status != StatusPublished || got.IssueIID != 9 {
		t.Fatalf("recovered requirement wrong: %+v", got)
	}
}

func TestSyncOnceMarksFailedThenRetriesNextRound(t *testing.T) {
	client := &fakeIssueClient{createErr: errors.New("500 internal"), createIssue: &codehost.Issue{IID: 5, WebURL: "u"}}
	service, requirementStore := newSyncTestService(t, client, registeredrepo.StatusEnabled)
	ctx := context.Background()
	item := &Requirement{Title: "会失败", RepositoryID: "repo-1", Status: StatusQueued}
	if err := requirementStore.Create(ctx, item); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := service.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("expected failure counted: %+v", result)
	}
	got, _ := requirementStore.Get(ctx, item.RequirementID)
	if got.Status != StatusFailed || got.LastError == "" {
		t.Fatalf("failed item must keep error: %+v", got)
	}

	client.createErr = nil
	result, err = service.SyncOnce(ctx)
	if err != nil || result.Created != 1 {
		t.Fatalf("retry must create: %+v err=%v", result, err)
	}
	got, _ = requirementStore.Get(ctx, item.RequirementID)
	if got.Status != StatusPublished || got.LastError != "" {
		t.Fatalf("retry must clear error: %+v", got)
	}
}

func TestSyncOnceLimitsCreateWritesPerRound(t *testing.T) {
	client := &fakeIssueClient{createIssue: &codehost.Issue{IID: 1, WebURL: "u"}}
	service, requirementStore := newSyncTestService(t, client, registeredrepo.StatusEnabled)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		if err := requirementStore.Create(ctx, &Requirement{Title: "r", RepositoryID: "repo-1", Status: StatusQueued}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	result, err := service.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Created != 5 || client.createCalls != 5 {
		t.Fatalf("write limit must cap at 5: %+v calls=%d", result, client.createCalls)
	}
	items, _ := requirementStore.ListProcessable(ctx, []Status{StatusQueued}, 10)
	if len(items) != 1 {
		t.Fatalf("one item must remain queued, got %d", len(items))
	}
}

func TestSyncOnceSkipsDisabledRepository(t *testing.T) {
	client := &fakeIssueClient{createIssue: &codehost.Issue{IID: 1, WebURL: "u"}}
	service, requirementStore := newSyncTestService(t, client, registeredrepo.StatusDisabled)
	ctx := context.Background()
	item := &Requirement{Title: "x", RepositoryID: "repo-1", Status: StatusQueued}
	if err := requirementStore.Create(ctx, item); err != nil {
		t.Fatalf("seed: %v", err)
	}
	result, err := service.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Skipped != 1 || client.createCalls != 0 {
		t.Fatalf("disabled repo must skip: %+v calls=%d", result, client.createCalls)
	}
	got, _ := requirementStore.Get(ctx, item.RequirementID)
	if got.Status != StatusQueued || got.LastError == "" {
		t.Fatalf("skipped item stays queued with hint: %+v", got)
	}
}

func TestSyncOnceMirrorsClosedIssue(t *testing.T) {
	client := &fakeIssueClient{states: map[int]codehost.IssueState{7: {Closed: true}}}
	service, requirementStore := newSyncTestService(t, client, registeredrepo.StatusEnabled)
	ctx := context.Background()
	item := &Requirement{Title: "done 信号", RepositoryID: "repo-1", Status: StatusPublished, IssueIID: 7}
	if err := requirementStore.Create(ctx, item); err != nil {
		t.Fatalf("seed: %v", err)
	}
	result, err := service.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Mirrored != 1 {
		t.Fatalf("expected mirrored=1, got %+v", result)
	}
	got, _ := requirementStore.Get(ctx, item.RequirementID)
	if got.Status != StatusDone || got.CompletedAt == nil {
		t.Fatalf("closed issue must complete requirement: %+v", got)
	}
}

func TestSyncOnceKeepsPublishedWhenOpenOrUnreadable(t *testing.T) {
	client := &fakeIssueClient{states: map[int]codehost.IssueState{7: {Closed: false}}, stateErr: errors.New("read failed")}
	service, requirementStore := newSyncTestService(t, client, registeredrepo.StatusEnabled)
	ctx := context.Background()
	item := &Requirement{Title: "still open", RepositoryID: "repo-1", Status: StatusPublished, IssueIID: 7}
	if err := requirementStore.Create(ctx, item); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 读失败分支:stateErr 非 nil
	result, err := service.SyncOnce(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.MirrorErrors != 1 {
		t.Fatalf("read failure must count mirror_errors: %+v", result)
	}
	got, _ := requirementStore.Get(ctx, item.RequirementID)
	if got.Status != StatusPublished {
		t.Fatalf("read failure must keep published: %+v", got)
	}
	// open 分支:恢复可读但 issue 未关闭
	client.stateErr = nil
	if _, err := service.SyncOnce(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got, _ = requirementStore.Get(ctx, item.RequirementID)
	if got.Status != StatusPublished {
		t.Fatalf("open issue must keep published: %+v", got)
	}
}

func TestSyncOnceRejectsConcurrentRun(t *testing.T) {
	client := &fakeIssueClient{createIssue: &codehost.Issue{IID: 1, WebURL: "u"}}
	service, _ := newSyncTestService(t, client, registeredrepo.StatusEnabled)
	service.syncMu.Lock()
	defer service.syncMu.Unlock()
	if _, err := service.SyncOnce(context.Background()); !errors.Is(err, ErrSyncInProgress) {
		t.Fatalf("concurrent sync must be refused, got %v", err)
	}
}
