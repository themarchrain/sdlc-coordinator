package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	storerepo "sdlc-coordinator/internal/store/repository"
)

func newMergeGateRepositories(t *testing.T) (*storerepo.TaskRepository, *storerepo.RunRepository, *storerepo.ArtifactRepository, *storerepo.MergeGateAuditRepository) {
	t.Helper()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.MergeGateAudit{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return storerepo.NewTaskRepository(db), storerepo.NewRunRepository(db), storerepo.NewArtifactRepository(db), storerepo.NewMergeGateAuditRepository(db)
}

// gateArtifact 落一条 verdict/绑定 artifact:门禁消费的证据本体。
func gateArtifact(t *testing.T, repo *storerepo.ArtifactRepository, taskID, runID, name string, value any) {
	t.Helper()
	metadata, err := json.Marshal(map[string]any{"source_key": name, "value": value})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertByIdentity(context.Background(), &model.Artifact{
		ArtifactID:   "artifact-" + taskID + "-" + name,
		TaskID:       taskID,
		RunID:        runID,
		Kind:         "generic",
		Name:         name,
		MetadataJSON: string(metadata),
	}); err != nil {
		t.Fatal(err)
	}
}

const gateHeadSHA = "abc123abc123abc123abc123abc123abc123abc1"

// gateFixture 铺一条绿证据链:验证任务 succeeded(上下文带 MR 身份与父任务),
// tester verdict=passed 且 head 绑定一致,父审查任务 approved 且同一头部。
func gateFixture(t *testing.T, ctx context.Context, tasks *storerepo.TaskRepository, artifacts *storerepo.ArtifactRepository) *model.Task {
	t.Helper()
	repositoryID := fixtureRepositoryID
	verifier := &model.Task{
		TaskID: "task-gate-verify", DedupeKey: "verify:group/project:12:" + gateHeadSHA, Type: "verify_merge_request", Role: "TesterWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: &repositoryID, Status: model.TaskStatusSucceeded,
		Objective: "verify", AcceptanceCriteria: "evidence",
		ContextJSON: `{"parent_task_id":"task-gate-review","project":{"path":"group/project"},"merge_request":{"iid":12,"head_sha":"` + gateHeadSHA + `"},"repos":[{"name":"project","branch":"feature/change","access":"readable"}]}`,
	}
	if err := tasks.Create(ctx, verifier); err != nil {
		t.Fatal(err)
	}
	parent := &model.Task{
		TaskID: "task-gate-review", DedupeKey: "review:task-gate-review", Type: "review_mr", Role: "ReviewerWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: &repositoryID, Status: model.TaskStatusSucceeded,
		Objective: "review", AcceptanceCriteria: "verdict",
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":12,"head_sha":"` + gateHeadSHA + `"},"repos":[{"name":"project","branch":"feature/change","access":"readable"}]}`,
	}
	if err := tasks.Create(ctx, parent); err != nil {
		t.Fatal(err)
	}
	gateArtifact(t, artifacts, verifier.TaskID, "run-gate-verify", "verification_verdict", "verification_passed")
	gateArtifact(t, artifacts, verifier.TaskID, "run-gate-verify", "verification_head_sha", gateHeadSHA)
	gateArtifact(t, artifacts, parent.TaskID, "run-gate-review", "review_verdict", "approved")
	return verifier
}

type fakeGateClient struct {
	liveHeadSHA string
	// mergedAfterRead 从第 N 次读起报告平台已合并(0=从不),模拟「合并已
	// 在更早轮次生效」的真相恢复场景。
	mergedAfterRead int
	mergeCommitSHA  string
	readErr         error
	mergeCalls      int
	mergeReq        codehost.MergePullRequestRequest
	mergeErr        error
	reads           int
}

func (f *fakeGateClient) GetMergeRequest(context.Context, string, int) (*codehost.MergeRequest, error) {
	f.reads++
	if f.readErr != nil {
		return nil, f.readErr
	}
	if f.mergedAfterRead > 0 && f.reads >= f.mergedAfterRead {
		return &codehost.MergeRequest{IID: 12, HeadSHA: f.liveHeadSHA, Merged: true, MergeCommitSHA: f.mergeCommitSHA}, nil
	}
	return &codehost.MergeRequest{IID: 12, HeadSHA: f.liveHeadSHA}, nil
}

func (f *fakeGateClient) MergePullRequest(_ context.Context, _ string, _ int, request codehost.MergePullRequestRequest) (*codehost.MergePullRequestResult, error) {
	f.mergeCalls++
	f.mergeReq = request
	if f.mergeErr != nil {
		return nil, f.mergeErr
	}
	return &codehost.MergePullRequestResult{Merged: true, MergedCommitSHA: "merged999"}, nil
}

type gateRepositoryLookup struct {
	repo *registeredrepo.RegisteredRepository
}

func (f gateRepositoryLookup) Get(context.Context, string) (*registeredrepo.RegisteredRepository, error) {
	return f.repo, nil
}

// newGateOrch 组装被测编排器:开关恒开、客户端由 fakeGateClient 承载。
func newGateOrch(t *testing.T, client *fakeGateClient) (*Orchestrator, *storerepo.TaskRepository, *storerepo.ArtifactRepository, *storerepo.MergeGateAuditRepository) {
	t.Helper()
	tasks, runs, artifacts, audits := newMergeGateRepositories(t)
	lookup := gateRepositoryLookup{repo: &registeredrepo.RegisteredRepository{
		RepositoryID: fixtureRepositoryID, Provider: "github",
		BaseURL: "https://github.example", PathWithNamespace: "group/project", Token: "gate-token",
	}}
	orch := New(tasks, runs,
		WithArtifactRepository(artifacts),
		WithVerificationArtifactReader(artifacts),
		WithMergeGateAuditRepository(audits),
		WithRepositoryLookup(lookup),
		WithAutoMergeEnabled(func(context.Context) bool { return true }),
		WithMergeGateClients(func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error) {
			return client, client, nil
		}),
	)
	return orch, tasks, artifacts, audits
}

