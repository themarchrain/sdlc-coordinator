package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/adapters/gitlab"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestBuildGitLabWritebacksUsesExplicitFields(t *testing.T) {
	writebacks, err := BuildGitLabWritebacksFromResult(results.CanonicalResult{
		TaskID: "task-writeback-build",
		RunID:  "run-writeback-build",
		Writebacks: []results.CodeHostWritebackIntent{
			{
				DedupeKey:          "gitlab:mr:framework:agent-fix-validator",
				Kind:               "create_merge_request",
				ProjectID:          "group/framework",
				Title:              "fix framework validator",
				Description:        "created by coordinator",
				SourceBranch:       "agent/fix-validator",
				TargetBranch:       "main",
				RemoveSourceBranch: true,
				Labels:             []string{"agent:generated", "agent:pending-review"},
			},
		},
	})
	if err != nil {
		t.Fatalf("build writebacks: %v", err)
	}
	if len(writebacks) != 1 {
		t.Fatalf("expected one writeback, got %d", len(writebacks))
	}

	writeback := writebacks[0]
	if writeback.TaskID != "task-writeback-build" {
		t.Fatalf("expected task id, got %s", writeback.TaskID)
	}
	if writeback.RunID != "run-writeback-build" {
		t.Fatalf("expected run id, got %s", writeback.RunID)
	}
	if writeback.DedupeKey != "gitlab:mr:framework:agent-fix-validator" {
		t.Fatalf("expected explicit dedupe key, got %s", writeback.DedupeKey)
	}
	if writeback.Kind != "create_merge_request" {
		t.Fatalf("expected create_merge_request kind, got %s", writeback.Kind)
	}
	if writeback.ProjectID != "group/framework" {
		t.Fatalf("expected project id, got %s", writeback.ProjectID)
	}
	if writeback.Status != "pending" {
		t.Fatalf("expected pending status, got %s", writeback.Status)
	}

	var request map[string]any
	if err := json.Unmarshal([]byte(writeback.RequestJSON), &request); err != nil {
		t.Fatalf("decode request json: %v", err)
	}
	if request["source_branch"] != "agent/fix-validator" {
		t.Fatalf("expected source branch in request, got %#v", request)
	}
	if request["remove_source_branch"] != true {
		t.Fatalf("expected remove_source_branch in request, got %#v", request)
	}
}

func TestValidateCodeHostWritebacksUsesPersistedDeliveryBranch(t *testing.T) {
	task := &model.Task{
		TaskID:         "task-persisted-branch",
		TargetRepo:     "group/framework",
		DeliveryBranch: "alice/issue-13-20260727",
	}
	payload := results.CanonicalResult{Writebacks: []results.CodeHostWritebackIntent{{
		Kind:         CodeHostWritebackCreateMergeRequest,
		ProjectID:    "group/framework",
		SourceBranch: "alice/issue-13-20260727",
		TargetBranch: "main",
	}}}
	if err := validateCodeHostWritebacksForTask(task, payload); err != nil {
		t.Fatalf("validate persisted delivery branch: %v", err)
	}
}

func TestBuildGitLabWritebacksDefaultsStableDedupeKey(t *testing.T) {
	writebacks, err := BuildGitLabWritebacksFromResult(results.CanonicalResult{
		TaskID: "task-writeback-default",
		RunID:  "run-writeback-default",
		Writebacks: []results.CodeHostWritebackIntent{
			{
				Kind:        "create_issue",
				ProjectID:   "group/framework",
				Title:       "framework defect",
				Description: "downstream failure evidence",
			},
		},
	})
	if err != nil {
		t.Fatalf("build writebacks: %v", err)
	}
	if writebacks[0].DedupeKey != "codehost:task-writeback-default:run-writeback-default:0:create_issue" {
		t.Fatalf("expected stable default dedupe key, got %s", writebacks[0].DedupeKey)
	}
}

