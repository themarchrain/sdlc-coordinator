package events

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestProcessorCreatesTaskFromGitLabMergeRequestWebhook(t *testing.T) {
	processor, inboundRepo, taskRepo := newProcessorFixture(t)
	ctx := context.Background()

	result, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatalf("process gitlab webhook: %v", err)
	}
	if result.Duplicate {
		t.Fatal("expected first webhook to be processed, got duplicate")
	}
	if result.Ignored {
		t.Fatalf("expected task-created result, got ignored: %s", result.IgnoreReason)
	}
	if result.Event == nil || result.Event.Status != model.InboundEventStatusTaskCreated {
		t.Fatalf("expected task_created inbound event, got %#v", result.Event)
	}
	if result.Event.PayloadJSON == "" {
		t.Fatal("expected raw payload to be stored")
	}
	if result.Event.NormalizedJSON == "" {
		t.Fatal("expected normalized payload to be stored")
	}
	if result.Task == nil {
		t.Fatal("expected created task")
	}
	if result.Task.Type != "review_mr" || result.Task.Role != "ReviewerWorker" {
		t.Fatalf("unexpected task type/role: %s/%s", result.Task.Type, result.Task.Role)
	}

	storedEvent, err := inboundRepo.GetByDedupeKey(ctx, result.Event.DedupeKey)
	if err != nil {
		t.Fatalf("get inbound event: %v", err)
	}
	if storedEvent.Status != model.InboundEventStatusTaskCreated {
		t.Fatalf("expected stored event task_created, got %s", storedEvent.Status)
	}

	storedTask, err := taskRepo.GetByDedupeKey(ctx, "gitlab:mr-review:24207:12:abc123")
	if err != nil {
		t.Fatalf("get task by dedupe key: %v", err)
	}
	if storedTask.TaskID != result.Task.TaskID {
		t.Fatalf("expected task id %s, got %s", result.Task.TaskID, storedTask.TaskID)
	}
}

func TestProcessorIgnoresGitLabMergeRequestMetadataUpdate(t *testing.T) {
	processor, inboundRepo, taskRepo := newProcessorFixture(t)
	ctx := context.Background()

	result, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMRUpdateWithoutOldRevPayload))
	if err != nil {
		t.Fatalf("process gitlab webhook: %v", err)
	}
	if !result.Ignored {
		t.Fatal("expected metadata-only update to be ignored")
	}
	if result.Task != nil {
		t.Fatalf("expected no task, got %#v", result.Task)
	}
	if result.Event == nil || result.Event.Status != model.InboundEventStatusIgnored {
		t.Fatalf("expected ignored inbound event, got %#v", result.Event)
	}
	if result.IgnoreReason == "" || result.Event.ErrorMessage == "" {
		t.Fatalf("expected ignore reason to be recorded: %#v", result)
	}

	events, err := inboundRepo.ListByStatus(ctx, model.InboundEventStatusIgnored, 10)
	if err != nil {
		t.Fatalf("list ignored events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one ignored event, got %d", len(events))
	}

	if _, err := taskRepo.GetByDedupeKey(ctx, "gitlab:mr-review:24207:12:def456"); err == nil {
		t.Fatal("expected no task for metadata-only update")
	}
}

func TestProcessorReturnsExistingInboundEventForDuplicateWebhook(t *testing.T) {
	processor, _, _ := newProcessorFixture(t)
	ctx := context.Background()

	first, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatalf("process first webhook: %v", err)
	}
	second, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatalf("process duplicate webhook: %v", err)
	}

	if !second.Duplicate {
		t.Fatal("expected second webhook to be marked duplicate")
	}
	if first.Event.EventID != second.Event.EventID {
		t.Fatalf("expected duplicate to return event %s, got %s", first.Event.EventID, second.Event.EventID)
	}
	if second.Task != nil {
		t.Fatalf("expected duplicate webhook not to rebuild task response, got %#v", second.Task)
	}
}