func TestAutoMergeGateMergesWhenEvidenceChainIsGreen(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	orch, tasks, artifacts, audits := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "merged" {
		t.Fatalf("green chain must merge, got %#v", decisions)
	}
	if client.mergeCalls != 1 {
		t.Fatalf("merger must be called once, got %d", client.mergeCalls)
	}
	if client.mergeReq.ExpectedHeadSHA != gateHeadSHA {
		t.Fatalf("merge call must assert the evidence head sha, got %#v", client.mergeReq)
	}
	audit, err := audits.GetByTaskID(ctx, "task-gate-verify")
	if err != nil {
		t.Fatalf("audit must be written: %v", err)
	}
	if audit.Decision != "merged" || audit.MergedCommitSHA != "merged999" || audit.HeadSHA != gateHeadSHA {
		t.Fatalf("unexpected audit: %#v", audit)
	}

	// 审计防重:同一验证任务至多尝试一次。
	repeat, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil || len(repeat) != 0 {
		t.Fatalf("audited task must not be re-evaluated: %#v err=%v", repeat, err)
	}
}

func TestAutoMergeGateSkipsWhenSwitchOff(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	tasks, runs, artifacts, audits := newMergeGateRepositories(t)
	orch := New(tasks, runs,
		WithArtifactRepository(artifacts),
		WithVerificationArtifactReader(artifacts),
		WithMergeGateAuditRepository(audits),
		WithRepositoryLookup(gateRepositoryLookup{}),
		WithAutoMergeEnabled(func(context.Context) bool { return false }),
		WithMergeGateClients(func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error) {
			return client, client, nil
		}),
	)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil || len(decisions) != 0 {
		t.Fatalf("switch off must short-circuit: decisions=%#v err=%v", decisions, err)
	}
	if client.mergeCalls != 0 {
		t.Fatalf("switch off must not merge")
	}
}

func TestAutoMergeGateFallsBackWhenReviewerEvidenceMissing(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	orch, tasks, artifacts, audits := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)
	// 清掉父任务的 verdict artifact:审查证据缺失。
	if err := artifacts.UpsertByIdentity(ctx, &model.Artifact{
		ArtifactID: "artifact-task-gate-review-review_verdict", TaskID: "task-gate-review", RunID: "run-gate-review",
		Kind: "generic", Name: "review_verdict", MetadataJSON: `{"source_key":"review_verdict","value":"changes_requested"}`,
	}); err != nil {
		t.Fatal(err)
	}

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "fallback" {
		t.Fatalf("missing approval must fall back, got %#v", decisions)
	}
	if client.mergeCalls != 0 {
		t.Fatalf("fallback must not merge")
	}
	if _, err := audits.GetByTaskID(ctx, "task-gate-verify"); err != nil {
		t.Fatalf("fallback must leave an audit trail: %v", err)
	}
}

func TestAutoMergeGateFallsBackWhenReviewerHeadDiffers(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	orch, tasks, artifacts, _ := newGateOrch(t, client)
	verifier := gateFixture(t, ctx, tasks, artifacts)
	// 父审查任务上下文头部改写为旧头部:审查证据 SHA 不一致即过期。
	parent, err := tasks.GetByTaskID(ctx, "task-gate-review")
	if err != nil {
		t.Fatal(err)
	}
	parent.ContextJSON = `{"project":{"path":"group/project"},"merge_request":{"iid":12,"head_sha":"oldhead"},"repos":[{"name":"project","branch":"feature/change","access":"readable"}]}`
	if err := tasks.Update(ctx, parent); err != nil {
		t.Fatal(err)
	}

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "fallback" {
		t.Fatalf("stale reviewer evidence must fall back, got %#v", decisions)
	}
	if client.mergeCalls != 0 || verifier == nil {
		t.Fatalf("stale evidence must not merge")
	}
}