func TestBuildCodeHostWritebacksPersistsAndEmbedsOperationMarker(t *testing.T) {
	writebacks, err := BuildCodeHostWritebacksFromResult(results.CanonicalResult{
		TaskID: "task-writeback-marker",
		RunID:  "run-writeback-marker",
		Writebacks: []results.CodeHostWritebackIntent{
			{
				DedupeKey:   "codehost:task-writeback-marker:issue",
				Kind:        CodeHostWritebackCreateIssue,
				ProjectID:   "group/framework",
				Title:       "framework defect",
				Description: "failure evidence",
			},
			{
				DedupeKey:       "codehost:task-writeback-marker:note",
				Kind:            CodeHostWritebackCreateMergeRequestNote,
				ProjectID:       "group/framework",
				MergeRequestIID: 7,
				Body:            "review passed",
			},
		},
	})
	if err != nil {
		t.Fatalf("build writebacks: %v", err)
	}
	if len(writebacks) != 2 {
		t.Fatalf("expected two writebacks, got %d", len(writebacks))
	}
	for _, writeback := range writebacks {
		wantMarker := CodeHostOperationMarker(writeback.DedupeKey)
		if !strings.HasPrefix(wantMarker, "<!-- agent-coordinator-operation:") {
			t.Fatalf("new writeback used a retired marker namespace: %q", wantMarker)
		}
		if writeback.OperationMarker != wantMarker {
			t.Fatalf("expected marker %q, got %q", wantMarker, writeback.OperationMarker)
		}
		var request results.CodeHostWritebackIntent
		if err := json.Unmarshal([]byte(writeback.RequestJSON), &request); err != nil {
			t.Fatalf("decode marked request: %v", err)
		}
		if !strings.Contains(request.Description+request.Body, wantMarker) {
			t.Fatalf("expected request content to contain marker %q: %#v", wantMarker, request)
		}
	}
}

func TestBuildGitLabWritebacksRejectsMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name   string
		intent results.CodeHostWritebackIntent
		want   string
	}{
		{
			name: "missing project",
			intent: results.CodeHostWritebackIntent{
				Kind:  "create_issue",
				Title: "framework defect",
			},
			want: "project_id",
		},
		{
			name: "missing issue title",
			intent: results.CodeHostWritebackIntent{
				Kind:      "create_issue",
				ProjectID: "group/framework",
			},
			want: "title",
		},
		{
			name: "missing merge request source branch",
			intent: results.CodeHostWritebackIntent{
				Kind:         "create_merge_request",
				ProjectID:    "group/framework",
				Title:        "fix framework",
				TargetBranch: "main",
			},
			want: "source_branch",
		},
		{
			name: "missing issue note iid",
			intent: results.CodeHostWritebackIntent{
				Kind:      "create_issue_note",
				ProjectID: "group/test-automates",
				Body:      "created upstream fix",
			},
			want: "issue_iid",
		},
		{
			name: "missing merge request note body",
			intent: results.CodeHostWritebackIntent{
				Kind:            "create_merge_request_note",
				ProjectID:       "group/framework",
				MergeRequestIID: 7,
			},
			want: "body",
		},
		{
			name: "missing issue labels",
			intent: results.CodeHostWritebackIntent{
				Kind:      "set_issue_labels",
				ProjectID: "group/test-automates",
				IssueIID:  42,
			},
			want: "labels",
		},
		{
			name: "missing merge request labels",
			intent: results.CodeHostWritebackIntent{
				Kind:            "set_merge_request_labels",
				ProjectID:       "group/framework",
				MergeRequestIID: 7,
			},
			want: "labels",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildGitLabWritebacksFromResult(results.CanonicalResult{
				TaskID:     "task-invalid-writeback",
				RunID:      "run-invalid-writeback",
				Writebacks: []results.CodeHostWritebackIntent{tc.intent},
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestApplyCanonicalResultExecutesGitLabWritebacks(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	backend := &fakeGitLabWritebackBackend{}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, taskRepo, runRepo, "task-gitlab-writeback", "run-gitlab-writeback")

	task, run, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-gitlab-writeback",
		RunID:  "run-gitlab-writeback",
		Status: results.StatusSucceeded,
		Writebacks: []results.CodeHostWritebackIntent{
			{
				DedupeKey:    "gitlab:mr:framework:agent-fix-validator",
				Kind:         GitLabWritebackCreateMergeRequest,
				ProjectID:    "group/framework",
				Title:        "fix framework validator",
				Description:  "created by coordinator",
				SourceBranch: model.DeliveryBranchForTask("task-gitlab-writeback"),
				TargetBranch: "main",
				Labels:       []string{"agent:generated"},
			},
		},
	})
	if err != nil {
		t.Fatalf("apply callback: %v", err)
	}
	if task.Status != model.TaskStatusSucceeded {
		t.Fatalf("expected task succeeded, got %s", task.Status)
	}
	if run.Status != model.RunStatusSucceeded {
		t.Fatalf("expected run succeeded, got %s", run.Status)
	}
	if backend.createMergeRequestCalls != 1 {
		t.Fatalf("expected one create MR call, got %d", backend.createMergeRequestCalls)
	}

	writebacks, err := writebackRepo.ListByRunID(ctx, "run-gitlab-writeback")
	if err != nil {
		t.Fatalf("list writebacks: %v", err)
	}
	if len(writebacks) != 1 {
		t.Fatalf("expected one writeback, got %d", len(writebacks))
	}
	if writebacks[0].Status != model.WritebackStatusSucceeded {
		t.Fatalf("expected succeeded writeback, got %s", writebacks[0].Status)
	}
	if writebacks[0].ExternalID != "7" {
		t.Fatalf("expected external id 7, got %s", writebacks[0].ExternalID)
	}
	if writebacks[0].ExternalURL != "https://gitlab.example/group/framework/-/merge_requests/7" {
		t.Fatalf("unexpected external url: %s", writebacks[0].ExternalURL)
	}

	artifacts, err := artifactRepo.ListByRunID(ctx, "run-gitlab-writeback")
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected one writeback artifact, got %d", len(artifacts))
	}
	if artifacts[0].Kind != "merge_request" {
		t.Fatalf("expected merge_request artifact, got %s", artifacts[0].Kind)
	}
	if artifacts[0].Name != "writeback:gitlab:mr:framework:agent-fix-validator" {
		t.Fatalf("unexpected artifact name: %s", artifacts[0].Name)
	}
}

