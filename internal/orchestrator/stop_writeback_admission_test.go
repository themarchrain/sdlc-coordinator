package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestStopBeforeWritebackAdmission(t *testing.T) {
	for _, kind := range []string{CodeHostWritebackCreateIssue, CodeHostWritebackCreateMergeRequest} {
		t.Run(kind, func(t *testing.T) {
			db, orch, task, run := externalAdmissionFixture(t)
			repositoryID := fixtureRepositoryID
			if err := db.Model(task).Update("repository_id", repositoryID).Error; err != nil {
				t.Fatal(err)
			}
			backend := &fakeGitLabWritebackBackend{}
			orch.codeHostResolver = backendResolverFor(repositoryID, backend)
			writebacks, err := BuildCodeHostWritebacksFromResult(results.CanonicalResult{TaskID: task.TaskID, RunID: run.RunID, Writebacks: []results.CodeHostWritebackIntent{{Kind: kind, ProjectID: "group/framework", Title: "title", SourceBranch: "feature", TargetBranch: "main"}}})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if _, err := orch.StopTask(ctx, task.TaskID, StopActor{UserID: "admin"}, ""); err != nil {
				t.Fatal(err)
			}
			if err := orch.applyOneCodeHostWriteback(ctx, &writebacks[0]); err != nil && !errors.Is(err, repository.ErrTaskInterrupted) {
				t.Fatal(err)
			}
			stored, _ := orch.writebacks.GetByDedupeKey(ctx, writebacks[0].DedupeKey)
			if backend.createIssueCalls != 0 || backend.createMergeRequestCalls != 0 || stored.Status != model.WritebackStatusSkipped || stored.ErrorMessage != "task_interrupted" {
				t.Fatalf("stopped writeback sent: issue=%d mr=%d record=%+v", backend.createIssueCalls, backend.createMergeRequestCalls, stored)
			}
		})
	}
}