func TestAutoMergeGateFallsBackWhenLiveHeadMoved(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: "movedhead"}
	orch, tasks, artifacts, _ := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "fallback" {
		t.Fatalf("live head drift must fall back, got %#v", decisions)
	}
	if client.mergeCalls != 0 {
		t.Fatalf("head drift must never reach the merge call")
	}
}

func TestAutoMergeGateSkipsTransientPlatformReadFailure(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{readErr: errors.New("network unreachable")}
	orch, tasks, artifacts, audits := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "skipped" || decisions[0].Error == "" {
		t.Fatalf("transient read failure must skip without fallback, got %#v", decisions)
	}
	if _, err := audits.GetByTaskID(ctx, "task-gate-verify"); err == nil {
		t.Fatalf("transient failure must not write an audit row")
	}
}

func TestAutoMergeGateFallsBackOnHeadMismatchSentinel(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{
		liveHeadSHA: gateHeadSHA,
		mergeErr:    fmt.Errorf("%w: head moved between read and merge", codehost.ErrMergeHeadMismatch),
	}
	orch, tasks, artifacts, audits := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "fallback" {
		t.Fatalf("head mismatch rejection must fall back, got %#v", decisions)
	}
	if _, err := audits.GetByTaskID(ctx, "task-gate-verify"); err != nil {
		t.Fatalf("rejection must be audited: %v", err)
	}
}

func TestAutoMergeGateDoesNotAuditAmbiguousMergeFailure(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{
		liveHeadSHA: gateHeadSHA,
		mergeErr:    codehost.NewAmbiguousError(errors.New("connection reset")),
	}
	orch, tasks, artifacts, audits := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "skipped" {
		t.Fatalf("ambiguous merge failure must skip, got %#v", decisions)
	}
	if _, err := audits.GetByTaskID(ctx, "task-gate-verify"); err == nil {
		t.Fatalf("ambiguous failure must not be audited: the next round may converge")
	}
}

func TestAutoMergeGateFallsBackWhenMergeCapabilityMissing(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	orch, tasks, artifacts, _ := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)
	// 平台客户端只读不写:合并能力缺失回落人工。
	orch.mergeGateClients = func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error) {
		return client, nil, nil
	}

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "fallback" {
		t.Fatalf("missing merge capability must fall back, got %#v", decisions)
	}
}

func TestListSucceededVerificationsWithoutAuditExcludesAuditedTasks(t *testing.T) {
	ctx := context.Background()
	tasks, _, artifacts, audits := newMergeGateRepositories(t)
	gateFixture(t, ctx, tasks, artifacts)

	if err := audits.Create(ctx, &model.MergeGateAudit{
		AuditID: "mga-1", TaskID: "task-gate-verify", RepositoryID: fixtureRepositoryID, Decision: "fallback",
	}); err != nil {
		t.Fatal(err)
	}

	candidates, err := tasks.ListSucceededVerificationsWithoutAudit(ctx, 5)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("audited verification must be excluded: %#v", candidates)
	}
}

// 确定性故障必须归 fallback 并写审计:留在 skipped 会形成每轮重扫的静默
// 死信队列,还会占满取数窗口饿死后续任务。
func TestAutoMergeGateFallsBackOnUnreadableContext(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	orch, tasks, artifacts, audits := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)
	corrupted, err := tasks.GetByTaskID(ctx, "task-gate-verify")
	if err != nil {
		t.Fatal(err)
	}
	corrupted.ContextJSON = "{not-json"
	if err := tasks.Update(ctx, corrupted); err != nil {
		t.Fatal(err)
	}

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "fallback" {
		t.Fatalf("unreadable context must fall back, got %#v", decisions)
	}
	if client.mergeCalls != 0 {
		t.Fatalf("fallback must not merge")
	}
	if _, err := audits.GetByTaskID(ctx, "task-gate-verify"); err != nil {
		t.Fatalf("deterministic failure must be audited: %v", err)
	}
}