func TestApplyCanonicalResultRejectsUnapprovedMergeRequestBranch(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	backend := &fakeGitLabWritebackBackend{}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, taskRepo, runRepo, "task-gitlab-unapproved", "run-gitlab-unapproved")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-gitlab-unapproved",
		RunID:  "run-gitlab-unapproved",
		Status: results.StatusSucceeded,
		Writebacks: []results.CodeHostWritebackIntent{
			{
				Kind:         GitLabWritebackCreateMergeRequest,
				ProjectID:    "group/framework",
				Title:        "unapproved branch",
				SourceBranch: "agent/unapproved",
				TargetBranch: "main",
			},
		},
	})
	if err == nil {
		t.Fatal("expected delivery branch validation error")
	}
	if !strings.Contains(err.Error(), "delivery branch") {
		t.Fatalf("expected delivery branch error, got %v", err)
	}
	if backend.createMergeRequestCalls != 0 {
		t.Fatalf("expected no codehost call, got %d", backend.createMergeRequestCalls)
	}

	task, err := taskRepo.GetByTaskID(ctx, "task-gitlab-unapproved")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != model.TaskStatusRunning {
		t.Fatalf("expected task to remain running, got %s", task.Status)
	}
}

func TestApplyCanonicalResultRejectsUnapprovedMergeRequestProject(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	backend := &fakeGitLabWritebackBackend{}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, taskRepo, runRepo, "task-gitlab-wrong-project", "run-gitlab-wrong-project")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-gitlab-wrong-project",
		RunID:  "run-gitlab-wrong-project",
		Status: results.StatusSucceeded,
		Writebacks: []results.CodeHostWritebackIntent{
			{
				Kind:         GitLabWritebackCreateMergeRequest,
				ProjectID:    "group/other-project",
				Title:        "wrong project",
				SourceBranch: model.DeliveryBranchForTask("task-gitlab-wrong-project"),
				TargetBranch: "main",
			},
		},
	})
	if err == nil {
		t.Fatal("expected delivery repository validation error")
	}
	if !strings.Contains(err.Error(), "delivery repository") {
		t.Fatalf("expected delivery repository error, got %v", err)
	}
	if backend.createMergeRequestCalls != 0 {
		t.Fatalf("expected no codehost call, got %d", backend.createMergeRequestCalls)
	}
}

