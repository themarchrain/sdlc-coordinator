package orchestrator

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
)

func TestAssessTransientEligible(t *testing.T) {
	facts := eligibleRecoveryFacts()
	facts.FailureInfo = &failure.Info{Version: failure.VersionV1, Stage: failure.StageProvider, Source: failure.SourceProvider, Code: failure.CodeProviderRateLimited, Summary: "rate limited"}
	assessment := EvaluateTaskRecovery(facts)
	if !assessment.Eligible || assessment.RequiresRiskConfirmation {
		t.Fatalf("transient assessment = %+v", assessment)
	}
	if assessment.Classification != failure.ClassificationTransient || assessment.ReasonCode != "eligible" {
		t.Fatalf("unexpected assessment: %+v", assessment)
	}
	if len(assessment.SafetyBlockers) != 0 {
		t.Fatalf("unexpected safety blockers: %+v", assessment.SafetyBlockers)
	}
}

func TestAssessUnknownRequiresConfirmation(t *testing.T) {
	facts := eligibleRecoveryFacts()
	facts.FailureInfo = &failure.Info{Version: failure.VersionV1, Stage: failure.StageResult, Source: failure.SourceRunner, Code: failure.CodeProviderFailed, Summary: "provider failed"}
	assessment := EvaluateTaskRecovery(facts)
	if assessment.Classification != failure.ClassificationUnknown || !assessment.Eligible || !assessment.RequiresRiskConfirmation {
		t.Fatalf("unknown assessment = %+v", assessment)
	}
	if assessment.ReasonCode != "unknown_risk_confirmation_required" {
		t.Fatalf("reason code = %q", assessment.ReasonCode)
	}
}

func TestPrerequisiteResolvedCanRecover(t *testing.T) {
	facts := eligibleRecoveryFacts()
	facts.FailureInfo = &failure.Info{Version: failure.VersionV1, Stage: failure.StageInitialization, Source: failure.SourceRunner, Code: failure.CodeCLIMissing, Summary: "codex cli missing"}
	blocked := EvaluateTaskRecovery(facts)
	if blocked.Eligible || blocked.ReasonCode != RecoveryReasonFailurePrerequisite {
		t.Fatalf("unresolved prerequisite = %+v", blocked)
	}
	facts.PrerequisiteResolved = true
	resolved := EvaluateTaskRecovery(facts)
	if !resolved.Eligible || resolved.RequiresRiskConfirmation || resolved.ReasonCode != RecoveryReasonPrerequisiteResolved {
		t.Fatalf("resolved prerequisite = %+v", resolved)
	}
	facts.PrerequisiteResolved = false
	facts.PrerequisiteCheckError = "config check failed"
	failed := EvaluateTaskRecovery(facts)
	if failed.Eligible || !hasSafetyBlocker(failed, RecoverySafetyPrerequisiteCheckFailed) {
		t.Fatalf("prerequisite check failure = %+v", failed)
	}
}

func TestUnknownCannotOverrideSafety(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*RecoveryFacts)
		wantCode string
	}{
		{name: "stop", mutate: func(f *RecoveryFacts) {
			f.Stop = &model.StopRequest{StopID: "stop-1", Scope: model.StopScopeTask, TaskID: f.Task.TaskID}
		}, wantCode: "stop_requested"},
		{name: "active execution", mutate: func(f *RecoveryFacts) { f.Executions[0].Status = model.AgentExecutionStatusRunning }, wantCode: "execution_active"},
		{name: "runner restarted without exit", mutate: func(f *RecoveryFacts) {
			f.Executions[0].Status = model.AgentExecutionStatusInterrupted
			f.Executions[0].FinishedAt = nil
		}, wantCode: "execution_unconfirmed"},
		{name: "missing execution projection", mutate: func(f *RecoveryFacts) { f.Executions = nil }, wantCode: "execution_unconfirmed"},
		{name: "processing submission", mutate: func(f *RecoveryFacts) {
			f.ResultSubmissions = []model.ResultSubmission{{RunID: f.Runs[0].RunID, Status: model.ResultSubmissionStatusProcessing}}
		}, wantCode: "submission_processing"},
		{name: "pending writeback", mutate: func(f *RecoveryFacts) {
			f.Writebacks = []model.Writeback{{RunID: f.Runs[0].RunID, Status: model.WritebackStatusPending}}
		}, wantCode: "writeback_pending"},
		{name: "unsettled merge", mutate: func(f *RecoveryFacts) { f.MergeGateAudit = &model.MergeGateAudit{} }, wantCode: "merge_audit_unsettled"},
		{name: "unsettled git push", mutate: func(f *RecoveryFacts) {
			f.ActionAdmissions = []model.ActionAdmission{{RunID: f.Runs[0].RunID, Kind: model.ActionGitPush, ActionKey: "push"}}
		}, wantCode: "action_unsettled"},
		{name: "active execution lease", mutate: func(f *RecoveryFacts) {
			f.WorkItemLeases = []model.WorkItemLease{{Scope: "developer-delivery:" + f.Runs[0].RunID, LeaseUntil: f.CheckedAt.Add(time.Minute)}}
		}, wantCode: "active_execution_lease"},
		{name: "repository disabled", mutate: func(f *RecoveryFacts) {
			f.Repository = &registeredrepo.RegisteredRepository{RepositoryID: "repo-1", Status: registeredrepo.StatusDisabled}
		}, wantCode: "repository_disabled"},
		{name: "task object invalid", mutate: func(f *RecoveryFacts) { f.TaskObjectValid = boolPointer(false) }, wantCode: "task_object_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := eligibleRecoveryFacts()
			facts.FailureInfo = &failure.Info{Version: failure.VersionV1, Stage: failure.StageResult, Source: failure.SourceRunner, Code: failure.CodeProviderFailed}
			test.mutate(&facts)
			assessment := EvaluateTaskRecovery(facts)
			if assessment.Eligible || assessment.RequiresRiskConfirmation {
				t.Fatalf("unknown overrode safety: %+v", assessment)
			}
			if !hasSafetyBlocker(assessment, test.wantCode) {
				t.Fatalf("missing blocker %q: %+v", test.wantCode, assessment.SafetyBlockers)
			}
		})
	}
}