func TestProcessorDeduplicatesGitLabWebhookByEventUUID(t *testing.T) {
	processor, inboundRepo, _ := newProcessorFixture(t)
	ctx := context.Background()

	metadata := GitLabWebhookMetadata{
		EventName:   "Merge Request Hook",
		EventUUID:   "event-uuid-1",
		WebhookUUID: "webhook-delivery-1",
		InstanceURL: "https://gitlab.example",
	}
	first, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload), WithGitLabWebhookMetadata(metadata))
	if err != nil {
		t.Fatalf("process first webhook: %v", err)
	}

	secondPayload := strings.Replace(gitLabMROpenPayload, "abc123", "def456", 1)
	second, err := processor.ProcessGitLabWebhook(ctx, []byte(secondPayload), WithGitLabWebhookMetadata(metadata))
	if err != nil {
		t.Fatalf("process duplicate webhook: %v", err)
	}

	if first.Event.DedupeKey != "gitlab:webhook:event_uuid:event-uuid-1" {
		t.Fatalf("unexpected event uuid dedupe key: %s", first.Event.DedupeKey)
	}
	if !second.Duplicate {
		t.Fatal("expected second webhook with same event uuid to be duplicate")
	}
	if second.Task != nil {
		t.Fatalf("expected duplicate webhook not to create second task, got %#v", second.Task)
	}

	stored, err := inboundRepo.GetByDedupeKey(ctx, "gitlab:webhook:event_uuid:event-uuid-1")
	if err != nil {
		t.Fatalf("get inbound event by event uuid: %v", err)
	}
	if stored.EventID != first.Event.EventID {
		t.Fatalf("expected stored event %s, got %s", first.Event.EventID, stored.EventID)
	}
}

func TestProcessorResumesReceivedGitLabReceipt(t *testing.T) {
	processor, inboundRepo, _ := newProcessorFixture(t)
	ctx := context.Background()
	metadata := GitLabWebhookMetadata{EventUUID: "event-received-resume"}
	receipt := newGitLabReceipt([]byte(gitLabMROpenPayload), metadata)
	inbound := &model.InboundEvent{
		DedupeKey:   receipt.DedupeKey,
		Source:      "gitlab",
		EventType:   receipt.EventType,
		EventAction: receipt.EventAction,
		ExternalID:  receipt.ExternalID,
		Status:      model.InboundEventStatusReceived,
		PayloadJSON: gitLabMROpenPayload,
	}
	if err := inboundRepo.Create(ctx, inbound); err != nil {
		t.Fatalf("seed received inbound event: %v", err)
	}

	result, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload), WithGitLabWebhookMetadata(metadata))
	if err != nil {
		t.Fatalf("resume received webhook: %v", err)
	}
	if !result.Duplicate {
		t.Fatal("expected resumed receipt to remain marked duplicate")
	}
	if result.Task == nil {
		t.Fatal("expected resumed receipt to create a task")
	}
	if result.Event.Status != model.InboundEventStatusTaskCreated {
		t.Fatalf("expected task_created receipt, got %s", result.Event.Status)
	}
}

func TestProcessorResumesNormalizedGitLabReceiptFromStoredPayload(t *testing.T) {
	processor, inboundRepo, _ := newProcessorFixture(t)
	ctx := context.Background()
	metadata := GitLabWebhookMetadata{EventUUID: "event-normalized-resume"}
	receipt := newGitLabReceipt([]byte(gitLabMROpenPayload), metadata)
	normalization, err := NewGitLabNormalizer().Normalize([]byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatalf("normalize fixture: %v", err)
	}
	normalizedJSON, err := json.Marshal(normalization.Event)
	if err != nil {
		t.Fatalf("marshal normalized fixture: %v", err)
	}
	inbound := &model.InboundEvent{
		DedupeKey:      receipt.DedupeKey,
		Source:         "gitlab",
		EventType:      receipt.EventType,
		EventAction:    receipt.EventAction,
		ExternalID:     receipt.ExternalID,
		Status:         model.InboundEventStatusNormalized,
		PayloadJSON:    gitLabMROpenPayload,
		NormalizedJSON: string(normalizedJSON),
	}
	if err := inboundRepo.Create(ctx, inbound); err != nil {
		t.Fatalf("seed normalized inbound event: %v", err)
	}

	result, err := processor.ProcessGitLabWebhook(ctx, []byte(`{"ignored":"replayed body"}`), WithGitLabWebhookMetadata(metadata))
	if err != nil {
		t.Fatalf("resume normalized webhook: %v", err)
	}
	if !result.Duplicate || result.Task == nil {
		t.Fatalf("expected duplicate receipt to resume with task, got %#v", result)
	}
	if result.Event.Status != model.InboundEventStatusTaskCreated {
		t.Fatalf("expected task_created receipt, got %s", result.Event.Status)
	}
}

