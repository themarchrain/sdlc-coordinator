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
