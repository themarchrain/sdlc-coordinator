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

func TestCompletedDeliveryDoesNotBlockRecoveryButUnsettledGitActionDoes(t *testing.T) {
	completed := eligibleRecoveryFacts()
	completed.DeliveryCheckpoints = []model.DeveloperDeliveryCheckpoint{{
		RunID: completed.Runs[0].RunID, BaselineSHA: "base-sha", CommitSHA: "commit-sha",
	}}
	if assessment := EvaluateTaskRecovery(completed); !assessment.Eligible {
		t.Fatalf("completed delivery blocked recovery: %+v", assessment.SafetyBlockers)
	}

	unsettled := eligibleRecoveryFacts()
	unsettled.ActionAdmissions = []model.ActionAdmission{{
		AdmissionID: "admission-git", Kind: model.ActionGitPush, ActionKey: "push-1",
		Attempt: 1, LeaseOwner: "owner", LeaseGeneration: 1,
	}}
	assessment := EvaluateTaskRecovery(unsettled)
	if assessment.Eligible || !hasSafetyBlocker(assessment, RecoverySafetyActionUnsettled) {
		t.Fatalf("unsettled Git action did not block: %+v", assessment)
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
		{name: "unsettled cancel effect", mutate: func(f *RecoveryFacts) {
			f.Effects = []model.Effect{{EffectID: "effect-1", RunID: f.Runs[0].RunID, Status: model.EffectStatusPending, Critical: true}}
		}, wantCode: "critical_effect_pending"},
		{name: "unavailable head check", mutate: func(f *RecoveryFacts) { f.HeadCheckUnavailable = true }, wantCode: "head_check_unavailable"},
		{name: "unbound repository", mutate: func(f *RecoveryFacts) { f.Repository = nil }, wantCode: "repository_not_bound"},
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

func TestRetiredSafetyBlockersAreNotEmitted(t *testing.T) {
	retired := []string{"tenant_invalid", "permission_denied", "successor_conflict", "execution_capability_unavailable"}
	facts := eligibleRecoveryFacts()
	assessment := EvaluateTaskRecovery(facts)
	if !assessment.Eligible {
		t.Fatalf("baseline assessment = %+v", assessment)
	}
	for _, code := range retired {
		if hasSafetyBlocker(assessment, code) {
			t.Fatalf("retired blocker %q is still emitted", code)
		}
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

func TestOlderInterruptedExecutionBlocksRecovery(t *testing.T) {
	finished := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		finished *time.Time
	}{
		{name: "finished at recorded but exit unknown", finished: &finished},
		{name: "no finish record", finished: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := eligibleRecoveryFacts()
			olderRunID := "run-older-interrupted"
			facts.Runs = append([]model.Run{{RunID: olderRunID, TaskID: facts.Task.TaskID, Status: model.RunStatusInterrupted}}, facts.Runs...)
			facts.Executions = append([]model.AgentExecution{{
				RunID: olderRunID, ExecutionID: olderRunID, Status: model.AgentExecutionStatusInterrupted,
				RequestHash: "older-request-hash", ErrorCode: "runner_restarted", FinishedAt: test.finished,
			}}, facts.Executions...)
			assessment := EvaluateTaskRecovery(facts)
			if assessment.Eligible || !hasSafetyBlocker(assessment, RecoverySafetyExecutionUnconfirmed) {
				t.Fatalf("older execution without exit proof was ignored: %+v", assessment)
			}
		})
	}
}

func TestOlderTerminalExecutionNeedsFinishRecord(t *testing.T) {
	facts := eligibleRecoveryFacts()
	olderRunID := "run-older-failed"
	facts.Runs = append([]model.Run{{RunID: olderRunID, TaskID: facts.Task.TaskID, Status: model.RunStatusFailed}}, facts.Runs...)
	facts.Executions = append([]model.AgentExecution{{
		RunID: olderRunID, ExecutionID: olderRunID, Status: model.AgentExecutionStatusFailed,
		RequestHash: "older-request-hash", ErrorCode: "provider_failed",
	}}, facts.Executions...)
	assessment := EvaluateTaskRecovery(facts)
	if assessment.Eligible || !hasSafetyBlocker(assessment, RecoverySafetyExecutionUnconfirmed) {
		t.Fatalf("terminal execution without finish record was accepted as exited: %+v", assessment)
	}
}

func TestSourceInterruptedNeedsMatchingExitConfirmation(t *testing.T) {
	confirmed := func(facts *RecoveryFacts) model.ExecutionExitConfirmation {
		execution := facts.Executions[len(facts.Executions)-1]
		return model.ExecutionExitConfirmation{
			TaskID: facts.Task.TaskID, RunID: execution.RunID, ExecutionID: execution.ExecutionID,
			SourceGeneration: facts.Task.RecoveryGeneration, ExecutionFingerprint: model.ExecutionExitFingerprint(&execution),
			RemoteState: model.ExecutionExitRemoteTerminal,
		}
	}

	baseline := interruptedRecoveryFacts()
	if assessment := EvaluateTaskRecovery(baseline); assessment.Eligible || !hasSafetyBlocker(assessment, RecoverySafetyExecutionUnconfirmed) {
		t.Fatalf("unconfirmed interrupted source execution = %+v", assessment)
	}

	confirmedFacts := interruptedRecoveryFacts()
	confirmedFacts.ExitConfirmations = []model.ExecutionExitConfirmation{confirmed(&confirmedFacts)}
	if assessment := EvaluateTaskRecovery(confirmedFacts); !assessment.Eligible {
		t.Fatalf("matching exit confirmation did not clear the gate: %+v", assessment.SafetyBlockers)
	}

	staleFingerprint := interruptedRecoveryFacts()
	stale := confirmed(&staleFingerprint)
	stale.ExecutionFingerprint = "fingerprint-of-another-execution-state"
	staleFingerprint.ExitConfirmations = []model.ExecutionExitConfirmation{stale}
	if assessment := EvaluateTaskRecovery(staleFingerprint); !hasSafetyBlocker(assessment, RecoverySafetyExecutionUnconfirmed) {
		t.Fatalf("stale fingerprint cleared the gate: %+v", assessment.SafetyBlockers)
	}

	staleGeneration := interruptedRecoveryFacts()
	staleGeneration.Task.RecoveryGeneration = 2
	older := confirmed(&staleGeneration)
	older.SourceGeneration = 1
	staleGeneration.ExitConfirmations = []model.ExecutionExitConfirmation{older}
	if assessment := EvaluateTaskRecovery(staleGeneration); !hasSafetyBlocker(assessment, RecoverySafetyExecutionUnconfirmed) {
		t.Fatalf("confirmation from an older generation cleared the gate: %+v", assessment.SafetyBlockers)
	}
}

func TestExitConfirmationCoversAnOlderInterruptedExecution(t *testing.T) {
	finished := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	facts := eligibleRecoveryFacts()
	olderRunID := "run-older-interrupted-confirmed"
	facts.Runs = append([]model.Run{{RunID: olderRunID, TaskID: facts.Task.TaskID, Status: model.RunStatusInterrupted}}, facts.Runs...)
	older := model.AgentExecution{
		RunID: olderRunID, ExecutionID: olderRunID, Status: model.AgentExecutionStatusInterrupted,
		RequestHash: "older-request-hash", ErrorCode: "runner_restarted", FinishedAt: &finished,
	}
	facts.Executions = append([]model.AgentExecution{older}, facts.Executions...)
	facts.ExitConfirmations = []model.ExecutionExitConfirmation{{
		TaskID: facts.Task.TaskID, RunID: older.RunID, ExecutionID: older.ExecutionID,
		SourceGeneration: facts.Task.RecoveryGeneration, ExecutionFingerprint: model.ExecutionExitFingerprint(&older),
		RemoteState: model.ExecutionExitRemoteTerminal,
	}}
	assessment := EvaluateTaskRecovery(facts)
	if !assessment.Eligible {
		t.Fatalf("confirmation of the older execution did not clear the gate: %+v", assessment.SafetyBlockers)
	}
	if len(assessment.UnconfirmedExecutions) != 0 {
		t.Fatalf("confirmed execution is still listed: %+v", assessment.UnconfirmedExecutions)
	}
}

func interruptedRecoveryFacts() RecoveryFacts {
	facts := eligibleRecoveryFacts()
	facts.Executions[0].Status = model.AgentExecutionStatusInterrupted
	facts.Executions[0].ErrorCode = "runner_restarted"
	return facts
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

func TestObjectStateGatesRecovery(t *testing.T) {
	open := eligibleRecoveryFacts()
	open.ObjectStateCheckRequired = true
	open.CurrentObjectState = "open"
	if assessment := EvaluateTaskRecovery(open); !assessment.Eligible {
		t.Fatalf("open object was rejected: %+v", assessment.SafetyBlockers)
	}
	for _, state := range []string{"closed", "merged"} {
		facts := eligibleRecoveryFacts()
		facts.ObjectStateCheckRequired = true
		facts.CurrentObjectState = state
		assessment := EvaluateTaskRecovery(facts)
		if assessment.Eligible || !hasSafetyBlocker(assessment, RecoverySafetyObjectNotOpen) {
			t.Fatalf("%s object was accepted: %+v", state, assessment.SafetyBlockers)
		}
	}
	unreadable := eligibleRecoveryFacts()
	unreadable.ObjectStateCheckRequired = true
	unreadable.ObjectStateUnavailable = true
	unreadable.ObjectStateError = "read issue state: connection refused"
	assessment := EvaluateTaskRecovery(unreadable)
	if assessment.Eligible || !hasSafetyBlocker(assessment, RecoverySafetyObjectStateUnavailable) {
		t.Fatalf("unreadable object was accepted: %+v", assessment.SafetyBlockers)
	}
}

func TestEmptyCurrentHeadBlocksRecovery(t *testing.T) {
	facts := eligibleRecoveryFacts()
	facts.CurrentHeadSHA = ""
	assessment := EvaluateTaskRecovery(facts)
	if assessment.Eligible || !hasSafetyBlocker(assessment, RecoverySafetyHeadCheckUnavailable) {
		t.Fatalf("empty current head was accepted: %+v", assessment.SafetyBlockers)
	}
}

func TestDispatchingExecutionBlocksUntilTheDispatchIsProvenUnsent(t *testing.T) {
	facts := eligibleRecoveryFacts()
	facts.Executions[0].Status = model.AgentExecutionStatusDispatching
	facts.Executions[0].FinishedAt = nil
	if assessment := EvaluateTaskRecovery(facts); !hasSafetyBlocker(assessment, RecoverySafetyExecutionActive) {
		t.Fatalf("dispatching execution did not block recovery: %+v", assessment.SafetyBlockers)
	}
	facts.DispatchProvenNotSentRuns = map[string]bool{facts.Executions[0].RunID: true}
	if assessment := EvaluateTaskRecovery(facts); !assessment.Eligible {
		t.Fatalf("an unsent dispatch still blocked recovery: %+v", assessment.SafetyBlockers)
	}
}