func TestApplyCanonicalResultSkipsSucceededGitLabWriteback(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	backend := &fakeGitLabWritebackBackend{}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, taskRepo, runRepo, "task-gitlab-idempotent", "run-gitlab-idempotent")

	payload := results.CanonicalResult{
		TaskID: "task-gitlab-idempotent",
		RunID:  "run-gitlab-idempotent",
		Status: results.StatusSucceeded,
		Writebacks: []results.CodeHostWritebackIntent{
			{
				DedupeKey:    "gitlab:mr:framework:agent-fix-idempotent",
				Kind:         GitLabWritebackCreateMergeRequest,
				ProjectID:    "group/framework",
				Title:        "fix framework validator",
				SourceBranch: model.DeliveryBranchForTask("task-gitlab-idempotent"),
				TargetBranch: "main",
			},
		},
	}
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("first callback: %v", err)
	}
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("second callback: %v", err)
	}
	if backend.createMergeRequestCalls != 1 {
		t.Fatalf("expected repeated callback to skip GitLab, got %d calls", backend.createMergeRequestCalls)
	}
}

func TestApplyCanonicalResultGitLabWritebackFailureDoesNotAdvanceTaskOrRun(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	backend := &fakeGitLabWritebackBackend{err: errors.New("gitlab unavailable")}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, taskRepo, runRepo, "task-gitlab-fail", "run-gitlab-fail")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-gitlab-fail",
		RunID:  "run-gitlab-fail",
		Status: results.StatusSucceeded,
		Writebacks: []results.CodeHostWritebackIntent{
			{
				DedupeKey:    "gitlab:mr:framework:agent-fix-fail",
				Kind:         GitLabWritebackCreateMergeRequest,
				ProjectID:    "group/framework",
				Title:        "fix framework validator",
				SourceBranch: model.DeliveryBranchForTask("task-gitlab-fail"),
				TargetBranch: "main",
			},
		},
	})
	if err == nil {
		t.Fatal("expected gitlab writeback failure")
	}

	task, err := taskRepo.GetByTaskID(ctx, "task-gitlab-fail")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != model.TaskStatusRunning {
		t.Fatalf("expected task to remain running, got %s", task.Status)
	}
	run, err := runRepo.GetByRunID(ctx, "run-gitlab-fail")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != model.RunStatusRunning {
		t.Fatalf("expected run to remain running, got %s", run.Status)
	}

	writeback, err := writebackRepo.GetByDedupeKey(ctx, "gitlab:mr:framework:agent-fix-fail")
	if err != nil {
		t.Fatalf("get writeback: %v", err)
	}
	if writeback.Status != model.WritebackStatusFailed {
		t.Fatalf("expected failed writeback, got %s", writeback.Status)
	}
	if writeback.ErrorMessage != "gitlab unavailable" {
		t.Fatalf("expected error message, got %s", writeback.ErrorMessage)
	}
}

func TestApplyCanonicalResultMarksAmbiguousCodeHostWritebackUnknown(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	backend := &fakeGitLabWritebackBackend{err: codehost.NewAmbiguousError(errors.New("response ended prematurely"))}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, taskRepo, runRepo, "task-codehost-unknown", "run-codehost-unknown")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-codehost-unknown",
		RunID:  "run-codehost-unknown",
		Status: results.StatusSucceeded,
		Writebacks: []results.CodeHostWritebackIntent{
			{
				DedupeKey:    "codehost:mr:framework:unknown",
				Kind:         CodeHostWritebackCreateMergeRequest,
				ProjectID:    "group/framework",
				Title:        "fix framework validator",
				SourceBranch: model.DeliveryBranchForTask("task-codehost-unknown"),
				TargetBranch: "main",
			},
		},
	})
	if err == nil {
		t.Fatal("expected ambiguous codehost error")
	}
	writeback, err := writebackRepo.GetByDedupeKey(ctx, "codehost:mr:framework:unknown")
	if err != nil {
		t.Fatalf("get writeback: %v", err)
	}
	if writeback.Status != model.WritebackStatusUnknown {
		t.Fatalf("expected unknown writeback, got %s", writeback.Status)
	}
	if writeback.OperationMarker != CodeHostOperationMarker(writeback.DedupeKey) {
		t.Fatalf("expected persisted operation marker, got %q", writeback.OperationMarker)
	}
}