func TestProcessorDifferentEventUUIDsReuseBusinessTask(t *testing.T) {
	processor, _, _ := newProcessorFixture(t)
	ctx := context.Background()

	first, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload), WithGitLabWebhookMetadata(GitLabWebhookMetadata{EventUUID: "delivery-1"}))
	if err != nil {
		t.Fatalf("process first delivery: %v", err)
	}
	second, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload), WithGitLabWebhookMetadata(GitLabWebhookMetadata{EventUUID: "delivery-2"}))
	if err != nil {
		t.Fatalf("process second delivery: %v", err)
	}

	if first.Event.EventID == second.Event.EventID {
		t.Fatal("expected distinct receipt audit rows")
	}
	if first.Task == nil || second.Task == nil || first.Task.TaskID != second.Task.TaskID {
		t.Fatalf("expected both receipts to reuse one business task, got %#v and %#v", first.Task, second.Task)
	}
}

func TestProcessorResumesHumanRequiredDeveloperTaskFromIssueNote(t *testing.T) {
	processor, _, taskRepo := newProcessorFixture(t)
	ctx := context.Background()

	created, err := processor.ProcessGitLabWebhook(ctx, readGitLabFixture(t, "issue_opened_developer_agent.json"))
	if err != nil {
		t.Fatalf("create developer task: %v", err)
	}
	if created.Task == nil {
		t.Fatal("expected developer task")
	}

	task := created.Task
	task.Status = model.TaskStatusHumanRequired
	task.LastError = "need API field definition"
	if err := taskRepo.Update(ctx, task); err != nil {
		t.Fatalf("mark task human_required: %v", err)
	}

	resumed, err := processor.ProcessGitLabWebhook(ctx, readGitLabFixture(t, "issue_resume_note_created.json"))
	if err != nil {
		t.Fatalf("resume developer task: %v", err)
	}
	if resumed.Ignored || resumed.Task == nil {
		t.Fatalf("expected resumed task, got %#v", resumed)
	}
	if resumed.Task.TaskID != task.TaskID || resumed.Task.Status != model.TaskStatusPending {
		t.Fatalf("expected task %s pending, got %#v", task.TaskID, resumed.Task)
	}
	if !strings.Contains(resumed.Task.ContextJSON, "接口字段为 status") {
		t.Fatalf("expected resume context in task package: %s", resumed.Task.ContextJSON)
	}

	stored, err := taskRepo.GetByDedupeKey(ctx, task.DedupeKey)
	if err != nil {
		t.Fatalf("load resumed task: %v", err)
	}
	if stored.TaskID != task.TaskID {
		t.Fatalf("expected existing task id %s, got %s", task.TaskID, stored.TaskID)
	}

	secondResume, err := processor.ProcessGitLabWebhook(
		ctx,
		readGitLabFixture(t, "issue_resume_note_created.json"),
		WithGitLabWebhookMetadata(GitLabWebhookMetadata{EventUUID: "resume-after-pending"}),
	)
	if err != nil {
		t.Fatalf("process duplicate resume: %v", err)
	}
	if !secondResume.Ignored || secondResume.Task != nil {
		t.Fatalf("expected pending task resume to be ignored, got %#v", secondResume)
	}
}

func TestProcessorRoutesMergeRequestReworkRequest(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	handler := &capturingMergeRequestReworkHandler{}
	processor := NewProcessor(repository.NewInboundEventRepository(db), handler)

	result, err := processor.ProcessGitLabWebhook(context.Background(), readGitLabFixture(t, "merge_request_resume_note_created.json"))
	if err != nil {
		t.Fatalf("process MR rework note: %v", err)
	}
	if result.Ignored || result.Task == nil || result.Task.Type != model.TaskTypeReworkMergeRequest {
		t.Fatalf("expected routed rework task, got %#v", result)
	}
	request := handler.reworkRequest
	if request.ProjectID != "1001" || request.MergeRequestIID != 21 || request.NoteID != 20001 {
		t.Fatalf("unexpected routed identity: %#v", request)
	}
	if request.SourceBranch != "developer/issue-19-20260806" || request.ExpectedHeadSHA != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected routed branch facts: %#v", request)
	}
	if request.ResumeContext != "请修正空值处理并补充对应测试。" || request.RequesterID != 42 {
		t.Fatalf("unexpected routed request context: %#v", request)
	}
}