func TestStoppedUnknownAbsentDoesNotRequeue(t *testing.T) {
	db, orch, task, run := externalAdmissionFixture(t)
	repositoryID := fixtureRepositoryID
	if err := db.Model(task).Update("repository_id", repositoryID).Error; err != nil {
		t.Fatal(err)
	}
	backend := &fakeGitLabWritebackBackend{reconcileResult: codehost.ReconcileResult{State: codehost.ReconcileStateAbsent}}
	orch.codeHostResolver = backendResolverFor(repositoryID, backend)
	writeback := &model.Writeback{TaskID: task.TaskID, RunID: run.RunID, DedupeKey: "unknown-writeback", Kind: CodeHostWritebackCreateIssue, ProjectID: "group/framework", Status: model.WritebackStatusUnknown, RequestJSON: `{"kind":"create_issue","project_id":"group/framework","title":"title"}`, OperationMarker: CodeHostOperationMarker("unknown-writeback")}
	if err := db.Create(writeback).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := orch.StopTask(ctx, task.TaskID, StopActor{UserID: "admin"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := orch.ReconcileUnknownWritebacks(ctx, 20); err != nil {
		t.Fatal(err)
	}
	stored, _ := orch.writebacks.GetByDedupeKey(ctx, writeback.DedupeKey)
	if stored.Status != model.WritebackStatusSkipped || backend.reconcileCalls != 1 || backend.createIssueCalls != 0 {
		t.Fatalf("stopped absent outcome requeued: record=%+v reads=%d sends=%d", stored, backend.reconcileCalls, backend.createIssueCalls)
	}
}

func TestAdmissionConfirmedRejectionCanRetryWriteback(t *testing.T) {
	db, orch, task, run := externalAdmissionFixture(t)
	repositoryID := fixtureRepositoryID
	if err := db.Model(task).Update("repository_id", repositoryID).Error; err != nil {
		t.Fatal(err)
	}
	backend := &fakeGitLabWritebackBackend{err: codehost.NewTransientRejection(errors.New("HTTP 429 not applied"))}
	orch.codeHostResolver = backendResolverFor(repositoryID, backend)
	writebacks, err := BuildCodeHostWritebacksFromResult(results.CanonicalResult{TaskID: task.TaskID, RunID: run.RunID, Writebacks: []results.CodeHostWritebackIntent{{Kind: CodeHostWritebackCreateIssue, ProjectID: "group/framework", Title: "title"}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := orch.applyOneCodeHostWriteback(ctx, &writebacks[0]); !codehost.IsTransientRejection(err) {
		t.Fatalf("expected confirmed non-delivery, got %v", err)
	}
	backend.err = nil
	if err := orch.applyOneCodeHostWriteback(ctx, &writebacks[0]); err != nil {
		t.Fatalf("known non-delivery cannot retry: %v", err)
	}
	if backend.createIssueCalls != 2 {
		t.Fatalf("expected rejected call then successful retry, got %d", backend.createIssueCalls)
	}
}

func TestStoppedAutoMergeDoesNotCallRemote(t *testing.T) {
	db, orch, _, _ := externalAdmissionFixture(t)
	if err := store.Migrate(db, &model.MergeGateAudit{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tasks := repository.NewTaskRepository(db)
	artifacts := repository.NewArtifactRepository(db)
	verifier := gateFixture(t, ctx, tasks, artifacts)
	runID := "verify-run"
	if err := db.Create(&model.Run{RunID: runID, TaskID: verifier.TaskID, WorkerRole: verifier.Role, Status: model.RunStatusSucceeded}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(verifier).Update("current_run_id", runID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StopRequest{StopID: "stop-gate", DedupeKey: "task:" + verifier.TaskID, Scope: model.StopScopeTask, TaskID: verifier.TaskID, RunID: runID, RequestedBy: "admin", RequestedAt: time.Now(), ReasonCode: "manual_stop", DeliveryStatus: model.StopDeliveryPending}).Error; err != nil {
		t.Fatal(err)
	}
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA}
	orch.mergeAudits = repository.NewMergeGateAuditRepository(db)
	orch.verificationArtifacts = artifacts
	orch.autoMergeEnabled = func(context.Context) bool { return true }
	orch.repositoryLookup = &gateRepositoryLookup{repo: &registeredrepo.RegisteredRepository{RepositoryID: fixtureRepositoryID, Provider: "github", PathWithNamespace: "group/project", Status: registeredrepo.StatusEnabled}}
	orch.mergeGateClients = func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error) {
		return client, client, nil
	}
	if _, err := orch.RunAutoMergeGate(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if client.mergeCalls != 0 {
		t.Fatalf("stopped task merged remotely %d times", client.mergeCalls)
	}
	var notifications int64
	if err := db.Model(&model.Effect{}).Count(&notifications).Error; err != nil || notifications != 0 {
		t.Fatalf("stop generated fallback notification: %d err=%v", notifications, err)
	}
}

func TestUnknownWritebackCannotBorrowReplayException(t *testing.T) {
	db, orch, task, run := externalAdmissionFixture(t)
	repositoryID := fixtureRepositoryID
	if err := db.Model(task).Update("repository_id", repositoryID).Error; err != nil {
		t.Fatal(err)
	}
	backend := &fakeGitLabWritebackBackend{reconcileResult: codehost.ReconcileResult{State: codehost.ReconcileStateAbsent}}
	orch.codeHostResolver = backendResolverFor(repositoryID, backend)
	writeback := &model.Writeback{TaskID: task.TaskID, RunID: run.RunID, DedupeKey: "previous-send", Kind: CodeHostWritebackCreateIssue, ProjectID: "group/framework", Status: model.WritebackStatusUnknown, RequestJSON: `{"kind":"create_issue","project_id":"group/framework","title":"title"}`, OperationMarker: CodeHostOperationMarker("previous-send")}
	if err := db.Create(writeback).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := orch.ReconcileUnknownWritebacks(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	stored, _ := orch.writebacks.GetByDedupeKey(context.Background(), writeback.DedupeKey)
	if stored.Status != model.WritebackStatusUnknown {
		t.Fatalf("absence observation reactivated unknown send: %+v", stored)
	}
	if err := orch.applyOneCodeHostWriteback(context.Background(), stored); !errors.Is(err, ErrWritebackRequiresReconciliation) || backend.createIssueCalls != 0 {
		t.Fatalf("unknown outcome resent: calls=%d err=%v", backend.createIssueCalls, err)
	}
}

type admissionBoundaryFunc func(context.Context, ActionAdmissionRequest) (*model.ActionAdmission, error)

func (f admissionBoundaryFunc) AdmitAction(ctx context.Context, request ActionAdmissionRequest) (*model.ActionAdmission, error) {
	return f(ctx, request)
}

func TestLostWritebackRightDoesNotPublishLateFailure(t *testing.T) {
	db, orch, task, run := externalAdmissionFixture(t)
	repositoryID := fixtureRepositoryID
	if err := db.Model(task).Update("repository_id", repositoryID).Error; err != nil {
		t.Fatal(err)
	}
	backend := &fakeGitLabWritebackBackend{}
	orch.codeHostResolver = backendResolverFor(repositoryID, backend)
	orch.actionAdmissions = admissionBoundaryFunc(func(ctx context.Context, request ActionAdmissionRequest) (*model.ActionAdmission, error) {
		if err := db.WithContext(ctx).Model(&model.Writeback{}).Where("dedupe_key = ?", request.ActionKey).Update("status", model.WritebackStatusSucceeded).Error; err != nil {
			t.Fatal(err)
		}
		return nil, repository.ErrActionLeaseLost
	})
	writebacks, err := BuildCodeHostWritebacksFromResult(results.CanonicalResult{TaskID: task.TaskID, RunID: run.RunID, Writebacks: []results.CodeHostWritebackIntent{{Kind: CodeHostWritebackCreateIssue, ProjectID: "group/framework", Title: "title"}}})
	if err != nil {
		t.Fatal(err)
	}
	err = orch.applyOneCodeHostWriteback(context.Background(), &writebacks[0])
	stored, _ := orch.writebacks.GetByDedupeKey(context.Background(), writebacks[0].DedupeKey)
	if stored.Status != model.WritebackStatusSucceeded || !errors.Is(err, ErrWritebackRequiresReconciliation) || backend.createIssueCalls != 0 {
		t.Fatalf("stale worker overwrote winning outcome: status=%s calls=%d err=%v", stored.Status, backend.createIssueCalls, err)
	}
}