func TestApplyCanonicalResultDoesNotRetryFailedGitLabWritebackAutomatically(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	backend := &fakeGitLabWritebackBackend{}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, taskRepo, runRepo, "task-gitlab-existing-failed", "run-gitlab-existing-failed")
	if err := writebackRepo.Create(ctx, &model.Writeback{
		TaskID:       "task-gitlab-existing-failed",
		RunID:        "run-gitlab-existing-failed",
		DedupeKey:    "gitlab:mr:framework:agent-fix-existing-failed",
		Kind:         GitLabWritebackCreateMergeRequest,
		ProjectID:    "group/framework",
		Status:       model.WritebackStatusFailed,
		RequestJSON:  `{"kind":"create_merge_request","project_id":"group/framework","title":"fix","source_branch":"agent/task-gitlab-existing-failed","target_branch":"main"}`,
		ErrorMessage: "previous timeout",
	}); err != nil {
		t.Fatalf("create failed writeback: %v", err)
	}

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-gitlab-existing-failed",
		RunID:  "run-gitlab-existing-failed",
		Status: results.StatusSucceeded,
		Writebacks: []results.CodeHostWritebackIntent{
			{
				DedupeKey:    "gitlab:mr:framework:agent-fix-existing-failed",
				Kind:         GitLabWritebackCreateMergeRequest,
				ProjectID:    "group/framework",
				Title:        "fix",
				SourceBranch: model.DeliveryBranchForTask("task-gitlab-existing-failed"),
				TargetBranch: "main",
			},
		},
	})
	if err == nil {
		t.Fatal("expected existing failed writeback to stop callback")
	}
	if backend.createMergeRequestCalls != 0 {
		t.Fatalf("expected no automatic retry, got %d GitLab calls", backend.createMergeRequestCalls)
	}
}

func TestApplyCanonicalResultDoesNotReexecuteWritebackLeftRunningByCrash(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	backend := &fakeGitLabWritebackBackend{}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, taskRepo, runRepo, "task-writeback-crash", "run-writeback-crash")
	dedupeKey := "codehost:mr:framework:crash-window"
	if err := writebackRepo.Create(ctx, &model.Writeback{
		TaskID: "task-writeback-crash", RunID: "run-writeback-crash", DedupeKey: dedupeKey,
		Kind: CodeHostWritebackCreateMergeRequest, ProjectID: "group/framework", Status: model.WritebackStatusRunning,
		OperationMarker: CodeHostOperationMarker(dedupeKey),
		RequestJSON:     `{"kind":"create_merge_request","project_id":"group/framework","title":"fix","source_branch":"agent/task-writeback-crash","target_branch":"main"}`,
	}); err != nil {
		t.Fatal(err)
	}

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-writeback-crash", RunID: "run-writeback-crash", Status: results.StatusSucceeded,
		Writebacks: []results.CodeHostWritebackIntent{{
			DedupeKey: dedupeKey, Kind: CodeHostWritebackCreateMergeRequest, ProjectID: "group/framework",
			Title: "fix", SourceBranch: model.DeliveryBranchForTask("task-writeback-crash"), TargetBranch: "main",
		}},
	})
	if err == nil {
		t.Fatal("expected crash-window writeback to require reconciliation")
	}
	if backend.createMergeRequestCalls != 0 {
		t.Fatalf("running writeback was executed again after an ambiguous crash window: %d calls", backend.createMergeRequestCalls)
	}
	stored, err := writebackRepo.GetByDedupeKey(ctx, dedupeKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.WritebackStatusUnknown {
		t.Fatalf("expected running writeback to become unknown, got %s", stored.Status)
	}
}

