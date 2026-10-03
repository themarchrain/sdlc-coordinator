package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/workspace"
)

type resultChannelFixture struct {
	task        *model.Task
	run         *model.Run
	finalizer   *fakeDeveloperFinalizer
	checkpoints *fakeDeliveryCheckpoints
	submitter   *recordingResultSubmitter
	core        *ResultChannelCoordinator
}

func newResultChannelFixture(role string) resultChannelFixture {
	runID := "run-channel"
	task := &model.Task{
		TaskID: "task-channel", Type: "develop_from_issue", Role: role,
		TargetRepo: "group/project", SourceRepo: "group/project",
		DeliveryBranch: "agent/task-channel", Status: model.TaskStatusRunning, CurrentRunID: &runID,
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":42},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"master","access":"writable"}]}`,
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: role, Status: model.RunStatusRunning}
	finalizer := &fakeDeveloperFinalizer{delivery: &workspace.DeveloperDelivery{CommitSHA: "commit-1", ChangedFiles: []string{"main.go"}, DiffSummary: "1 file changed"}}
	checkpoints := &fakeDeliveryCheckpoints{record: &model.DeveloperDeliveryCheckpoint{
		TaskID: task.TaskID, RunID: run.RunID, TaskType: task.Type,
		RepositoryURL: "https://gitlab.example/group/project.git", DeliveryBranch: task.DeliveryBranch,
		BaselineSHA: "baseline-sha",
	}}
	submitter := &recordingResultSubmitter{}
	core := NewResultChannelCoordinator(finalizer, submitter, ReviewerRunnerSettings{WorkspaceRoot: "D:/tmp/ws", ResolveRepository: stubRunnerRepository()}, "mcp")
	core.SetDeveloperDeliveryCheckpoints(checkpoints)
	return resultChannelFixture{task: task, run: run, finalizer: finalizer, checkpoints: checkpoints, submitter: submitter, core: core}
}

func developerChangeReadyPayload() []byte {
	return []byte(`{"taskId":"task-channel","runId":"run-channel","outcome":"change_ready","summary":"did it","tests":[{"command":"go test ./...","passed":true,"summary":"green"}],"mergeRequest":{"title":"fix","description":"desc"},"commit_message":"fix: implement the requested change"}`)
}

func TestResultChannelAcceptsDeveloperChangeReadyWithNormalization(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelAccepted || outcome.Response == nil {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
	if f.finalizer.commits != 1 || f.finalizer.pushes != 1 {
		t.Fatalf("finalizer must commit and push for change_ready, commits=%d pushes=%d", f.finalizer.commits, f.finalizer.pushes)
	}
	if f.submitter.calls != 1 {
		t.Fatalf("submitter must be called once, calls=%d", f.submitter.calls)
	}
	if f.submitter.request.Source != "mcp" || f.submitter.request.ToolName != results.SubmitDeveloperResultTool {
		t.Fatalf("unexpected submit request: %#v", f.submitter.request)
	}
	if !strings.Contains(f.submitter.request.NormalizationTrail, `"path":"taskId"`) {
		t.Fatalf("normalization trail missing rename record: %q", f.submitter.request.NormalizationTrail)
	}
	if string(f.submitter.request.RawPayload) != string(developerChangeReadyPayload()) {
		t.Fatalf("raw payload must preserve original bytes: %s", f.submitter.request.RawPayload)
	}
	if len(f.submitter.request.Canonical.Writebacks) != 1 {
		t.Fatalf("change_ready must create an MR writeback: %#v", f.submitter.request.Canonical.Writebacks)
	}
	if outcome.Normalized != true {
		t.Fatalf("outcome must report normalization happened: %#v", outcome)
	}
}

func TestResultChannelRejectsContractViolationsWithProblems(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	raw := []byte(`{"task_id":"task-channel","run_id":"run-channel","outcome":"change_ready","summary":"done"}`)
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, raw)
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelRejected {
		t.Fatalf("expected rejection, got %#v", outcome)
	}
	if len(outcome.Problems) == 0 {
		t.Fatal("rejection must carry problems")
	}
	joined := ""
	for _, problem := range outcome.Problems {
		joined += problem.Problem + "|"
	}
	if !strings.Contains(joined, "tests") {
		t.Fatalf("problems must surface the tests violation verbatim: %s", joined)
	}
	if outcome.Problems[0].Path != "tests" && outcome.Problems[0].Path != "" {
		t.Fatalf("unexpected path: %q", outcome.Problems[0].Path)
	}
	if f.submitter.calls != 0 {
		t.Fatalf("rejected results must not reach the submitter, calls=%d", f.submitter.calls)
	}
}