func TestProcessorAppliesGitLabReworkRequesterAllowlist(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	handler := &capturingMergeRequestReworkHandler{}
	processor := NewProcessor(
		repository.NewInboundEventRepository(db),
		handler,
	)
	processor.SetGitLabReworkLookup(func(context.Context) []int { return []int{99} })
	raw := readGitLabFixture(t, "merge_request_resume_note_created.json")
	raw = []byte(strings.Replace(string(raw), `"id": 42`, `"id": 99`, 1))

	result, err := processor.ProcessGitLabWebhook(context.Background(), raw)
	if err != nil {
		t.Fatalf("process allowlisted rework comment: %v", err)
	}
	if result.Ignored || handler.reworkRequest.RequesterID != 99 {
		t.Fatalf("expected allowlisted requester to reach rework handler: %#v / %#v", result, handler.reworkRequest)
	}
}

func TestProcessorIgnoresUnauthorizedMergeRequestRework(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	handler := &rejectingMergeRequestReworkHandler{}
	processor := NewProcessor(repository.NewInboundEventRepository(db), handler)
	raw := []byte(strings.Replace(string(readGitLabFixture(t, "merge_request_resume_note_created.json")), `"id": 42`, `"id": 99`, 1))

	result, err := processor.ProcessGitLabWebhook(context.Background(), raw)
	if err != nil {
		t.Fatalf("process unauthorized rework comment: %v", err)
	}
	if !result.Ignored || result.IgnoreReason != model.ErrMergeRequestReworkRequesterUnauthorized.Error() {
		t.Fatalf("expected ignored unauthorized event, got %#v", result)
	}
}

func TestProcessorRoutesMergeRequestLifecycle(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	handler := &capturingMergeRequestReworkHandler{}
	processor := NewProcessor(repository.NewInboundEventRepository(db), handler)

	result, err := processor.ProcessGitLabWebhook(context.Background(), []byte(gitLabMRClosedPayload))
	if err != nil {
		t.Fatalf("process MR lifecycle: %v", err)
	}
	if result.Ignored || result.Task == nil || result.Task.Status != model.TaskStatusBlocked {
		t.Fatalf("expected routed lifecycle task, got %#v", result)
	}
	request := handler.lifecycleRequest
	if request.DedupeKey != "gitlab:mr-rework:24207:12" || request.MergeRequestIID != 12 || request.Action != "close" || request.State != "closed" {
		t.Fatalf("unexpected routed lifecycle request: %#v", request)
	}
}

type capturingMergeRequestReworkHandler struct {
	reworkRequest    model.MergeRequestReworkRequest
	lifecycleRequest model.MergeRequestLifecycleRequest
}

type rejectingMergeRequestReworkHandler struct{}

func (h *rejectingMergeRequestReworkHandler) ApplyMergeRequestLifecycle(_ context.Context, _ model.MergeRequestLifecycleRequest) (*model.Task, error) {
	return nil, nil
}

func (h *rejectingMergeRequestReworkHandler) CreateTask(_ context.Context, task *model.Task) (*model.Task, error) {
	return task, nil
}

func (h *rejectingMergeRequestReworkHandler) StartOrResumeMergeRequestRework(_ context.Context, _ model.MergeRequestReworkRequest) (*model.Task, error) {
	return nil, model.ErrMergeRequestReworkRequesterUnauthorized
}

func (h *capturingMergeRequestReworkHandler) ApplyMergeRequestLifecycle(_ context.Context, request model.MergeRequestLifecycleRequest) (*model.Task, error) {
	h.lifecycleRequest = request
	return &model.Task{TaskID: "task-rework", DedupeKey: request.DedupeKey, Type: model.TaskTypeReworkMergeRequest, Role: "DeveloperWorker", Status: model.TaskStatusBlocked}, nil
}

func (h *capturingMergeRequestReworkHandler) CreateTask(_ context.Context, task *model.Task) (*model.Task, error) {
	return task, nil
}