func TestReconcileUnknownWritebackFoundCompletesWithoutReexecution(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	backend := &fakeGitLabWritebackBackend{
		reconcileResult: codehost.ReconcileResult{
			State:       codehost.ReconcileStateFound,
			ExternalID:  "7",
			ExternalURL: "https://gitlab.example/group/framework/-/merge_requests/7",
			Response:    map[string]any{"iid": 7},
		},
	}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, taskRepo, runRepo, "task-reconcile", "run-reconcile")
	marker := CodeHostOperationMarker("codehost:mr:framework:reconcile")
	if err := writebackRepo.Create(ctx, &model.Writeback{
		TaskID:          "task-reconcile",
		RunID:           "run-reconcile",
		DedupeKey:       "codehost:mr:framework:reconcile",
		Kind:            CodeHostWritebackCreateMergeRequest,
		ProjectID:       "group/framework",
		Status:          model.WritebackStatusUnknown,
		OperationMarker: marker,
		RequestJSON:     `{"kind":"create_merge_request","project_id":"group/framework","source_branch":"agent/task-reconcile","target_branch":"main"}`,
		ErrorMessage:    "response ended prematurely",
	}); err != nil {
		t.Fatalf("create unknown writeback: %v", err)
	}

	results, err := orch.ReconcileUnknownWritebacks(ctx, 10)
	if err != nil {
		t.Fatalf("reconcile unknown writebacks: %v", err)
	}
	if len(results) != 1 || results[0].State != codehost.ReconcileStateFound {
		t.Fatalf("unexpected reconcile results: %#v", results)
	}
	stored, err := writebackRepo.GetByDedupeKey(ctx, "codehost:mr:framework:reconcile")
	if err != nil {
		t.Fatalf("get reconciled writeback: %v", err)
	}
	if stored.Status != model.WritebackStatusSucceeded || stored.ExternalID != "7" {
		t.Fatalf("expected reconciled success, got %#v", stored)
	}
	if stored.ReconcileAttemptCount != 1 || stored.LastReconciledAt == nil || stored.LastReconciledAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("expected reconcile audit fields, got %#v", stored)
	}
	if backend.createMergeRequestCalls != 0 {
		t.Fatalf("reconciliation must not re-execute create MR, got %d calls", backend.createMergeRequestCalls)
	}
	if backend.reconcileCalls != 1 || backend.reconcileRequest.OperationMarker != marker {
		t.Fatalf("unexpected reconcile request: calls=%d request=%#v", backend.reconcileCalls, backend.reconcileRequest)
	}
}

