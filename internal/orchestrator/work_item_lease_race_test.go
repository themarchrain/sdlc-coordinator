package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	storerepo "sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

func TestSameInstanceWorkItemLeaseAcquisitionsCompete(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "same_owner_lease")
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := store.Migrate(db, &model.WorkItemLease{}); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{workItemLeases: storerepo.NewWorkItemLeaseRepository(db), workItemLeaseOwner: "same-instance"}
	ctx := context.Background()
	first, won, err := o.acquireWorkItemLease(ctx, "codehost-writeback:same-key", "codehost-writeback", time.Minute)
	if err != nil || !won {
		t.Fatalf("first acquisition: won=%v err=%v", won, err)
	}
	defer o.releaseWorkItemLease(first)
	second, won, err := o.acquireWorkItemLease(ctx, "codehost-writeback:same-key", "codehost-writeback", time.Minute)
	if err != nil || won {
		t.Fatalf("same-instance second acquisition: lease=%+v won=%v err=%v", second, won, err)
	}
}

// TestWritebackLeaseDualInstanceSingleExecution 两实例同时执行同一 codehost
// 写回：租约执行权保证远端 CreateIssue 恰好发生一次，败者不执行远端调用。
func TestWritebackLeaseDualInstanceSingleExecution(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "wb_lease")

	var createIssueCalls atomic.Int32
	backend := &countingWritebackBackend{counter: &createIssueCalls}

	ctx := context.Background()
	orchs := make([]*Orchestrator, 2)
	for i := range orchs {
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open instance %d: %v", i, err)
		}
		defer func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}()
		if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Writeback{}, &model.WorkItemLease{}); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		orchs[i] = New(
			storerepo.NewTaskRepository(db),
			storerepo.NewRunRepository(db),
			WithArtifactRepository(storerepo.NewArtifactRepository(db)),
			WithWritebackRepository(storerepo.NewWritebackRepository(db)),
			WithWorkItemLeases(storerepo.NewWorkItemLeaseRepository(db)),
			WithWorkItemLeaseOwner(fmt.Sprintf("test-instance-%d", i)),
			WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
		)
	}

	seedTaskWithRepoBinding(t, orchs[0], "task-wb-lease", "run-wb-lease")
	// 先落一次行:两实例从同一 pending 快照出发,竞争的是租约执行权而不是行插入。
	if err := orchs[0].writebacks.UpsertByDedupeKey(ctx, &model.Writeback{
		TaskID: "task-wb-lease", RunID: "run-wb-lease", DedupeKey: "wb-lease:1",
		Kind: CodeHostWritebackCreateIssue, ProjectID: "1",
		Status:      model.WritebackStatusPending,
		RequestJSON: `{"title":"t","description":"d"}`,
	}); err != nil {
		t.Fatalf("seed writeback: %v", err)
	}

	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range orchs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = orchs[i].applyOneCodeHostWriteback(ctx, &model.Writeback{
				TaskID: "task-wb-lease", RunID: "run-wb-lease", DedupeKey: "wb-lease:1",
				Kind: CodeHostWritebackCreateIssue, ProjectID: "1",
				Status:      model.WritebackStatusPending,
				RequestJSON: `{"title":"t","description":"d"}`,
			})
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("instance %d: %v", i, err)
		}
	}
	if got := createIssueCalls.Load(); got != 1 {
		t.Fatalf("CreateIssue called %d times across instances, want exactly 1", got)
	}
}