func (h *capturingMergeRequestReworkHandler) StartOrResumeMergeRequestRework(_ context.Context, request model.MergeRequestReworkRequest) (*model.Task, error) {
	h.reworkRequest = request
	return &model.Task{TaskID: "task-rework", DedupeKey: request.DedupeKey, Type: model.TaskTypeReworkMergeRequest, Role: "DeveloperWorker", Status: model.TaskStatusPending}, nil
}

type receiptCreateBarrierStore struct {
	InboundEventStore
	arrived chan struct{}
	release chan struct{}
}

func (s *receiptCreateBarrierStore) Create(ctx context.Context, event *model.InboundEvent) error {
	s.arrived <- struct{}{}
	<-s.release
	return s.InboundEventStore.Create(ctx, event)
}

type idempotentTaskCreator struct {
	mu    sync.Mutex
	task  *model.Task
	calls int
}

func (c *idempotentTaskCreator) CreateTask(_ context.Context, task *model.Task) (*model.Task, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.task == nil {
		copy := *task
		c.task = &copy
	}
	copy := *c.task
	return &copy, nil
}

func TestProcessorConcurrentSameEventUUIDReusesReceipt(t *testing.T) {
	db, err := store.Open("file:" + model.NewID("receipt_fault") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	baseEvents := repository.NewInboundEventRepository(db)
	events := &receiptCreateBarrierStore{
		InboundEventStore: baseEvents,
		arrived:           make(chan struct{}, 2),
		release:           make(chan struct{}),
	}
	tasks := &idempotentTaskCreator{}
	processor := NewProcessor(events, tasks)
	metadata := WithGitLabWebhookMetadata(GitLabWebhookMetadata{EventUUID: "same-concurrent-delivery"})

	results := make(chan *ProcessResult, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			result, err := processor.ProcessGitLabWebhook(context.Background(), []byte(gitLabMROpenPayload), metadata)
			results <- result
			errs <- err
		}()
	}
	<-events.arrived
	<-events.arrived
	close(events.release)

	first, second := <-results, <-results
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent webhook returned error: %v", err)
		}
	}
	if first == nil || second == nil || first.Event.EventID != second.Event.EventID {
		t.Fatalf("expected one shared receipt, got %#v and %#v", first, second)
	}
	eventsFound, err := baseEvents.ListByStatus(context.Background(), model.InboundEventStatusTaskCreated, 10)
	if err != nil {
		t.Fatalf("list task-created receipts: %v", err)
	}
	if len(eventsFound) != 1 {
		t.Fatalf("expected one completed receipt, got %d", len(eventsFound))
	}
}