func TestReconcileUnknownWritebackEscalatesAfterPolicyLimit(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Writeback{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	writebacks := repository.NewWritebackRepository(db)
	submissions := repository.NewResultSubmissionRepository(db)
	backend := &fakeGitLabWritebackBackend{
		err:             codehost.NewAmbiguousError(errors.New("response ended prematurely")),
		reconcileResult: codehost.ReconcileResult{State: codehost.ReconcileStateUnknown},
	}
	orch := New(
		tasks,
		runs,
		WithArtifactRepository(repository.NewArtifactRepository(db)),
		WithWritebackRepository(writebacks),
		WithResultSubmissionRepository(submissions),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
		WithPolicy(Policy{MaxAttempts: 2}),
	)
	createCallbackArtifactFixture(t, ctx, tasks, runs, "task-reconcile-limit", "run-reconcile-limit")
	task, _ := tasks.GetByTaskID(ctx, "task-reconcile-limit")
	task.TargetRepo = "group/framework"
	if err := tasks.Update(ctx, task); err != nil {
		t.Fatalf("update task: %v", err)
	}
	request := SubmitResultRequest{
		Source: "mcp_developer",
		Canonical: results.CanonicalResult{
			TaskID: "task-reconcile-limit", RunID: "run-reconcile-limit", Status: results.StatusSucceeded,
			Writebacks: []results.CodeHostWritebackIntent{{
				DedupeKey: "codehost:mr:framework:limit", Kind: CodeHostWritebackCreateMergeRequest,
				ProjectID: "group/framework", Title: "fix framework",
				SourceBranch: model.DeliveryBranchForTask("task-reconcile-limit"), TargetBranch: "main",
			}},
		},
	}
	if _, err := orch.SubmitResult(ctx, request); err == nil {
		t.Fatal("expected ambiguous writeback failure")
	}
	if _, err := orch.ReconcileUnknownWritebacks(ctx, 10); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if _, err := orch.ReconcileUnknownWritebacks(ctx, 10); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	task, _ = tasks.GetByTaskID(ctx, "task-reconcile-limit")
	run, _ := runs.GetByRunID(ctx, "run-reconcile-limit")
	submission, _ := submissions.GetByRunID(ctx, "run-reconcile-limit")
	if task.Status != model.TaskStatusHumanRequired || run.Status != model.RunStatusBlocked {
		t.Fatalf("expected human gate, got task=%s run=%s", task.Status, run.Status)
	}
	if submission.Status != model.ResultSubmissionStatusUnknown {
		t.Fatalf("expected unknown submission at human gate, got %s", submission.Status)
	}
	recoverable, err := orch.FinalizeRecoverableSubmissions(ctx, 10)
	if err != nil {
		t.Fatalf("list recoverable finalizations: %v", err)
	}
	if len(recoverable) != 0 {
		t.Fatalf("human-gated unknown result must not auto-finalize, got %#v", recoverable)
	}
}

type fakeGitLabWritebackBackend struct {
	createIssueCalls        int
	createMergeRequestCalls int
	err                     error
	reconcileCalls          int
	reconcileRequest        codehost.ReconcileRequest
	reconcileResult         codehost.ReconcileResult
	reconcileErr            error
}

func (f *fakeGitLabWritebackBackend) ReconcileWriteback(ctx context.Context, req codehost.ReconcileRequest) (codehost.ReconcileResult, error) {
	f.reconcileCalls++
	f.reconcileRequest = req
	return f.reconcileResult, f.reconcileErr
}

func (f *fakeGitLabWritebackBackend) CreateIssue(ctx context.Context, projectID string, req gitlab.CreateIssueRequest) (*gitlab.Issue, error) {
	f.createIssueCalls++
	if f.err != nil {
		return nil, f.err
	}
	return &gitlab.Issue{IID: 42, WebURL: "https://gitlab.example/" + projectID + "/-/issues/42"}, nil
}

func (f *fakeGitLabWritebackBackend) CreateMergeRequest(ctx context.Context, projectID string, req gitlab.CreateMergeRequestRequest) (*gitlab.MergeRequest, error) {
	f.createMergeRequestCalls++
	if f.err != nil {
		return nil, f.err
	}
	return &gitlab.MergeRequest{IID: 7, WebURL: "https://gitlab.example/group/framework/-/merge_requests/7"}, nil
}

func (f *fakeGitLabWritebackBackend) CreateIssueNote(ctx context.Context, projectID string, issueIID int, req gitlab.CreateNoteRequest) (*gitlab.Note, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &gitlab.Note{ID: 101, Body: req.Body}, nil
}

func (f *fakeGitLabWritebackBackend) CreateMergeRequestNote(ctx context.Context, projectID string, mrIID int, req gitlab.CreateNoteRequest) (*gitlab.Note, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &gitlab.Note{ID: 202, Body: req.Body}, nil
}

func (f *fakeGitLabWritebackBackend) SetIssueLabels(ctx context.Context, projectID string, issueIID int, req gitlab.SetLabelsRequest) (*gitlab.Issue, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &gitlab.Issue{IID: issueIID, WebURL: "https://gitlab.example/" + projectID + "/-/issues/42"}, nil
}

func (f *fakeGitLabWritebackBackend) SetMergeRequestLabels(ctx context.Context, projectID string, mrIID int, req gitlab.SetLabelsRequest) (*gitlab.MergeRequest, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &gitlab.MergeRequest{IID: mrIID, WebURL: "https://gitlab.example/" + projectID + "/-/merge_requests/7"}, nil
}

func newGitLabWritebackRepositories(t *testing.T) (*repository.TaskRepository, *repository.RunRepository, *repository.ArtifactRepository, *repository.WritebackRepository) {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Writeback{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return repository.NewTaskRepository(db), repository.NewRunRepository(db), repository.NewArtifactRepository(db), repository.NewWritebackRepository(db)
}

func createGitLabWritebackFixture(
	t *testing.T,
	ctx context.Context,
	taskRepo *repository.TaskRepository,
	runRepo *repository.RunRepository,
	taskID string,
	runID string,
) {
	t.Helper()

	currentRunID := runID
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID:       taskID,
		DedupeKey:    "dedupe-" + taskID,
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		TargetRepo:   "group/framework",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task fixture: %v", err)
	}

	run := &model.Run{
		RunID:      runID,
		TaskID:     taskID,
		WorkerRole: "DeveloperWorker",
		Status:     model.RunStatusRunning,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}
}
