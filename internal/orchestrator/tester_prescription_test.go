package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	storerepo "sdlc-coordinator/internal/store/repository"
)

func newPrescriptionRepositories(t *testing.T) (*storerepo.TaskRepository, *storerepo.RunRepository, *storerepo.ArtifactRepository) {
	t.Helper()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return storerepo.NewTaskRepository(db), storerepo.NewRunRepository(db), storerepo.NewArtifactRepository(db)
}

func prescriptionTesterTask(taskID, sourceBranch string) *model.Task {
	repositoryID := fixtureRepositoryID
	return &model.Task{
		TaskID: taskID, DedupeKey: "verify:" + taskID, Type: "verify_merge_request", Role: "TesterWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: &repositoryID, Status: model.TaskStatusPending,
		Objective: "verify", AcceptanceCriteria: "evidence",
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"source_branch":"` + sourceBranch + `","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"repos":[{"name":"project","branch":"` + sourceBranch + `","access":"readable"}]}`,
	}
}

func prescriptionDeveloperTask(taskID, deliveryBranch string) *model.Task {
	return &model.Task{
		TaskID: taskID, DedupeKey: "dev:" + taskID, Type: "develop_from_issue", Role: "DeveloperWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", Status: model.TaskStatusSucceeded,
		Objective: "implement", AcceptanceCriteria: "tests pass",
		DeliveryBranch: deliveryBranch,
		ContextJSON:    `{"project":{"path":"group/project"},"repos":[{"name":"project","branch":"master","access":"writable"}]}`,
	}
}

func prescriptionArtifact(taskID, runID string, payload string) *model.Artifact {
	return &model.Artifact{
		ArtifactID:   "artifact-" + taskID + "-" + runID,
		TaskID:       taskID,
		RunID:        runID,
		Kind:         "test_result",
		Name:         "test_results",
		MetadataJSON: payload,
	}
}

func TestBuildVerificationPlanUnionsFloorAndReplay(t *testing.T) {
	ctx := context.Background()
	tasks, runs, artifacts := newPrescriptionRepositories(t)
	orch := New(tasks, runs, WithVerificationArtifactReader(artifacts))
	if err := tasks.Create(ctx, prescriptionTesterTask("task-verify-plan", "alice/issue-13-20260727")); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Create(ctx, prescriptionDeveloperTask("task-dev-plan", "alice/issue-13-20260727")); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.UpsertByIdentity(ctx, prescriptionArtifact("task-dev-plan", "run-dev-plan-1",
		`{"source_key":"test_results","value":[{"command":"go test ./internal/...","passed":true,"summary":"ok"}]}`)); err != nil {
		t.Fatal(err)
	}

	repoRow := &registeredrepo.RegisteredRepository{VerifyCommand: "go test ./..."}
	plan := orch.buildVerificationPlan(ctx, prescriptionTesterTask("task-verify-plan", "alice/issue-13-20260727"), repoRow)
	if plan == nil {
		t.Fatal("plan must exist when a floor is declared")
	}
	if plan.FloorCommand != "go test ./..." {
		t.Fatalf("floor=%q", plan.FloorCommand)
	}
	if len(plan.ReplayCommands) != 1 || plan.ReplayCommands[0] != "go test ./internal/..." {
		t.Fatalf("replay=%#v", plan.ReplayCommands)
	}
}

func TestBuildVerificationPlanDedupesReplayAgainstFloor(t *testing.T) {
	ctx := context.Background()
	tasks, runs, artifacts := newPrescriptionRepositories(t)
	orch := New(tasks, runs, WithVerificationArtifactReader(artifacts))
	if err := tasks.Create(ctx, prescriptionTesterTask("task-verify-dedupe", "alice/issue-13-20260727")); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Create(ctx, prescriptionDeveloperTask("task-dev-dedupe", "alice/issue-13-20260727")); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.UpsertByIdentity(ctx, prescriptionArtifact("task-dev-dedupe", "run-dev-dedupe-1",
		`{"source_key":"test_results","value":[{"command":"go test ./...","passed":true,"summary":"ok"},{"command":"make e2e","passed":true,"summary":"ok"}]}`)); err != nil {
		t.Fatal(err)
	}

	repoRow := &registeredrepo.RegisteredRepository{VerifyCommand: "go test ./..."}
	plan := orch.buildVerificationPlan(ctx, prescriptionTesterTask("task-verify-dedupe", "alice/issue-13-20260727"), repoRow)
	if plan == nil || len(plan.ReplayCommands) != 1 || plan.ReplayCommands[0] != "make e2e" {
		t.Fatalf("replay must dedupe against the floor: plan=%#v", plan)
	}
}

