package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	storerepo "sdlc-coordinator/internal/store/repository"
)

func newTesterResultRepositories(t *testing.T) (*storerepo.TaskRepository, *storerepo.RunRepository, *storerepo.ArtifactRepository, *storerepo.WritebackRepository) {
	t.Helper()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Writeback{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return storerepo.NewTaskRepository(db), storerepo.NewRunRepository(db), storerepo.NewArtifactRepository(db), storerepo.NewWritebackRepository(db)
}

func testerApplicationFixture(taskID, runID string) (*model.Task, *model.Run) {
	currentRunID := runID
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID: taskID, DedupeKey: "verify:" + taskID, Type: "verify_merge_request", Role: "TesterWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: &repositoryID,
		Status: model.TaskStatusRunning, CurrentRunID: &currentRunID,
		Objective: "verify MR !12 at head abc123", AcceptanceCriteria: "report real evidence",
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":12,"head_sha":"abc123","source_branch":"alice/issue-13-20260727"},"repos":[{"name":"project","branch":"alice/issue-13-20260727","access":"readable"}]}`,
	}
	run := &model.Run{RunID: runID, TaskID: taskID, WorkerRole: "TesterWorker", Status: model.RunStatusRunning}
	return task, run
}

func testerCanonicalResult(t *testing.T, task *model.Task, run *model.Run, raw string) results.CanonicalResult {
	t.Helper()
	contract := results.NewTesterContract()
	input, err := contract.DecodeStrict(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	if err := contract.Validate(task, run, input); err != nil {
		t.Fatalf("validate tester result: %v", err)
	}
	canonical, err := contract.Normalize(task, run, input)
	if err != nil {
		t.Fatalf("normalize tester result: %v", err)
	}
	return canonical
}

func TestTesterVerificationResultLandsEvidenceAndReportWriteback(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newTesterResultRepositories(t)
	orch := New(taskRepo, runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(&staticCodeHostResolver{backends: map[string]codehost.Backend{fixtureRepositoryID: &reviewerRunnerCodeHostBackend{}}}),
	)
	task, run := testerApplicationFixture("task-tester-apply", "run-tester-apply")
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatal(err)
	}

	canonical := testerCanonicalResult(t, task, run, `{
		"task_id":"task-tester-apply","run_id":"run-tester-apply","outcome":"verification_passed",
		"summary":"floor and replay commands are green",
		"tests":[{"command":"go test ./...","passed":true,"summary":"ok","attempts":2}]
	}`)
	appliedTask, appliedRun, err := orch.ApplyCanonicalResult(ctx, canonical)
	if err != nil {
		t.Fatalf("apply tester result: %v", err)
	}
	if appliedTask.Status != model.TaskStatusSucceeded || appliedRun.Status != model.RunStatusSucceeded {
		t.Fatalf("verification application must complete the task: task=%s run=%s", appliedTask.Status, appliedRun.Status)
	}

	artifacts, err := artifactRepo.ListByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]model.Artifact{}
	for _, artifact := range artifacts {
		byName[artifact.Name] = artifact
	}
	for _, name := range []string{"test_results", "verification_verdict", "verification_head_sha"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("evidence artifact %s is missing: %#v", name, artifacts)
		}
	}
	if !strings.Contains(byName["verification_head_sha"].MetadataJSON, "abc123") {
		t.Fatalf("head sha binding must come from the task context: %s", byName["verification_head_sha"].MetadataJSON)
	}

	writebacks, err := writebackRepo.ListByRunID(ctx, run.RunID)
	if err != nil || len(writebacks) != 1 {
		t.Fatalf("expected one report writeback, got %d err=%v", len(writebacks), err)
	}
	if writebacks[0].Kind != CodeHostWritebackCreateMergeRequestNote || writebacks[0].TargetType != "merge_request" || writebacks[0].TargetIID != 12 {
		t.Fatalf("unexpected writeback identity: %#v", writebacks[0])
	}
}

func TestTesterFailedVerdictStillCompletesTaskWithRedEvidence(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newTesterResultRepositories(t)
	orch := New(taskRepo, runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(&staticCodeHostResolver{backends: map[string]codehost.Backend{fixtureRepositoryID: &reviewerRunnerCodeHostBackend{}}}),
	)
	task, run := testerApplicationFixture("task-tester-red", "run-tester-red")
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatal(err)
	}

	canonical := testerCanonicalResult(t, task, run, `{
		"task_id":"task-tester-red","run_id":"run-tester-red","outcome":"verification_failed",
		"summary":"one command stays red",
		"tests":[{"command":"make e2e","passed":false,"summary":"timeout on every attempt","attempts":3}]
	}`)
	appliedTask, appliedRun, err := orch.ApplyCanonicalResult(ctx, canonical)
	if err != nil {
		t.Fatalf("apply tester result: %v", err)
	}
	if appliedTask.Status != model.TaskStatusSucceeded || appliedRun.Status != model.RunStatusSucceeded {
		t.Fatalf("a red verdict still completes the verification task: task=%s run=%s", appliedTask.Status, appliedRun.Status)
	}
	artifacts, err := artifactRepo.ListByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	verdictFound := false
	for _, artifact := range artifacts {
		if artifact.Name == "verification_verdict" && strings.Contains(artifact.MetadataJSON, "verification_failed") {
			verdictFound = true
		}
	}
	if !verdictFound {
		t.Fatalf("failed verdict must be recorded as evidence: %#v", artifacts)
	}
}