// TestResultChannelRecoversWithCheckpointedCommit 锁定恢复路径:检查点已记录
// 交付 SHA(如推送成功但结果入库前失败后的重复回传)时,跳过新建提交,只
// 按固定 SHA 重试推送。
func TestResultChannelRecoversWithCheckpointedCommit(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	f.checkpoints.record.CommitSHA = "recorded-commit"
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelAccepted {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
	if f.finalizer.commits != 0 || f.finalizer.pushes != 1 {
		t.Fatalf("checkpointed recovery must push only: commits=%d pushes=%d", f.finalizer.commits, f.finalizer.pushes)
	}
	if f.finalizer.pushRequest.CommitSHA != "recorded-commit" {
		t.Fatalf("push must use the checkpointed sha: %#v", f.finalizer.pushRequest)
	}
}

// TestResultChannelFailsClosedWithoutRecordedBaseline 锁定:基线缺失(部署前
// 在途 run 或检查点未写入)时 fail-closed,不碰 git、不进矫正通道。
func TestResultChannelFailsClosedWithoutRecordedBaseline(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	f.checkpoints.record = nil
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelFailed || outcome.Stage != ChannelStageFinalize {
		t.Fatalf("missing baseline must fail closed: %#v", outcome)
	}
	if f.finalizer.commits != 0 || f.finalizer.pushes != 0 {
		t.Fatalf("missing baseline must not touch git: commits=%d pushes=%d", f.finalizer.commits, f.finalizer.pushes)
	}
}

// TestResultChannelFailsClosedWhenCheckpointWriteFails 锁定:提交已创建但
// 检查点写入失败时不得推送(fail-closed),由重试路径按缺失检查点拒绝。
func TestResultChannelFailsClosedWhenCheckpointWriteFails(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	f.checkpoints.recordErr = errors.New("tenant db unavailable")
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelFailed || outcome.Stage != ChannelStageFinalize {
		t.Fatalf("checkpoint write failure must fail closed: %#v", outcome)
	}
	if f.finalizer.pushes != 0 {
		t.Fatalf("created commit must not be pushed without a persisted checkpoint: pushes=%d", f.finalizer.pushes)
	}
}

func TestResultChannelRejectsUnknownFieldsFromDecode(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	raw := []byte(`{"task_id":"task-channel","run_id":"run-channel","outcome":"fix_failed","summary":"x","blocker_reason":"y","bogus_field":1}`)
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, raw)
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelRejected || outcome.Stage != ChannelStageDecode {
		t.Fatalf("expected decode-stage rejection, got %#v", outcome)
	}
	joined := outcome.Problems[0].Problem
	if !strings.Contains(joined, "bogus_field") {
		t.Fatalf("decode problem must name the offending field: %s", joined)
	}
}

func TestResultChannelMapsFinalizeFailureToRejectedDeliveryStage(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	f.finalizer.commitErr = workspace.ErrWorkspaceConflict
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelRejected || outcome.Stage != ChannelStageFinalize {
		t.Fatalf("expected finalize-stage rejection, got %#v", outcome)
	}
}

func TestResultChannelAcceptsReviewerVerdict(t *testing.T) {
	f := newResultChannelFixture("ReviewerWorker")
	raw := []byte(`{"task_id":"task-channel","run_id":"run-channel","verdict":"approved","summary":"fine","findings":[],"test_summary":"ran","comment_body":"/lgtm"}`)
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitReviewerResultTool, raw)
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelAccepted {
		t.Fatalf("unexpected outcome: %#v err=%v", outcome, err)
	}
	if f.submitter.request.NormalizationTrail != "" {
		t.Fatalf("clean payload must leave no trail: %q", f.submitter.request.NormalizationTrail)
	}
	if outcome.Normalized {
		t.Fatal("clean payload must not report normalization")
	}
}

func TestResultChannelSurfacesSubmitterConflictsAsFailed(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	f.submitter.err = ErrResultConflict
	f.submitter.response = nil
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelFailed {
		t.Fatalf("conflict must map to failed, got %#v", outcome)
	}
	if !errors.Is(outcome.Err, ErrResultConflict) {
		t.Fatalf("conflict cause must be preserved: %v", outcome.Err)
	}
}

func TestResultChannelAcceptsChangeReadyWithEmptyValueShapes(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	raw := []byte(`{"task_id":"task-channel","run_id":"run-channel","outcome":"change_ready","summary":"wrote readme and verified","tests":[{"command":"go test ./...","passed":true,"summary":"green"}],"merge_request":{"title":"docs: add readme","description":"records commit time"},"commit_message":"fix: implement the requested change","defect":{},"blocker_reason":"null"}`)
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, raw)
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelAccepted {
		t.Fatalf("empty-value shapes must be treated as absent, got %#v problems=%v", outcome, outcome.Problems)
	}
	if f.submitter.calls != 1 {
		t.Fatalf("submitter must be called once, calls=%d", f.submitter.calls)
	}
	for _, path := range []string{"defect", "blocker_reason"} {
		if !strings.Contains(f.submitter.request.NormalizationTrail, `"path":"`+path+`"`) {
			t.Fatalf("normalization trail missing drop record for %s: %q", path, f.submitter.request.NormalizationTrail)
		}
	}
}

func TestResultChannelRejectsNullLiteralsInsideRequiredFields(t *testing.T) {
	f := newResultChannelFixture("DeveloperWorker")
	raw := []byte(`{"task_id":"task-channel","run_id":"run-channel","outcome":"change_ready","summary":"done","tests":[{"command":"go test ./...","passed":true,"summary":"green"}],"merge_request":{"title":"null","description":"records commit time"},"commit_message":"fix: implement the requested change"}`)
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, raw)
	if err != nil {
		t.Fatalf("submit agent result: %v", err)
	}
	if outcome.Decision != ChannelRejected {
		t.Fatalf(`"null" in a required field must be rejected as missing, got %#v`, outcome)
	}
	if !strings.Contains(outcome.Problems[0].Problem, "merge_request title is required") {
		t.Fatalf("rejection must name the required-field violation instead of shipping a literal null title: %s", outcome.Problems[0].Problem)
	}
	if f.submitter.calls != 0 {
		t.Fatalf("rejected results must not reach the submitter, calls=%d", f.submitter.calls)
	}
}