func TestCompletedAdmissionIsNotPending(t *testing.T) {
	facts := eligibleRecoveryFacts()
	completed := time.Date(2026, 11, 10, 9, 55, 0, 0, time.UTC)
	facts.ActionAdmissions = []model.ActionAdmission{{
		RunID: facts.Runs[0].RunID, Kind: model.ActionGitPush, ActionKey: "push", CompletedAt: &completed,
	}}
	assessment := EvaluateTaskRecovery(facts)
	if !assessment.Eligible || len(assessment.SafetyBlockers) != 0 {
		t.Fatalf("completed admission was treated as pending: %+v", assessment)
	}
}

func TestScanAllPreviousRunsForUncertainAction(t *testing.T) {
	facts := eligibleRecoveryFacts()
	oldRunID := "run-old"
	facts.Runs = append([]model.Run{{RunID: oldRunID, TaskID: facts.Task.TaskID, Status: model.RunStatusFailed}}, facts.Runs...)
	facts.ActionAdmissions = []model.ActionAdmission{{RunID: oldRunID, Kind: model.ActionGitPush, ActionKey: "old-push"}}
	assessment := EvaluateTaskRecovery(facts)
	if assessment.Eligible || !hasSafetyBlocker(assessment, "action_unsettled") {
		t.Fatalf("old run action was not scanned: %+v", assessment)
	}
}

func TestAssessmentTimeoutHasNoSideEffects(t *testing.T) {
	task := eligibleRecoveryFacts().Task
	reader := &fakeRecoveryFactsReader{err: context.DeadlineExceeded}
	assessment, err := AssessTaskRecovery(context.Background(), reader, task.TaskID)
	if err == nil {
		t.Fatal("timeout must return an assessment error")
	}
	if assessment.Eligible {
		t.Fatalf("timeout produced eligible assessment: %+v", assessment)
	}
	if task.Status != model.TaskStatusBlocked || task.AttemptCount != 5 {
		t.Fatalf("assessment changed task: %+v", task)
	}
}

func TestOldHeadNotEligible(t *testing.T) {
	facts := eligibleRecoveryFacts()
	facts.ExpectedHeadSHA = "old-head"
	facts.CurrentHeadSHA = "new-head"
	assessment := EvaluateTaskRecovery(facts)
	if assessment.Eligible || !hasSafetyBlocker(assessment, "head_superseded") {
		t.Fatalf("old head was not rejected: %+v", assessment)
	}
}

func eligibleRecoveryFacts() RecoveryFacts {
	runID := "run-recovery-1"
	finishedAt := time.Date(2026, 11, 10, 9, 55, 0, 0, time.UTC)
	checkedAt := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	valid := true
	return RecoveryFacts{
		Task: &model.Task{
			TaskID: "task-recovery-1", DedupeKey: "dedupe-recovery-1", Type: "review_mr", Role: "ReviewerWorker",
			Status: model.TaskStatusBlocked, AttemptCount: 5, CurrentRunID: &runID,
			ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"head_sha":"head-1"}}`,
		},
		Runs: []model.Run{{RunID: runID, TaskID: "task-recovery-1", Status: model.RunStatusFailed}},
		Executions: []model.AgentExecution{{
			RunID: runID, ExecutionID: runID, Status: model.AgentExecutionStatusFailed, FinishedAt: &finishedAt,
			RequestHash: "request-hash", ErrorCode: "provider_failed", ErrorMessage: "provider failed",
		}},
		Repository:        &registeredrepo.RegisteredRepository{RepositoryID: "repo-1", Status: registeredrepo.StatusEnabled},
		TaskObjectValid:   &valid,
		ContextValid:      &valid,
		ExpectedHeadSHA:   "head-1",
		CurrentHeadSHA:    "head-1",
		HeadCheckRequired: true,
		CheckedAt:         checkedAt,
	}
}

func boolPointer(value bool) *bool { return &value }

func hasSafetyBlocker(assessment RecoveryAssessment, code string) bool {
	for _, blocker := range assessment.SafetyBlockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}

type fakeRecoveryFactsReader struct {
	facts RecoveryFacts
	err   error
}

func (f *fakeRecoveryFactsReader) Read(context.Context, string) (RecoveryFacts, error) {
	return f.facts, f.err
}