// insufficient_context 停靠人工必须有 IM 通知物化:否则整条信任链在
// 无证据出口静默消失,人工侧无触点(健壮性审查修复轮引入)。
func TestTesterInsufficientContextMaterializesHumanRequiredIM(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, _, writebackRepo := newTesterResultRepositories(t)
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatalf("migrate effects: %v", err)
	}
	effectRepo := storerepo.NewEffectRepository(db)
	orch := New(taskRepo, runRepo,
		WithWritebackRepository(writebackRepo),
		WithEffectRepository(effectRepo),
	)
	task, run := testerApplicationFixture("task-tester-unclear", "run-tester-unclear")
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatal(err)
	}

	canonical := testerCanonicalResult(t, task, run, `{
		"task_id":"task-tester-unclear","run_id":"run-tester-unclear","outcome":"insufficient_context",
		"summary":"no verification commands available","blocker_reason":"repo declares no floor and no replay source"
	}`)
	appliedTask, _, err := orch.ApplyCanonicalResult(ctx, canonical)
	if err != nil {
		t.Fatalf("apply tester result: %v", err)
	}
	if appliedTask.Status != model.TaskStatusHumanRequired {
		t.Fatalf("insufficient context must park for human, got %s", appliedTask.Status)
	}
	if _, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:task-tester-unclear:run-tester-unclear"); err != nil {
		t.Fatalf("human required IM effect must be materialized: %v", err)
	}
}

// 重试预算耗尽的 environment_blocker 是无证据出口的最后落点:必须停靠
// blocked 且有 IM 触点,静默死亡即整条信任链消失。
func TestTesterEnvironmentBlockerExhaustionParksBlockedWithIM(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, _, writebackRepo := newTesterResultRepositories(t)
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatalf("migrate effects: %v", err)
	}
	effectRepo := storerepo.NewEffectRepository(db)
	orch := New(taskRepo, runRepo,
		WithWritebackRepository(writebackRepo),
		WithEffectRepository(effectRepo),
	)
	task, run := testerApplicationFixture("task-tester-exhaust", "run-tester-exhaust-1")
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatal(err)
	}

	canonical := testerCanonicalResult(t, task, run, `{
		"task_id":"task-tester-exhaust","run_id":"run-tester-exhaust-1","outcome":"environment_blocker",
		"summary":"runner cannot start","blocker_reason":"docker daemon unreachable"
	}`)
	appliedTask, _, err := orch.ApplyCanonicalResult(ctx, canonical)
	if err != nil {
		t.Fatalf("apply tester result: %v", err)
	}
	if appliedTask.Status != model.TaskStatusRetryPending {
		t.Fatalf("budget not exhausted must enter the retry path, got %s", appliedTask.Status)
	}

	// 模拟两次重派再败后的预算耗尽:任务经重派回到 running、计数推过预算,
	// 再走一轮结果应用(真实流里失败应用发生在 running 态)。
	stored, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = model.TaskStatusRunning
	stored.AttemptCount = 99
	nextRunID := "run-tester-exhaust-2"
	stored.CurrentRunID = &nextRunID
	if err := taskRepo.Update(ctx, stored); err != nil {
		t.Fatal(err)
	}
	nextRun := &model.Run{RunID: nextRunID, TaskID: task.TaskID, WorkerRole: "TesterWorker", Status: model.RunStatusRunning}
	if err := runRepo.Create(ctx, nextRun); err != nil {
		t.Fatal(err)
	}
	retryCanonical := testerCanonicalResult(t, stored, nextRun, `{
		"task_id":"task-tester-exhaust","run_id":"run-tester-exhaust-2","outcome":"environment_blocker",
		"summary":"runner still cannot start","blocker_reason":"docker daemon unreachable"
	}`)
	finalTask, _, err := orch.ApplyCanonicalResult(ctx, retryCanonical)
	if err != nil {
		t.Fatalf("apply exhausted tester result: %v", err)
	}
	if finalTask.Status != model.TaskStatusBlocked {
		t.Fatalf("exhausted environment blocker must park blocked, got %s", finalTask.Status)
	}
	if _, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:task-tester-exhaust:run-tester-exhaust-2"); err != nil {
		t.Fatalf("exhausted parking must materialize the IM effect: %v", err)
	}
}