func newProcessorFixture(t *testing.T) (*Processor, *repository.InboundEventRepository, *repository.TaskRepository) {
	t.Helper()

	databaseName := strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(t.Name())
	db, err := store.Open("file:" + databaseName + "_" + model.NewID("testdb") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	inboundRepo := repository.NewInboundEventRepository(db)
	orch := orchestrator.New(taskRepo, runRepo)

	return NewProcessor(inboundRepo, orch), inboundRepo, taskRepo
}

func newProcessorFixtureWithNormalizerOptions(t *testing.T, opts ...GitLabNormalizerOption) (*Processor, *repository.InboundEventRepository, *repository.TaskRepository) {
	t.Helper()

	databaseName := strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(t.Name())
	db, err := store.Open("file:" + databaseName + "_" + model.NewID("testdb") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	inboundRepo := repository.NewInboundEventRepository(db)
	orch := orchestrator.New(taskRepo, runRepo)

	return NewProcessor(inboundRepo, orch, opts...), inboundRepo, taskRepo
}

type mutableRepositoryLookup struct {
	repositories map[string]*registeredrepo.RegisteredRepository
}

func (l *mutableRepositoryLookup) GetByProjectID(_ context.Context, provider, projectID string) (*registeredrepo.RegisteredRepository, error) {
	if repo, ok := l.repositories[projectID]; ok && strings.EqualFold(repo.Provider, provider) {
		return repo, nil
	}
	return nil, repository.ErrNotFound
}

func TestProcessorDefersUnenabledEventThenReplaysAfterEnable(t *testing.T) {
	processor, inboundRepo, taskRepo := newProcessorFixture(t)
	ctx := context.Background()

	lookup := &mutableRepositoryLookup{repositories: map[string]*registeredrepo.RegisteredRepository{
		"24207": {RepositoryID: "repo-mr", Provider: "gitlab", PathWithNamespace: "group/project", Status: registeredrepo.StatusConfiguring},
	}}
	processor.SetRepositoryLookup(lookup)

	first, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatalf("process first webhook: %v", err)
	}
	if first.Ignored {
		t.Fatalf("expected deferred, not ignored: %s", first.IgnoreReason)
	}
	if first.Event == nil || first.Event.Status != model.InboundEventStatusDeferred {
		t.Fatalf("expected deferred event, got %#v", first.Event)
	}
	if first.Task != nil {
		t.Fatalf("expected no task while repository unenabled, got %#v", first.Task)
	}

	lookup.repositories["24207"] = &registeredrepo.RegisteredRepository{RepositoryID: "repo-mr", Provider: "gitlab", PathWithNamespace: "group/project", Status: registeredrepo.StatusEnabled}
	replayed, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatalf("process replayed webhook: %v", err)
	}
	if replayed.Event == nil || replayed.Event.Status != model.InboundEventStatusTaskCreated {
		t.Fatalf("expected task_created after replay, got %#v", replayed.Event)
	}
	if replayed.Task == nil {
		t.Fatal("expected task after replay")
	}

	again, err := processor.ProcessGitLabWebhook(ctx, []byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatalf("process third webhook: %v", err)
	}
	if !again.Duplicate {
		t.Fatal("expected duplicate on third replay")
	}
	stored, err := taskRepo.GetByDedupeKey(ctx, "gitlab:mr-review:24207:12:abc123")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.TaskID != replayed.Task.TaskID {
		t.Fatalf("expected replayed task %s to be the only one, got %s", replayed.Task.TaskID, stored.TaskID)
	}
	storedEvent, err := inboundRepo.GetByDedupeKey(ctx, first.Event.DedupeKey)
	if err != nil {
		t.Fatalf("get stored event: %v", err)
	}
	if storedEvent.Status != model.InboundEventStatusTaskCreated {
		t.Fatalf("expected stored event task_created, got %s", storedEvent.Status)
	}
}

// TestProcessorGitLabAllowlistLookupHotReloads 锁定:GitLab 白名单经查找函数
// 供给,组织段配置变更后同处理器的下一个事件即按新白名单判定。
func TestProcessorGitLabAllowlistLookupHotReloads(t *testing.T) {
	allowed := []int{77}
	processor, _, taskRepo := newProcessorFixtureWithNormalizerOptions(t)
	processor.SetGitLabReworkLookup(func(context.Context) []int { return allowed })
	ctx := context.Background()

	created, err := processor.ProcessGitLabWebhook(ctx, readGitLabFixture(t, "issue_opened_developer_agent.json"))
	if err != nil {
		t.Fatalf("create developer task: %v", err)
	}
	task := created.Task
	if task == nil {
		t.Fatal("expected developer task")
	}
	task.Status = model.TaskStatusHumanRequired
	if err := taskRepo.Update(ctx, task); err != nil {
		t.Fatalf("mark task human_required: %v", err)
	}

	stranger := []byte(strings.Replace(string(readGitLabFixture(t, "issue_resume_note_created.json")), `"id": 42`, `"id": 77`, 1))
	resumed, err := processor.ProcessGitLabWebhook(ctx, stranger, WithGitLabWebhookMetadata(GitLabWebhookMetadata{EventUUID: "hot-allow-1"}))
	if err != nil {
		t.Fatalf("resume with allowlisted stranger: %v", err)
	}
	if resumed.Ignored || resumed.Task == nil {
		t.Fatalf("allowlisted stranger must resume task, got %#v", resumed)
	}
	task.Status = model.TaskStatusHumanRequired
	if err := taskRepo.Update(ctx, task); err != nil {
		t.Fatalf("re-mark task human_required: %v", err)
	}

	allowed = []int{}
	denied, err := processor.ProcessGitLabWebhook(ctx, stranger, WithGitLabWebhookMetadata(GitLabWebhookMetadata{EventUUID: "hot-allow-2"}))
	if err != nil {
		t.Fatalf("resume after allowlist removal: %v", err)
	}
	if !denied.Ignored || denied.Task != nil {
		t.Fatalf("stranger outside allowlist must be ignored, got %#v", denied)
	}
}