// TestMergeGateLeaseDualInstanceSingleMergeCall 双实例同评同一验证任务：
// 租约执行权保证合并调用只发生一次，败者本轮 skipped 且不写审计。
func TestMergeGateLeaseDualInstanceSingleMergeCall(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "gate_lease")

	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	orchs := make([]*Orchestrator, 2)
	var audits *storerepo.MergeGateAuditRepository
	for i := range orchs {
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open instance %d: %v", i, err)
		}
		defer func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}()
		if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.MergeGateAudit{}, &model.Effect{}, &model.WorkItemLease{}); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		lookup := &gateRepositoryLookup{repo: &registeredrepo.RegisteredRepository{
			RepositoryID: fixtureRepositoryID, Provider: "github",
			BaseURL: "https://github.example", PathWithNamespace: "group/project", Token: "gate-token", Status: registeredrepo.StatusEnabled,
		}}
		orchs[i] = New(
			storerepo.NewTaskRepository(db),
			storerepo.NewRunRepository(db),
			WithArtifactRepository(storerepo.NewArtifactRepository(db)),
			WithVerificationArtifactReader(storerepo.NewArtifactRepository(db)),
			WithMergeGateAuditRepository(storerepo.NewMergeGateAuditRepository(db)),
			WithRepositoryLookup(lookup),
			WithAutoMergeEnabled(func(context.Context) bool { return true }),
			WithMergeGateClients(func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error) {
				return client, client, nil
			}),
			WithWorkItemLeases(storerepo.NewWorkItemLeaseRepository(db)),
			WithWorkItemLeaseOwner(fmt.Sprintf("test-instance-%d", i)),
		)
		audits = storerepo.NewMergeGateAuditRepository(db)
	}

	verifier := gateFixture(t, ctx,
		storerepo.NewTaskRepository(mustOpen(t, dsn)),
		storerepo.NewArtifactRepository(mustOpen(t, dsn)),
	)

	results := make([][]MergeGateTickResult, 2)
	gateErrs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range orchs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], gateErrs[i] = orchs[i].RunAutoMergeGate(ctx, 5)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range gateErrs {
		if err != nil {
			t.Fatalf("instance %d gate: %v", i, err)
		}
	}
	if client.mergeCalls != 1 {
		t.Fatalf("MergePullRequest called %d times across instances, want exactly 1", client.mergeCalls)
	}
	totalAudited := 0
	for i := range results {
		for _, decision := range results[i] {
			if decision.TaskID != verifier.TaskID {
				continue
			}
			if decision.Decision == MergeGateDecisionSkipped && decision.Error == "" {
				continue
			}
			if decision.Decision == MergeGateDecisionMerged || decision.Decision == MergeGateDecisionFallback {
				totalAudited++
			}
		}
	}
	if totalAudited != 1 {
		t.Fatalf("exactly one instance may produce a deterministic decision, got %d (%+v / %+v)", totalAudited, results[0], results[1])
	}
	if _, err := audits.GetByTaskID(ctx, verifier.TaskID); err != nil {
		t.Fatalf("winner must persist the audit: %v", err)
	}
}

// seedTaskWithRepoBinding 铺一条绑定注册仓的开发任务，供写回链解析 backend。
func seedTaskWithRepoBinding(t *testing.T, orch *Orchestrator, taskID, runID string) {
	t.Helper()
	ctx := context.Background()
	repositoryID := fixtureRepositoryID
	if err := orch.tasks.Create(ctx, &model.Task{
		TaskID: taskID, DedupeKey: "dedupe-" + taskID, Type: "bugfix", Role: "DeveloperWorker",
		TargetRepo: "group/framework", RepositoryID: &repositoryID, Status: model.TaskStatusRunning,
		Objective: "o", AcceptanceCriteria: "a",
	}); err != nil {
		t.Fatal(err)
	}
	completed := model.RunStatusSucceeded
	if err := orch.runs.Create(ctx, &model.Run{
		RunID: runID, TaskID: taskID, WorkerRole: "DeveloperWorker", Status: completed,
	}); err != nil {
		t.Fatal(err)
	}
}

// countingWritebackBackend 计数 CreateIssue 的线程安全 backend 桩。
type countingWritebackBackend struct {
	counter *atomic.Int32
}

func (b *countingWritebackBackend) CreateIssue(ctx context.Context, projectID string, req codehost.CreateIssueRequest) (*codehost.Issue, error) {
	b.counter.Add(1)
	return &codehost.Issue{IID: 42, WebURL: "https://gitlab.example/" + projectID + "/-/issues/42"}, nil
}

func (b *countingWritebackBackend) CreateMergeRequest(ctx context.Context, projectID string, req codehost.CreateMergeRequestRequest) (*codehost.MergeRequest, error) {
	return &codehost.MergeRequest{IID: 7}, nil
}

func (b *countingWritebackBackend) CreateIssueNote(ctx context.Context, projectID string, iid int, req codehost.CreateNoteRequest) (*codehost.Note, error) {
	return &codehost.Note{ID: 1}, nil
}

func (b *countingWritebackBackend) CreateMergeRequestNote(ctx context.Context, projectID string, iid int, req codehost.CreateNoteRequest) (*codehost.Note, error) {
	return &codehost.Note{ID: 2}, nil
}

func (b *countingWritebackBackend) SetIssueLabels(ctx context.Context, projectID string, iid int, req codehost.SetLabelsRequest) (*codehost.Issue, error) {
	return &codehost.Issue{IID: iid}, nil
}

func (b *countingWritebackBackend) SetMergeRequestLabels(ctx context.Context, projectID string, iid int, req codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	return &codehost.MergeRequest{IID: iid}, nil
}

func mustOpen(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}