func TestBuildVerificationPlanKeepsFloorWithoutReaderOrHistory(t *testing.T) {
	ctx := context.Background()
	tasks, runs, artifacts := newPrescriptionRepositories(t)

	// 未装配 artifact 读取端口:复放降级为空,下限保留。
	orch := New(tasks, runs)
	repoRow := &registeredrepo.RegisteredRepository{VerifyCommand: "go test ./..."}
	plan := orch.buildVerificationPlan(ctx, prescriptionTesterTask("task-verify-noreader", "alice/issue-13-20260727"), repoRow)
	if plan == nil || plan.FloorCommand != "go test ./..." || len(plan.ReplayCommands) != 0 {
		t.Fatalf("floor must survive without the reader port: plan=%#v", plan)
	}

	// 有端口但无历史记录:同样只保下限。
	orchWithReader := New(tasks, runs, WithVerificationArtifactReader(artifacts))
	plan = orchWithReader.buildVerificationPlan(ctx, prescriptionTesterTask("task-verify-nohistory", "alice/issue-13-20260727"), repoRow)
	if plan == nil || plan.FloorCommand != "go test ./..." || len(plan.ReplayCommands) != 0 {
		t.Fatalf("missing history must degrade to floor only: plan=%#v", plan)
	}
}

func TestBuildVerificationPlanSkipsNonDeveloperCandidates(t *testing.T) {
	ctx := context.Background()
	tasks, runs, artifacts := newPrescriptionRepositories(t)
	orch := New(tasks, runs, WithVerificationArtifactReader(artifacts))
	if err := tasks.Create(ctx, prescriptionTesterTask("task-verify-role", "alice/issue-13-20260727")); err != nil {
		t.Fatal(err)
	}
	// 同 delivery branch 的任务但角色不是 developer:不得充当复放来源。
	impostor := prescriptionDeveloperTask("task-dev-impostor", "alice/issue-13-20260727")
	impostor.Role = "ReviewerWorker"
	if err := tasks.Create(ctx, impostor); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.UpsertByIdentity(ctx, prescriptionArtifact("task-dev-impostor", "run-impostor-1",
		`{"source_key":"test_results","value":[{"command":"evil command","passed":true,"summary":"ok"}]}`)); err != nil {
		t.Fatal(err)
	}

	plan := orch.buildVerificationPlan(ctx, prescriptionTesterTask("task-verify-role", "alice/issue-13-20260727"), &registeredrepo.RegisteredRepository{})
	if plan != nil {
		t.Fatalf("no floor and no developer history must yield no plan: %#v", plan)
	}
}

func TestBuildVerificationPlanUsesLatestDeveloperRun(t *testing.T) {
	ctx := context.Background()
	tasks, runs, artifacts := newPrescriptionRepositories(t)
	orch := New(tasks, runs, WithVerificationArtifactReader(artifacts))
	if err := tasks.Create(ctx, prescriptionTesterTask("task-verify-latest", "alice/issue-13-20260727")); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Create(ctx, prescriptionDeveloperTask("task-dev-latest", "alice/issue-13-20260727")); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.UpsertByIdentity(ctx, prescriptionArtifact("task-dev-latest", "run-dev-latest-1",
		`{"source_key":"test_results","value":[{"command":"go vet ./...","passed":true,"summary":"old round"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.UpsertByIdentity(ctx, prescriptionArtifact("task-dev-latest", "run-dev-latest-2",
		`{"source_key":"test_results","value":[{"command":"go test ./... -count=1","passed":true,"summary":"new round"}]}`)); err != nil {
		t.Fatal(err)
	}

	plan := orch.buildVerificationPlan(ctx, prescriptionTesterTask("task-verify-latest", "alice/issue-13-20260727"), &registeredrepo.RegisteredRepository{})
	if plan == nil || len(plan.ReplayCommands) != 1 || plan.ReplayCommands[0] != "go test ./... -count=1" {
		t.Fatalf("replay must come from the latest recorded round: plan=%#v", plan)
	}
}