func TestAutoMergeGateFallsBackWhenReviewerTaskMissing(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	orch, tasks, artifacts, _ := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)
	// 把血缘指向不存在的审查任务:父任务查无是确定性缺环。
	orphaned, err := tasks.GetByTaskID(ctx, "task-gate-verify")
	if err != nil {
		t.Fatal(err)
	}
	orphaned.ContextJSON = `{"parent_task_id":"task-gone","project":{"path":"group/project"},"merge_request":{"iid":12,"head_sha":"` + gateHeadSHA + `"},"repos":[{"name":"project","branch":"feature/change","access":"readable"}]}`
	if err := tasks.Update(ctx, orphaned); err != nil {
		t.Fatal(err)
	}

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "fallback" {
		t.Fatalf("missing reviewer task must fall back, got %#v", decisions)
	}
	if client.mergeCalls != 0 {
		t.Fatalf("fallback must not merge")
	}
}

func TestAutoMergeGateFallsBackOnPlatformReadRejection(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{
		readErr: fmt.Errorf("%w: not found after repository transfer", codehost.ErrMergeRejected),
	}
	orch, tasks, artifacts, audits := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "fallback" {
		t.Fatalf("deterministic platform rejection must fall back, got %#v", decisions)
	}
	if _, err := audits.GetByTaskID(ctx, "task-gate-verify"); err != nil {
		t.Fatalf("deterministic rejection must be audited: %v", err)
	}
}

// 平台已显示合并=此前某轮合并已生效(审计丢失或人工合并):按 merged 补
// 审计,不再重发合并调用。
func TestAutoMergeGateRecordsMergedWhenPlatformAlreadyMerged(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{
		liveHeadSHA:     gateHeadSHA,
		mergedAfterRead: 1,
		mergeCommitSHA:  "already777",
	}
	orch, tasks, artifacts, audits := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "merged" {
		t.Fatalf("already-merged platform state must record merged, got %#v", decisions)
	}
	if client.mergeCalls != 0 {
		t.Fatalf("already-merged platform must not be merged again")
	}
	audit, err := audits.GetByTaskID(ctx, "task-gate-verify")
	if err != nil {
		t.Fatal(err)
	}
	if audit.MergedCommitSHA != "already777" || audit.MergeRequestIID != 12 || audit.ProjectPath != "group/project" {
		t.Fatalf("audit must carry the platform commit and MR identity: %#v", audit)
	}
}

// 拒绝先回读真相:合并已在更早的歧义轮次生效时,按 merged 补审计而不是
// 误记 fallback。
func TestAutoMergeGateRecoversMergedTruthAfterRejection(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{
		liveHeadSHA:     gateHeadSHA,
		mergedAfterRead: 2,
		mergeCommitSHA:  "truthy888",
		mergeErr:        fmt.Errorf("%w: pull request is not mergeable", codehost.ErrMergeRejected),
	}
	orch, tasks, artifacts, audits := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "merged" {
		t.Fatalf("rejection with merged truth must record merged, got %#v", decisions)
	}
	audit, err := audits.GetByTaskID(ctx, "task-gate-verify")
	if err != nil {
		t.Fatal(err)
	}
	if audit.MergedCommitSHA != "truthy888" {
		t.Fatalf("audit must carry the recovered commit: %#v", audit)
	}
}

// 回落人工必须有 IM 触点:MR 上证据链看着全绿却没被合并,通知必须送达,
// 否则只能靠查库发现(与停靠通知同款 best-effort,按验证任务去重)。
func TestAutoMergeGateMaterializesFallbackIM(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: "movedhead"}
	tasks, runs, artifacts, audits := newMergeGateRepositories(t)
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatal(err)
	}
	effectRepo := storerepo.NewEffectRepository(db)
	orch := New(tasks, runs,
		WithArtifactRepository(artifacts),
		WithVerificationArtifactReader(artifacts),
		WithMergeGateAuditRepository(audits),
		WithRepositoryLookup(gateRepositoryLookup{repo: &registeredrepo.RegisteredRepository{
			RepositoryID: fixtureRepositoryID, Provider: "github",
			BaseURL: "https://github.example", PathWithNamespace: "group/project", Token: "gate-token",
		}}),
		WithAutoMergeEnabled(func(context.Context) bool { return true }),
		WithMergeGateClients(func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error) {
			return client, client, nil
		}),
		WithEffectRepository(effectRepo),
	)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != "fallback" {
		t.Fatalf("head drift must fall back, got %#v", decisions)
	}
	effect, err := effectRepo.GetByDedupeKey(ctx, "merge-gate-fallback-im:task-gate-verify")
	if err != nil {
		t.Fatalf("fallback must materialize the IM effect: %v", err)
	}
	if !strings.Contains(effect.RequestJSON, "回落人工") {
		t.Fatalf("IM body must carry the fallback notice: %s", effect.RequestJSON)
	}
}

// 手动 tick 等入口未传批量时按默认值执行,而不是零值静默短路。
func TestAutoMergeGateNormalizesZeroLimit(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	orch, tasks, artifacts, _ := newGateOrch(t, client)
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 0)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("zero limit must normalize to the default batch, got %#v", decisions)
	}
}
