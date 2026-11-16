package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestRecoverySafetyReaderReadsAllPreviousRuns(t *testing.T) {
	db := openRecoverySafetyDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	currentRunID := "run-current"
	repositoryID := "repo-1"
	task := &model.Task{
		TaskID: "task-safety-reader", DedupeKey: "dedupe-safety-reader", Type: "review_mr", Role: "ReviewerWorker",
		RepositoryID: &repositoryID, Status: model.TaskStatusBlocked, AttemptCount: 5, CurrentRunID: &currentRunID,
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"head_sha":"head-1"}}`,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	runs := []model.Run{
		{RunID: "run-old", TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFailed},
		{RunID: currentRunID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFailed},
	}
	if err := db.Create(&runs).Error; err != nil {
		t.Fatal(err)
	}
	finishedAt := now.Add(-5 * time.Minute)
	if err := db.Create(&model.AgentExecution{RunID: currentRunID, ExecutionID: currentRunID, Status: model.AgentExecutionStatusFailed, FinishedAt: &finishedAt, RequestHash: "hash-current"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResultSubmission{TaskID: task.TaskID, RunID: "run-old", Source: "mcp", Role: task.Role, PayloadHash: "payload-hash", RawPayloadJSON: `{}`, CanonicalJSON: `{}`, Status: model.ResultSubmissionStatusProcessing}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ActionAdmission{AdmissionID: "admission-old", TaskID: task.TaskID, RunID: "run-old", Kind: model.ActionGitPush, ActionKey: "old-push", Attempt: 1, LeaseOwner: "owner", LeaseGeneration: 1, AdmittedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StopRequest{StopID: "stop-reader", DedupeKey: "task:" + task.TaskID, Scope: model.StopScopeTask, TaskID: task.TaskID, RequestedBy: "admin", RequestedAt: now, ReasonCode: "manual_stop", DeliveryStatus: model.StopDeliveryConfirmed}).Error; err != nil {
		t.Fatal(err)
	}
	reader := &RecoverySafetyReader{
		Store:            repository.NewTaskRecoveryRepository(db),
		RepositoryLookup: fakeRecoveryRepositoryLookup{repo: &registeredrepo.RegisteredRepository{RepositoryID: "repo-1", Status: registeredrepo.StatusEnabled}},
		Now:              func() time.Time { return now },
	}
	facts, err := reader.Read(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("read recovery facts: %v", err)
	}
	if facts.Task == nil || facts.Task.TaskID != task.TaskID || len(facts.Runs) != 2 {
		t.Fatalf("unexpected facts: %+v", facts)
	}
	if len(facts.ResultSubmissions) != 1 || facts.ResultSubmissions[0].Status != model.ResultSubmissionStatusProcessing {
		t.Fatalf("submission facts missing: %+v", facts.ResultSubmissions)
	}
	if len(facts.ActionAdmissions) != 1 || facts.ActionAdmissions[0].RunID != "run-old" {
		t.Fatalf("old run action missing: %+v", facts.ActionAdmissions)
	}
	if facts.Stop == nil || facts.Stop.StopID != "stop-reader" {
		t.Fatalf("stop facts missing: %+v", facts.Stop)
	}
	if facts.Repository == nil || facts.Repository.Status != registeredrepo.StatusEnabled {
		t.Fatalf("repository facts missing: %+v", facts.Repository)
	}
	if facts.CheckedAt != now || facts.SnapshotVersion == "" {
		t.Fatalf("assessment metadata missing: %+v", facts)
	}

	var after model.Task
	if err := db.Where("task_id = ?", task.TaskID).First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != model.TaskStatusBlocked || after.AttemptCount != 5 {
		t.Fatalf("read changed task: %+v", after)
	}
}

func TestRecoverySafetyReaderReportsRepositoryLookupFailure(t *testing.T) {
	db := openRecoverySafetyDB(t)
	task := &model.Task{TaskID: "task-safety-repo", DedupeKey: "dedupe-safety-repo", Type: "bugfix", Role: "DeveloperWorker", Status: model.TaskStatusBlocked, TargetRepo: "group/project"}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	repositoryID := "repo-missing"
	task.RepositoryID = &repositoryID
	if err := db.Model(task).Update("repository_id", repositoryID).Error; err != nil {
		t.Fatal(err)
	}
	reader := &RecoverySafetyReader{
		Store:            repository.NewTaskRecoveryRepository(db),
		RepositoryLookup: fakeRecoveryRepositoryLookup{err: errors.New("repository lookup unavailable")},
		Now:              func() time.Time { return time.Now().UTC() },
	}
	facts, err := reader.Read(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("read recovery facts: %v", err)
	}
	if facts.RepositoryLookupError == "" {
		t.Fatal("repository lookup failure was silently omitted")
	}
	assessment := EvaluateTaskRecovery(facts)
	if assessment.Eligible || !hasSafetyBlocker(assessment, "repository_lookup_failed") {
		t.Fatalf("repository lookup failure did not block: %+v", assessment)
	}
}

func openRecoverySafetyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.Writeback{}, &model.Effect{}, &model.ActionAdmission{}, &model.WorkItemLease{}, &model.DeveloperDeliveryCheckpoint{}, &model.MergeGateAudit{}, &model.StopRequest{}, &model.TaskRecovery{}, &model.ExecutionExitConfirmation{}); err != nil {
		t.Fatal(err)
	}
	return db
}

type fakeRecoveryRepositoryLookup struct {
	repo *registeredrepo.RegisteredRepository
	err  error
}

func (f fakeRecoveryRepositoryLookup) Get(context.Context, string) (*registeredrepo.RegisteredRepository, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.repo, nil
}

func TestRunnerSetupCodesNeverUnlockRecovery(t *testing.T) {
	for _, source := range []string{failure.SourceRunner, failure.SourceProvider} {
		t.Run(source, func(t *testing.T) {
			db := openRecoverySafetyDB(t)
			ctx := context.Background()
			now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
			runID := "run-setup-code-" + source
			repositoryID := "repo-setup-code"
			task := &model.Task{
				TaskID: "task-setup-code-" + source, DedupeKey: "dedupe-setup-code-" + source, Type: "bugfix", Role: "DeveloperWorker",
				RepositoryID: &repositoryID, TargetRepo: "group/project", Status: model.TaskStatusBlocked,
				CurrentRunID: &runID, AttemptCount: 5, ContextJSON: `{}`,
			}
			if err := db.Create(task).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusBlocked}).Error; err != nil {
				t.Fatal(err)
			}
			finished := now.Add(-time.Minute)
			failureJSON := `{"version":1,"stage":"initialization","source":"` + source + `","code":"` + failure.CodeCLIMissing + `"}`
			if err := db.Create(&model.AgentExecution{
				RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusFailed,
				RequestHash: "request-hash", FinishedAt: &finished, FailureJSON: failureJSON,
			}).Error; err != nil {
				t.Fatal(err)
			}
			reader := &RecoverySafetyReader{
				Store:            repository.NewTaskRecoveryRepository(db),
				RepositoryLookup: fakeRecoveryRepositoryLookup{repo: &registeredrepo.RegisteredRepository{RepositoryID: repositoryID, Status: registeredrepo.StatusEnabled}},
				Now:              func() time.Time { return now },
			}
			facts, err := reader.Read(ctx, task.TaskID)
			if err != nil {
				t.Fatalf("read recovery facts: %v", err)
			}
			assessment := EvaluateTaskRecovery(facts)
			if source == failure.SourceProvider {
				// 供应商回声的码不可信：按 unknown 处理，必须显式承担风险，
				// 不能当成"已修复的前置条件"直接放行。
				if assessment.Classification != failure.ClassificationUnknown || !assessment.RequiresRiskConfirmation {
					t.Fatalf("provider echoed setup code = %+v", assessment)
				}
				return
			}
			if assessment.Eligible || assessment.ReasonCode != RecoveryReasonFailurePrerequisite {
				t.Fatalf("a setup failure unlocked recovery: %+v", assessment)
			}
			if !strings.Contains(assessment.Remediation, "新建任务") {
				t.Fatalf("setup failure remediation = %q", assessment.Remediation)
			}
		})
	}
}

func TestTaskRecoveryObjectKind(t *testing.T) {
	tests := []struct {
		name string
		task *model.Task
		want RecoveryObjectKind
	}{
		{name: "reviewer checks the merge request", task: &model.Task{Role: "ReviewerWorker"}, want: RecoveryObjectMergeRequest},
		{name: "tester checks the merge request", task: &model.Task{Role: "TesterWorker"}, want: RecoveryObjectMergeRequest},
		{name: "rework checks the merge request", task: &model.Task{Role: "DeveloperWorker", Type: "rework_merge_request"}, want: RecoveryObjectMergeRequest},
		{name: "developer issue checks the issue", task: &model.Task{Role: "DeveloperWorker", Type: "develop_from_issue", ContextJSON: `{"issue":{"iid":7}}`}, want: RecoveryObjectIssue},
		{name: "developer without issue has no live object", task: &model.Task{Role: "DeveloperWorker", Type: "bugfix"}, want: RecoveryObjectNone},
		{name: "nil task has no live object", task: nil, want: RecoveryObjectNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := TaskRecoveryObjectKind(test.task); got != test.want {
				t.Fatalf("TaskRecoveryObjectKind() = %q, want %q", got, test.want)
			}
		})
	}
}

type fakeRecoveryObjectReader struct {
	object RecoveryObjectState
	err    error
}

func (f fakeRecoveryObjectReader) ReadRecoveryObject(context.Context, *model.Task) (RecoveryObjectState, error) {
	return f.object, f.err
}

func TestRecoverySafetyReaderFailsClosedOnObjectState(t *testing.T) {
	tests := []struct {
		name     string
		object   RecoveryObjectState
		err      error
		wantCode string
		wantHead string
	}{
		{name: "closed merge request blocks", object: RecoveryObjectState{HeadSHA: "head-1", State: codehost.MergeRequestStateClosed}, wantCode: RecoverySafetyObjectNotOpen, wantHead: "head-1"},
		{name: "merged merge request blocks", object: RecoveryObjectState{HeadSHA: "head-1", State: codehost.MergeRequestStateMerged}, wantCode: RecoverySafetyObjectNotOpen, wantHead: "head-1"},
		{name: "missing state is unavailable", object: RecoveryObjectState{HeadSHA: "head-1"}, wantCode: RecoverySafetyObjectStateUnavailable, wantHead: "head-1"},
		{name: "empty head is unavailable", object: RecoveryObjectState{State: codehost.MergeRequestStateOpen}, wantCode: RecoverySafetyHeadCheckUnavailable},
		{name: "read failure is unavailable", err: errors.New("read merge request state: connection refused"), wantCode: RecoverySafetyObjectStateUnavailable},
		{name: "open merge request passes", object: RecoveryObjectState{HeadSHA: "head-1", State: codehost.MergeRequestStateOpen}, wantHead: "head-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openRecoverySafetyDB(t)
			ctx := context.Background()
			now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
			repositoryID := "repo-object-state"
			runID := "run-object-state"
			task := &model.Task{
				TaskID: "task-object-state", DedupeKey: "dedupe-object-state", Type: "review_mr", Role: "ReviewerWorker",
				RepositoryID: &repositoryID, Status: model.TaskStatusBlocked, CurrentRunID: &runID, AttemptCount: 5,
				ContextJSON: `{"merge_request":{"iid":21,"head_sha":"head-1"}}`,
			}
			if err := db.Create(task).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusBlocked}).Error; err != nil {
				t.Fatal(err)
			}
			finished := now.Add(-time.Minute)
			failureJSON := `{"version":1,"stage":"provider","source":"provider","code":"provider_rate_limited"}`
			if err := db.Create(&model.AgentExecution{
				RunID: runID, ExecutionID: runID, Status: model.AgentExecutionStatusFailed,
				RequestHash: "request-hash", FinishedAt: &finished, FailureJSON: failureJSON,
			}).Error; err != nil {
				t.Fatal(err)
			}
			reader := &RecoverySafetyReader{
				Store:            repository.NewTaskRecoveryRepository(db),
				RepositoryLookup: fakeRecoveryRepositoryLookup{repo: &registeredrepo.RegisteredRepository{RepositoryID: repositoryID, Status: registeredrepo.StatusEnabled}},
				ObjectReader:     fakeRecoveryObjectReader{object: test.object, err: test.err},
				Now:              func() time.Time { return now },
			}
			facts, err := reader.Read(ctx, task.TaskID)
			if err != nil {
				t.Fatalf("read recovery facts: %v", err)
			}
			if !facts.ObjectStateCheckRequired {
				t.Fatal("reviewer task did not require an object state check")
			}
			if facts.CurrentHeadSHA != test.wantHead {
				t.Fatalf("current head = %q, want %q", facts.CurrentHeadSHA, test.wantHead)
			}
			assessment := EvaluateTaskRecovery(facts)
			if test.wantCode == "" {
				if !assessment.Eligible {
					t.Fatalf("inspectable open object was rejected: %+v", assessment.SafetyBlockers)
				}
				return
			}
			if assessment.Eligible || !hasSafetyBlocker(assessment, test.wantCode) {
				t.Fatalf("missing blocker %q: %+v", test.wantCode, assessment.SafetyBlockers)
			}
		})
	}
}

func TestRecoverySafetyReaderTreatsAnUnsentDispatchAsExited(t *testing.T) {
	db := openRecoverySafetyDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	runID := "run-unsent"
	repositoryID := "repo-1"
	task := &model.Task{
		TaskID: "task-unsent", DedupeKey: "dedupe-unsent", Type: "bugfix", Role: "DeveloperWorker",
		RepositoryID: &repositoryID, TargetRepo: "group/project", Status: model.TaskStatusBlocked, AttemptCount: 5,
		CurrentRunID: &runID, ContextJSON: `{}`,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFailed}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AgentExecution{RunID: runID, ExecutionID: runID, Status: model.AgentExecutionStatusDispatching, RequestHash: "hash-unsent"}).Error; err != nil {
		t.Fatal(err)
	}
	reader := &RecoverySafetyReader{
		Store:            repository.NewTaskRecoveryRepository(db),
		RepositoryLookup: fakeRecoveryRepositoryLookup{repo: &registeredrepo.RegisteredRepository{RepositoryID: repositoryID, Status: registeredrepo.StatusEnabled}},
		Now:              func() time.Time { return now },
	}
	facts, err := reader.Read(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("read recovery facts: %v", err)
	}
	if facts.DispatchProvenNotSent || facts.DispatchProvenNotSentRuns[runID] {
		t.Fatalf("an empty admission list looked like evidence: %+v", facts.DispatchProvenNotSentRuns)
	}
	if assessment := EvaluateTaskRecovery(facts); !hasSafetyBlocker(assessment, RecoverySafetyExecutionActive) {
		t.Fatalf("a dispatching execution without evidence did not block recovery: %+v", assessment.SafetyBlockers)
	}
	if err := db.Create(&model.ActionAdmission{
		AdmissionID: "admission-unsent", TaskID: task.TaskID, RunID: runID, Kind: model.ActionExecutionCreate,
		ActionKey: "execution-create", Attempt: 1, LeaseOwner: "owner", LeaseGeneration: 1, AdmittedAt: now, NotSentAt: &now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	facts, err = reader.Read(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("read recovery facts with the admission: %v", err)
	}
	if !facts.DispatchProvenNotSent || !facts.DispatchProvenNotSentRuns[runID] {
		t.Fatalf("the execution-create admission was not read: %+v", facts.DispatchProvenNotSentRuns)
	}
	if assessment := EvaluateTaskRecovery(facts); !assessment.Eligible {
		t.Fatalf("the unsent evidence did not clear the dispatching execution: %+v", assessment.SafetyBlockers)
	}
	// 换端点后的第二次派发真的发出去了：第一笔"未发出"不能代表整个 Run。
	if err := db.Create(&model.ActionAdmission{
		AdmissionID: "admission-resent", TaskID: task.TaskID, RunID: runID, Kind: model.ActionExecutionCreate,
		ActionKey: "execution-create", Attempt: 2, LeaseOwner: "owner", LeaseGeneration: 1, AdmittedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	facts, err = reader.Read(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("read recovery facts with the retried dispatch: %v", err)
	}
	if facts.DispatchProvenNotSentRuns[runID] {
		t.Fatalf("an earlier unsent attempt still covered the run: %+v", facts.DispatchProvenNotSentRuns)
	}
	if assessment := EvaluateTaskRecovery(facts); !hasSafetyBlocker(assessment, RecoverySafetyExecutionActive) {
		t.Fatalf("a possibly sent retry did not block recovery: %+v", assessment.SafetyBlockers)
	}
}
